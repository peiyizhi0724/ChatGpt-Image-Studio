package handler

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"chatgpt2api/internal/imaging"
	"github.com/google/uuid"
)

const (
	codexResponsesBaseURL    = "https://chatgpt.com/backend-api/codex"
	codexResponsesUserAgent  = "codex-tui/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9 (codex-tui; 0.118.0)"
	codexResponsesOriginator = "codex-tui"
	maxResponsesInlineImages = 1
	maxResponsesInlineBytes  = 768 << 10
	maxResponsesSSELineBytes = 128 << 20
)

type ImageWorkflowClient interface {
	DownloadBytes(url string) ([]byte, error)
	DownloadAsBase64(ctx context.Context, url string) (string, error)
	GenerateImage(ctx context.Context, prompt, model string, n int, size, quality, background string) ([]ImageResult, error)
	EditImageByUpload(ctx context.Context, prompt, model string, images [][]byte, mask []byte, size, quality string) ([]ImageResult, error)
	InpaintImageByMask(
		ctx context.Context,
		prompt string,
		model string,
		originalFileID string,
		originalGenID string,
		conversationID string,
		parentMessageID string,
		mask []byte,
		size string,
		quality string,
	) ([]ImageResult, error)
}

type ResponsesClient struct {
	backend             *ChatGPTClient
	accountID           string
	httpClient          *http.Client
	requestedImageModel string
}

func NewResponsesClientWithProxy(accessToken, proxyURL string, authData map[string]any) *ResponsesClient {
	return NewResponsesClientWithProxyAndConfig(accessToken, proxyURL, authData, ImageRequestConfig{})
}

func NewResponsesClientWithProxyAndConfig(accessToken, proxyURL string, authData map[string]any, requestConfig ImageRequestConfig) *ResponsesClient {
	requestConfig = normalizeImageRequestConfig(requestConfig)
	backend := NewChatGPTClientWithProxyAndAuthData(accessToken, proxyURL, authData, requestConfig)
	return &ResponsesClient{
		backend:   backend,
		accountID: resolveChatGPTAccountID(accessToken, authData),
		httpClient: &http.Client{
			Timeout:   requestConfig.SSETimeout + 30*time.Second,
			Transport: newChromeTransport(proxyURL),
		},
	}
}

func (c *ResponsesClient) DownloadBytes(url string) ([]byte, error) {
	if payload, err := decodeImageDataURL(url); err == nil {
		return payload, nil
	}
	return c.backend.DownloadBytes(url)
}

func (c *ResponsesClient) DownloadAsBase64(ctx context.Context, url string) (string, error) {
	if payload, err := decodeImageDataURL(url); err == nil {
		return base64.StdEncoding.EncodeToString(payload), nil
	}
	return c.backend.DownloadAsBase64(ctx, url)
}

func (c *ResponsesClient) GenerateImage(ctx context.Context, prompt, model string, n int, size, quality, background string) ([]ImageResult, error) {
	return c.generateViaResponses(ctx, buildResponsesPrompt(prompt), model, size, quality, background, nil, nil)
}

func (c *ResponsesClient) EditImageByUpload(ctx context.Context, prompt, model string, images [][]byte, mask []byte, size, quality string) ([]ImageResult, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("at least one image is required")
	}
	if !SupportsResponsesInlineEdit(images, mask) {
		return nil, fmt.Errorf("responses inline edit payload is too large")
	}
	return c.generateViaResponses(ctx, buildResponsesEditPrompt(prompt, len(images), len(mask) > 0), model, size, quality, "", images, mask)
}

func (c *ResponsesClient) InpaintImageByMask(
	ctx context.Context,
	prompt string,
	model string,
	originalFileID string,
	originalGenID string,
	conversationID string,
	parentMessageID string,
	mask []byte,
	size string,
	quality string,
) ([]ImageResult, error) {
	_ = size
	_ = quality
	return nil, fmt.Errorf("selection edit requires conversation context")
}

