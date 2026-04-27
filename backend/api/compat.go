package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chatgpt2api/handler"
	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/imaging"
)

type imageGenerationRequest struct {
	Model          string
	Prompt         string
	N              int
	Size           string
	Quality        string
	Background     string
	ResponseFormat string
}

type imageEditRequest struct {
	Model          string
	Prompt         string
	Images         [][]byte
	Mask           []byte
	Size           string
	Quality        string
	ResponseFormat string
}

type compatChatCompletionRequest struct {
	Model          string              `json:"model"`
	Messages       []compatChatMessage `json:"messages"`
	Stream         bool                `json:"stream"`
	N              int                 `json:"n"`
	Size           string              `json:"size"`
	Quality        string              `json:"quality"`
	Background     string              `json:"background"`
	ResponseFormat string              `json:"response_format"`
}

type compatChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type compatResponseRequest struct {
	Model          string               `json:"model"`
	Input          any                  `json:"input"`
	Tools          []compatResponseTool `json:"tools"`
	Stream         bool                 `json:"stream"`
	N              int                  `json:"n"`
	Size           string               `json:"size"`
	Quality        string               `json:"quality"`
	Background     string               `json:"background"`
	ResponseFormat string               `json:"response_format"`
}

type compatResponseTool struct {
	Type string `json:"type"`
}

var compatImageFetchClient = &http.Client{Timeout: 30 * time.Second}

type compatInputError struct {
	code    string
	message string
}

func (e *compatInputError) Error() string {
	return firstNonEmpty(strings.TrimSpace(e.message), strings.TrimSpace(e.code))
}

func newCompatInputError(code, message string) error {
	return &compatInputError{
		code:    strings.TrimSpace(code),
		message: strings.TrimSpace(message),
	}
}

func (s *Server) executeImageGeneration(ctx context.Context, req imageGenerationRequest, r *http.Request) (map[string]any, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, newRequestError("prompt_required", "prompt is required")
	}
	if req.N < 1 {
		req.N = 1
	}
	size := normalizeGenerateImageSize(req.Size)
	requirePaidAccount := s.configuredImageMode() == "studio" && imaging.RequiresPaidGenerateAccount(size)

	requestedModel := normalizeRequestedImageModel(req.Model, s.cfg.ChatGPT.Model)
	responseFormat := firstNonEmpty(req.ResponseFormat, s.cfg.App.ImageFormat, "url")
	combined := make([]map[string]any, 0, req.N)
	imageErrors := make([]string, 0)
	var firstRequestErr error
	for range req.N {
		var allowAccount func(accounts.PublicAccount) bool
		if requirePaidAccount {
			allowAccount = func(account accounts.PublicAccount) bool {
				return isPaidImageAccountType(account.Type)
			}
		}

		data, err := s.withImageResultsFilteredWithMetadata(ctx, "generate", responseFormat, "", requestedModel, true, allowAccount, newImageRequestMetadata(prompt, size, req.Quality), func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
			return client.GenerateImage(ctx, prompt, upstreamModel, 1, size, req.Quality, req.Background)
		}, r)
		if err != nil {
			if requirePaidAccount && errors.Is(err, accounts.ErrNoAvailableImageAuth) {
				err = newRequestError("paid_resolution_requires_paid_account", "当前分辨率仅支持 Plus / Pro / Team 图片账号，请先确保有可用 Paid 账号")
			}
			if firstRequestErr == nil && requestErrorCode(err) != "" {
				firstRequestErr = err
			}
			imageErrors = append(imageErrors, err.Error())
			continue
		}
		combined = append(combined, data...)
	}

	if len(combined) == 0 {
		if firstRequestErr != nil {
			return nil, firstRequestErr
		}
		return nil, errors.New(firstNonEmpty(strings.Join(imageErrors, "; "), "image generation failed"))
	}

	payload := map[string]any{
		"created": time.Now().Unix(),
		"data":    combined,
	}
	if len(imageErrors) > 0 {
		payload["errors"] = imageErrors
	}
	return payload, nil
}

func normalizeGenerateImageSize(value string) string {
	return imaging.NormalizeGenerateSize(value)
}