func (c *ResponsesClient) generateViaResponses(ctx context.Context, prompt, model string, size, quality, background string, images [][]byte, mask []byte) ([]ImageResult, error) {
	if c == nil || c.backend == nil {
		return nil, fmt.Errorf("responses client is not initialized")
	}
	if strings.TrimSpace(c.backend.accessToken) == "" {
		return nil, fmt.Errorf("access token is required")
	}
	if strings.TrimSpace(c.accountID) == "" {
		return nil, fmt.Errorf("chatgpt account id is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultUpstreamModel
	}
	toolAction := "generate"
	if len(images) > 0 {
		toolAction = "edit"
	}
	tool, err := buildResponsesImageGenerationToolWithOptions(responsesImageToolOptions{
		RequestedModel: c.resolveRequestedImageToolModel(),
		Action:         toolAction,
		Size:           size,
		Quality:        quality,
		Background:     background,
		Mask:           mask,
	})
	if err != nil {
		return nil, err
	}

	content := make([]map[string]any, 0, 1+len(images))
	content = append(content, map[string]any{
		"type": "input_text",
		"text": prompt,
	})
	for _, image := range images {
		if len(image) == 0 {
			continue
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": encodeImageDataURL(image, detectMIME(image)),
		})
	}
	payload := map[string]any{
		"model":               model,
		"input":               []any{map[string]any{"role": "user", "content": content}},
		"tools":               []any{tool},
		"tool_choice":         map[string]any{"type": "image_generation"},
		"instructions":        "You generate and edit images for the user.",
		"stream":              true,
		"store":               false,
		"parallel_tool_calls": true,
		"include":             []string{"reasoning.encrypted_content"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal responses payload: %w", err)
	}

	resp, err := c.backend.doRequestWithRetry(ctx, c.httpClient, "responses request", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexResponsesBaseURL+"/responses", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create responses request: %w", err)
		}
		c.setResponsesHeaders(req)
		return req, nil
	})
	if err != nil {
		return nil, fmt.Errorf("responses request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("responses returned %d: %s", resp.StatusCode, summarizeResponsesError(respBody))
	}

	return c.parseResponsesSSE(resp.Body, prompt)
}

func buildResponsesImageGenerationTool(size, quality, background string) (map[string]any, error) {
	return buildResponsesImageGenerationToolWithOptions(responsesImageToolOptions{
		Action:     "generate",
		Size:       size,
		Quality:    quality,
		Background: background,
	})
}

type responsesImageToolOptions struct {
	RequestedModel string
	Action         string
	Size           string
	Quality        string
	Background     string
	Mask           []byte
}

func buildResponsesImageGenerationToolWithOptions(options responsesImageToolOptions) (map[string]any, error) {
	tool := map[string]any{
		"type":          "image_generation",
		"output_format": "png",
	}
	if model := normalizeResponsesImageToolModel(options.RequestedModel); model != "" {
		tool["model"] = model
	}
	if action := strings.ToLower(strings.TrimSpace(options.Action)); action != "" {
		tool["action"] = action
	}
	if normalized := imaging.NormalizeGenerateSize(options.Size); normalized != "" {
		if err := imaging.ValidateGenerateSize(normalized); err != nil {
			return nil, err
		}
		tool["size"] = normalized
	}
	if normalizedQuality := normalizeResponsesImageQuality(options.Quality); normalizedQuality != "" {
		tool["quality"] = normalizedQuality
	}
	if normalizedBackground := normalizeResponsesImageBackground(options.Background); normalizedBackground != "" {
		tool["background"] = normalizedBackground
	}
	if len(options.Mask) > 0 {
		tool["input_image_mask"] = map[string]any{
			"image_url": encodeImageDataURL(options.Mask, detectMIME(options.Mask)),
		}
	}
	return tool, nil
}

func (c *ResponsesClient) parseResponsesSSE(reader io.Reader, prompt string) ([]ImageResult, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), maxResponsesSSELineBytes)

	var dataLines []string
	finalImages := make([]ImageResult, 0)
	partialImages := make(map[string]string)
	seen := make(map[[32]byte]struct{})

	emitImage := func(b64, outputFormat string) {
		if strings.TrimSpace(b64) == "" {
			return
		}
		hash := sha256.Sum256([]byte(b64))
		if _, ok := seen[hash]; ok {
			return
		}
		seen[hash] = struct{}{}
		finalImages = append(finalImages, ImageResult{
			URL:           encodeImageDataURLFromBase64(b64, mimeTypeFromOutputFormat(outputFormat)),
			RevisedPrompt: prompt,
		})
	}

	processFrame := func(frame string) error {
		frame = strings.TrimSpace(frame)
		if frame == "" || frame == "[DONE]" {
			return nil
		}

		var payload struct {
			Type   string `json:"type"`
			ItemID string `json:"item_id"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
			PartialImageB64 string `json:"partial_image_b64"`
			Item            *struct {
				Type         string `json:"type"`
				Result       string `json:"result"`
				OutputFormat string `json:"output_format"`
			} `json:"item"`
			Response *struct {
				Output []struct {
					Type         string `json:"type"`
					Result       string `json:"result"`
					OutputFormat string `json:"output_format"`
				} `json:"output"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(frame), &payload); err != nil {
			return nil
		}

		switch payload.Type {
		case "error":
			if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
				return errors.New(payload.Error.Message)
			}
			return errors.New("responses stream returned an error")
		case "response.image_generation_call.partial_image":
			if payload.ItemID != "" && payload.PartialImageB64 != "" {
				partialImages[payload.ItemID] = payload.PartialImageB64
			}
		case "response.output_item.done":
			if payload.Item != nil && payload.Item.Type == "image_generation_call" {
				emitImage(payload.Item.Result, payload.Item.OutputFormat)
			}
		case "response.completed":
			if payload.Response == nil {
				return nil
			}
			for _, item := range payload.Response.Output {
				if item.Type != "image_generation_call" {
					continue
				}
				emitImage(item.Result, item.OutputFormat)
			}
		}
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := processFrame(strings.Join(dataLines, "\n")); err != nil {
				return nil, err
			}
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("responses SSE read error: %w", err)
	}
	if err := processFrame(strings.Join(dataLines, "\n")); err != nil {
		return nil, err
	}

	if len(finalImages) == 0 {
		for _, b64 := range partialImages {
			emitImage(b64, "png")
		}
	}
	if len(finalImages) == 0 {
		return nil, fmt.Errorf("no images generated")
	}
	return finalImages, nil
}

func (c *ResponsesClient) setResponsesHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.backend.accessToken))
	req.Header.Set("Chatgpt-Account-Id", c.accountID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", codexResponsesUserAgent)
	req.Header.Set("Originator", codexResponsesOriginator)
	req.Header.Set("Session_id", uuid.NewString())
	req.Header.Set("Connection", "Keep-Alive")
}

func buildResponsesPrompt(prompt string) string {
	return strings.TrimSpace(prompt)
}

func buildResponsesEditPrompt(prompt string, imageCount int, hasMask bool) string {
	fullPrompt := strings.TrimSpace(prompt)
	if imageCount > 0 {
		fullPrompt += " Use the provided image inputs as source references."
	}
	if hasMask {
		fullPrompt += " The final input image is an edit mask. Only modify the masked region and preserve the unmasked content."
	}
	return strings.TrimSpace(fullPrompt)
}

func resolveChatGPTAccountID(accessToken string, authData map[string]any) string {
	if accountID := firstString(authData, "account_id", "chatgpt_account_id"); accountID != "" {
		return accountID
	}
	if authPayload, ok := authData["https://api.openai.com/auth"].(map[string]any); ok {
		if accountID := firstString(authPayload, "chatgpt_account_id"); accountID != "" {
			return accountID
		}
	}
	if tokenPayload := decodeAccessTokenPayload(accessToken); len(tokenPayload) > 0 {
		if authPayload, ok := tokenPayload["https://api.openai.com/auth"].(map[string]any); ok {
			if accountID := firstString(authPayload, "chatgpt_account_id"); accountID != "" {
				return accountID
			}
		}
	}
	return ""
}

func summarizeResponsesError(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "empty error response"
	}

	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return payload.Error.Message
		}
		if strings.TrimSpace(payload.Detail) != "" {
			return payload.Detail
		}
	}
	return string(body)
}

func encodeImageDataURL(data []byte, mimeType string) string {
	return encodeImageDataURLFromBase64(base64.StdEncoding.EncodeToString(data), mimeType)
}

func encodeImageDataURLFromBase64(encoded, mimeType string) string {
	trimmedMimeType := strings.TrimSpace(mimeType)
	if trimmedMimeType == "" {
		trimmedMimeType = "image/png"
	}
	return "data:" + trimmedMimeType + ";base64," + strings.TrimSpace(encoded)
}

func decodeImageDataURL(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "data:image/") {
		return nil, fmt.Errorf("not an image data url")
	}
	index := strings.Index(trimmed, ",")
	if index < 0 {
		return nil, fmt.Errorf("invalid image data url")
	}
	encoded := trimmed[index+1:]
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode image data url: %w", err)
	}
	return payload, nil
}

func mimeTypeFromOutputFormat(outputFormat string) string {
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func SupportsResponsesInlineEdit(images [][]byte, mask []byte) bool {
	if len(images) != maxResponsesInlineImages {
		return false
	}

	totalBytes := 0
	for _, image := range images {
		if len(image) == 0 {
			return false
		}
		totalBytes += len(image)
	}
	totalBytes += len(mask)
	return totalBytes > 0 && totalBytes <= maxResponsesInlineBytes
}

func (c *ResponsesClient) SetRequestedImageModel(model string) {
	if c == nil {
		return
	}
	c.requestedImageModel = strings.TrimSpace(model)
}

func (c *ResponsesClient) resolveRequestedImageToolModel() string {
	if c == nil {
		return normalizeResponsesImageToolModel("")
	}
	return normalizeResponsesImageToolModel(c.requestedImageModel)
}

func (c *ResponsesClient) ImageToolModel() string {
	return c.resolveRequestedImageToolModel()
}

func normalizeResponsesImageQuality(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "":
		return ""
	case "hd":
		return "high"
	default:
		return normalized
	}
}

func normalizeResponsesImageBackground(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeResponsesImageToolModel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto", "gpt-image-1", "gpt-image-2":
		return ""
	case "gpt-5.4-mini":
		return "gpt-5.4-mini"
	case "gpt-5.4":
		return "gpt-5.4"
	case "gpt-5.5":
		return "gpt-5.5"
	case "gpt-5-5-thinking":
		return "gpt-5-5-thinking"
	default:
		return ""
	}
}