func (s *Server) executeImageEdit(ctx context.Context, req imageEditRequest, r *http.Request) (map[string]any, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, newRequestError("prompt_required", "prompt is required")
	}
	if len(req.Images) == 0 {
		return nil, newRequestError("image_required", "at least one image is required")
	}

	requestedModel := normalizeRequestedImageModel(req.Model, s.cfg.ChatGPT.Model)
	responseFormat := firstNonEmpty(req.ResponseFormat, s.cfg.App.ImageFormat, "url")
	data, err := s.withImageResultsWithMetadata(ctx, "edit", responseFormat, "", requestedModel, handler.SupportsResponsesInlineEdit(req.Images, req.Mask), newImageRequestMetadata(prompt, req.Size, req.Quality), func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
		return client.EditImageByUpload(ctx, prompt, upstreamModel, req.Images, req.Mask, req.Size, req.Quality)
	}, r)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"created": time.Now().Unix(),
		"data":    data,
	}, nil
}

func (s *Server) handleImageChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req compatChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "", "invalid request body")
		return
	}
	if req.Stream {
		writeAPIError(w, http.StatusBadRequest, "stream_not_supported", "stream is not supported for image generation")
		return
	}

	prompt, imageURLs := extractCompatPromptAndImagesFromMessages(req.Messages)
	if strings.TrimSpace(prompt) == "" {
		writeAPIError(w, http.StatusBadRequest, "prompt_required", "prompt is required")
		return
	}

	payload, err := s.runCompatImageRequest(r.Context(), r, compatRunRequest{
		DisplayModel:   firstNonEmpty(strings.TrimSpace(req.Model), "gpt-image-2"),
		RequestedModel: normalizeCompatRequestedModel(req.Model, s.cfg.ChatGPT.Model),
		Prompt:         prompt,
		ImageURLs:      imageURLs,
		N:              req.N,
		Size:           req.Size,
		Quality:        req.Quality,
		Background:     req.Background,
	})
	if err != nil {
		writeCompatImageError(w, err)
		return
	}

	s.recordPortalUsage(r, 1, len(compatResponseDataItems(payload)))
	writeJSON(w, http.StatusOK, buildCompatChatCompletionResponse(firstNonEmpty(strings.TrimSpace(req.Model), "gpt-image-2"), payload))
}

func (s *Server) handleImageResponses(w http.ResponseWriter, r *http.Request) {
	var req compatResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "", "invalid request body")
		return
	}
	if req.Stream {
		writeAPIError(w, http.StatusBadRequest, "stream_not_supported", "stream is not supported for image generation")
		return
	}
	if !hasCompatImageGenerationTool(req.Tools) {
		writeAPIError(w, http.StatusBadRequest, "image_generation_tool_required", "only image_generation tool requests are supported on this endpoint")
		return
	}

	prompt, imageURLs := extractCompatPromptAndImages(req.Input)
	if strings.TrimSpace(prompt) == "" {
		writeAPIError(w, http.StatusBadRequest, "prompt_required", "input text is required")
		return
	}

	payload, err := s.runCompatImageRequest(r.Context(), r, compatRunRequest{
		DisplayModel:   firstNonEmpty(strings.TrimSpace(req.Model), "gpt-image-2"),
		RequestedModel: normalizeCompatRequestedModel(req.Model, s.cfg.ChatGPT.Model),
		Prompt:         prompt,
		ImageURLs:      imageURLs,
		N:              req.N,
		Size:           req.Size,
		Quality:        req.Quality,
		Background:     req.Background,
	})
	if err != nil {
		writeCompatImageError(w, err)
		return
	}

	s.recordPortalUsage(r, 1, len(compatResponseDataItems(payload)))
	writeJSON(w, http.StatusOK, buildCompatResponsesResponse(firstNonEmpty(strings.TrimSpace(req.Model), "gpt-image-2"), payload))
}

type compatRunRequest struct {
	DisplayModel   string
	RequestedModel string
	Prompt         string
	ImageURLs      []string
	N              int
	Size           string
	Quality        string
	Background     string
}

func (s *Server) runCompatImageRequest(ctx context.Context, r *http.Request, req compatRunRequest) (map[string]any, error) {
	if len(req.ImageURLs) == 0 {
		return s.executeImageGeneration(ctx, imageGenerationRequest{
			Model:          req.RequestedModel,
			Prompt:         req.Prompt,
			N:              max(1, req.N),
			Size:           req.Size,
			Quality:        req.Quality,
			Background:     req.Background,
			ResponseFormat: "b64_json",
		}, r)
	}

	images := make([][]byte, 0, len(req.ImageURLs))
	for _, rawURL := range req.ImageURLs {
		data, err := readCompatImageURL(ctx, r, rawURL)
		if err != nil {
			return nil, newCompatInputError("invalid_image_input", err.Error())
		}
		images = append(images, data)
	}
	return s.executeImageEdit(ctx, imageEditRequest{
		Model:          req.RequestedModel,
		Prompt:         req.Prompt,
		Images:         images,
		Size:           req.Size,
		Quality:        req.Quality,
		ResponseFormat: "b64_json",
	}, r)
}

func normalizeCompatRequestedModel(model, fallback string) string {
	switch strings.TrimSpace(model) {
	case "gpt-image-1", "gpt-image-2":
		return strings.TrimSpace(model)
	default:
		return normalizeRequestedImageModel("", fallback)
	}
}

func hasCompatImageGenerationTool(tools []compatResponseTool) bool {
	for _, tool := range tools {
		if strings.EqualFold(strings.TrimSpace(tool.Type), "image_generation") {
			return true
		}
	}
	return false
}

func extractCompatPromptAndImagesFromMessages(messages []compatChatMessage) (string, []string) {
	texts := make([]string, 0, len(messages))
	images := make([]string, 0)
	for _, message := range messages {
		role := strings.TrimSpace(strings.ToLower(message.Role))
		if role == "assistant" || role == "tool" {
			continue
		}
		appendCompatPromptAndImages(message.Content, &texts, &images)
	}
	return strings.Join(texts, "\n\n"), images
}

func extractCompatPromptAndImages(input any) (string, []string) {
	texts := make([]string, 0, 2)
	images := make([]string, 0)
	appendCompatPromptAndImages(input, &texts, &images)
	return strings.Join(texts, "\n\n"), images
}

func appendCompatPromptAndImages(value any, texts *[]string, images *[]string) {
	switch typed := value.(type) {
	case nil:
		return
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			*texts = append(*texts, trimmed)
		}
	case []any:
		for _, item := range typed {
			appendCompatPromptAndImages(item, texts, images)
		}
	case map[string]any:
		if content, ok := typed["content"]; ok {
			role := strings.TrimSpace(strings.ToLower(stringValue(typed["role"])))
			if role == "assistant" || role == "tool" {
				return
			}
			appendCompatPromptAndImages(content, texts, images)
			return
		}

		itemType := strings.TrimSpace(stringValue(typed["type"]))
		switch itemType {
		case "text", "input_text":
			if text := strings.TrimSpace(stringValue(typed["text"])); text != "" {
				*texts = append(*texts, text)
			}
			return
		case "image_url", "input_image":
			if imageURL := strings.TrimSpace(extractCompatImageURL(typed["image_url"])); imageURL != "" {
				*images = append(*images, imageURL)
			}
			return
		}
		if imageURL := strings.TrimSpace(extractCompatImageURL(typed["image_url"])); imageURL != "" {
			*images = append(*images, imageURL)
		}
	case []map[string]any:
		for _, item := range typed {
			appendCompatPromptAndImages(item, texts, images)
		}
	}
}

func extractCompatImageURL(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		return firstNonEmpty(
			strings.TrimSpace(stringValue(typed["url"])),
			strings.TrimSpace(stringValue(typed["image_url"])),
		)
	default:
		return ""
	}
}

func readCompatImageURL(ctx context.Context, r *http.Request, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("image url is required")
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "data:") {
		return decodeCompatDataURL(rawURL)
	}

	resolvedURL, err := resolveCompatRemoteURL(rawURL, r)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolvedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create image request failed")
	}
	resp, err := compatImageFetchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download image failed")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("read image failed")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image is empty")
	}
	return data, nil
}

func resolveCompatRemoteURL(rawURL string, r *http.Request) (string, error) {
	if strings.HasPrefix(rawURL, "/") {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
			scheme = "https"
		}
		return fmt.Sprintf("%s://%s%s", scheme, r.Host, rawURL), nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid image url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("only data urls and same-origin image urls are supported")
	}

	requestAuthority := normalizeCompatAuthority(r.Host)
	if forwardedHost := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwardedHost != "" {
		requestAuthority = normalizeCompatAuthority(forwardedHost)
	}
	targetAuthority := normalizeCompatAuthority(parsed.Host)
	if targetAuthority == "" {
		return "", fmt.Errorf("invalid image url")
	}
	if requestAuthority != "" && requestAuthority == targetAuthority {
		return parsed.String(), nil
	}
	return "", fmt.Errorf("only data urls and same-origin image urls are supported")
}

func normalizeCompatAuthority(rawHost string) string {
	authority := strings.TrimSpace(rawHost)
	if authority == "" {
		return ""
	}
	if parsedHost, parsedPort, err := net.SplitHostPort(authority); err == nil {
		return strings.ToLower(net.JoinHostPort(strings.Trim(parsedHost, "[]"), parsedPort))
	}
	return strings.ToLower(strings.Trim(authority, "[]"))
}

func decodeCompatDataURL(raw string) ([]byte, error) {
	comma := strings.Index(raw, ",")
	if comma < 0 {
		return nil, fmt.Errorf("invalid data url")
	}
	meta := strings.ToLower(raw[:comma])
	if !strings.Contains(meta, ";base64") {
		return nil, fmt.Errorf("only base64 data urls are supported")
	}
	payload, err := base64.StdEncoding.DecodeString(raw[comma+1:])
	if err != nil {
		return nil, fmt.Errorf("invalid base64 image")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("image is empty")
	}
	return payload, nil
}

func buildCompatChatCompletionResponse(model string, payload map[string]any) map[string]any {
	created := int64Value(payload["created"])
	imageItems := compatResponseDataItems(payload)
	messageImages := make([]map[string]any, 0, len(imageItems))
	markdownImages := make([]string, 0, len(imageItems))
	for index, item := range imageItems {
		if b64 := strings.TrimSpace(stringValue(item["b64_json"])); b64 != "" {
			messageImages = append(messageImages, map[string]any{
				"b64_json":       b64,
				"revised_prompt": stringValue(item["revised_prompt"]),
			})
			markdownImages = append(markdownImages, fmt.Sprintf("![image_%d](data:image/png;base64,%s)", index+1, b64))
		}
	}
	content := "Image generation completed."
	if len(markdownImages) > 0 {
		content = strings.Join(markdownImages, "\n\n")
	}
	return map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
					"images":  messageImages,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
}

func buildCompatResponsesResponse(model string, payload map[string]any) map[string]any {
	created := int64Value(payload["created"])
	imageItems := compatResponseDataItems(payload)
	output := make([]map[string]any, 0, len(imageItems))
	for index, item := range imageItems {
		if b64 := strings.TrimSpace(stringValue(item["b64_json"])); b64 != "" {
			output = append(output, map[string]any{
				"id":             fmt.Sprintf("ig_%d", index+1),
				"type":           "image_generation_call",
				"status":         "completed",
				"result":         b64,
				"revised_prompt": strings.TrimSpace(stringValue(item["revised_prompt"])),
			})
		}
	}
	return map[string]any{
		"id":                  fmt.Sprintf("resp_%d", created),
		"object":              "response",
		"created_at":          created,
		"status":              "completed",
		"error":               nil,
		"incomplete_details":  nil,
		"model":               model,
		"output":              output,
		"parallel_tool_calls": false,
	}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return 0
	}
}

func compatResponseDataItems(payload map[string]any) []map[string]any {
	rawItems, _ := payload["data"].([]map[string]any)
	if rawItems != nil {
		return rawItems
	}
	itemsAny, _ := payload["data"].([]any)
	items := make([]map[string]any, 0, len(itemsAny))
	for _, item := range itemsAny {
		typed, _ := item.(map[string]any)
		if typed != nil {
			items = append(items, typed)
		}
	}
	return items
}

func writeCompatImageError(w http.ResponseWriter, err error) {
	if err == nil {
		writeAPIError(w, http.StatusBadGateway, "", "image generation failed")
		return
	}
	var inputErr *compatInputError
	if errors.As(err, &inputErr) {
		writeAPIError(w, http.StatusBadRequest, inputErr.code, inputErr.message)
		return
	}
	writeAPIError(w, http.StatusBadGateway, requestErrorCode(err), err.Error())
}
