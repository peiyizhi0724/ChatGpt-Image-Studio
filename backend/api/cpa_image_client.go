package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"chatgpt2api/handler"
)

type cpaImageClient struct {
	baseURL       string
	apiKey        string
	httpClient    *http.Client
	routeStrategy string
	lastRoute     string
	lastModel     string
	lastToolModel string
}

const maxCPAResponsesSSELineBytes = 128 << 20

func newCPAImageClient(baseURL, apiKey string, timeout time.Duration, routeStrategy string) *cpaImageClient {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &cpaImageClient{
		baseURL:       strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:        strings.TrimSpace(apiKey),
		routeStrategy: normalizeCPAImageClientRouteStrategy(routeStrategy),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *cpaImageClient) LastRoute() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.lastRoute)
}

func (c *cpaImageClient) LastModelLabel() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.lastModel)
}

func (c *cpaImageClient) ImageToolModel() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.lastToolModel)
}

func (c *cpaImageClient) DownloadBytes(url string) ([]byte, error) {
	if payload, err := decodeCPAImageDataURL(url); err == nil {
		return payload, nil
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimSpace(url), nil)
	if err != nil {
		return nil, fmt.Errorf("create image request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download image returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	return data, nil
}

func (c *cpaImageClient) DownloadAsBase64(ctx context.Context, url string) (string, error) {
	if payload, err := decodeCPAImageDataURL(url); err == nil {
		return base64.StdEncoding.EncodeToString(payload), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(url), nil)
	if err != nil {
		return "", fmt.Errorf("create image request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download image returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (c *cpaImageClient) GenerateImage(ctx context.Context, prompt, model string, n int, size, quality, background string) ([]handler.ImageResult, error) {
	if c.shouldUseResponsesRoute() {
		return c.generateViaResponses(ctx, prompt, size, quality, background)
	}

	body := map[string]any{
		"prompt":          strings.TrimSpace(prompt),
		"model":           strings.TrimSpace(model),
		"n":               max(1, n),
		"response_format": "b64_json",
	}
	if strings.TrimSpace(size) != "" {
		body["size"] = strings.TrimSpace(size)
	}
	if strings.TrimSpace(quality) != "" {
		body["quality"] = strings.TrimSpace(quality)
	}
	if strings.TrimSpace(background) != "" {
		body["background"] = strings.TrimSpace(background)
	}
	c.setLastRoute("images_api")
	results, err := c.executeJSONRequest(ctx, "/v1/images/generations", body)
	if err != nil && c.shouldFallbackToResponses(err) {
		fallbackResults, fallbackErr := c.generateViaResponses(ctx, prompt, size, quality, background)
		if fallbackErr == nil {
			return fallbackResults, nil
		}
		return nil, fmt.Errorf("images_api failed: %v; codex_responses fallback failed: %w", err, fallbackErr)
	}
	return results, err
}

func (c *cpaImageClient) EditImageByUpload(ctx context.Context, prompt, model string, images [][]byte, mask []byte, size, quality string) ([]handler.ImageResult, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("at least one image is required")
	}
	if c.shouldUseResponsesRoute() {
		return c.editViaResponses(ctx, prompt, images, mask, size, quality)
	}

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	_ = writer.WriteField("prompt", strings.TrimSpace(prompt))
	_ = writer.WriteField("model", strings.TrimSpace(model))
	_ = writer.WriteField("response_format", "b64_json")
	if strings.TrimSpace(size) != "" {
		_ = writer.WriteField("size", strings.TrimSpace(size))
	}
	if strings.TrimSpace(quality) != "" {
		_ = writer.WriteField("quality", strings.TrimSpace(quality))
	}

	for index, image := range images {
		part, err := writer.CreateFormFile("image", fmt.Sprintf("image-%d.png", index+1))
		if err != nil {
			return nil, fmt.Errorf("create image form field: %w", err)
		}
		if _, err := part.Write(image); err != nil {
			return nil, fmt.Errorf("write image form field: %w", err)
		}
	}
	if len(mask) > 0 {
		part, err := writer.CreateFormFile("mask", "mask.png")
		if err != nil {
			return nil, fmt.Errorf("create mask form field: %w", err)
		}
		if _, err := part.Write(mask); err != nil {
			return nil, fmt.Errorf("write mask form field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/images/edits", &payload)
	if err != nil {
		return nil, fmt.Errorf("create CPA edit request: %w", err)
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpa image edit request: %w", err)
	}
	defer resp.Body.Close()
	c.setLastRoute("images_api")
	results, parseErr := c.parseImageAPIResponse(resp)
	if parseErr != nil && c.shouldFallbackToResponses(parseErr) {
		fallbackResults, fallbackErr := c.editViaResponses(ctx, prompt, images, mask, size, quality)
		if fallbackErr == nil {
			return fallbackResults, nil
		}
		return nil, fmt.Errorf("images_api failed: %v; codex_responses fallback failed: %w", parseErr, fallbackErr)
	}
	return results, parseErr
}

func (c *cpaImageClient) InpaintImageByMask(
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
) ([]handler.ImageResult, error) {
	_ = size
	_ = quality
	return nil, newRequestError("source_context_missing", "CPA 路由不支持上下文选区编辑，将自动回退为源图加遮罩编辑")
}

func (c *cpaImageClient) executeJSONRequest(ctx context.Context, path string, body map[string]any) ([]handler.ImageResult, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal CPA image request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create CPA image request: %w", err)
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpa image request: %w", err)
	}
	defer resp.Body.Close()
	return c.parseImageAPIResponse(resp)
}

func (c *cpaImageClient) setLastRoute(route string) {
	c.lastRoute = strings.TrimSpace(route)
	c.lastToolModel = cpaFixedImageModel
	if c.lastRoute == "codex_responses" {
		c.lastModel = cpaResponsesMainModel + " (tool: " + cpaFixedImageModel + ")"
		return
	}
	c.lastModel = cpaFixedImageModel
}

func (c *cpaImageClient) parseImageAPIResponse(resp *http.Response) ([]handler.ImageResult, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read CPA response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cpa returned %d: %s", resp.StatusCode, summarizeCPAError(body))
	}

	var payload struct {
		Data []struct {
			URL             string `json:"url"`
			B64JSON         string `json:"b64_json"`
			RevisedPrompt   string `json:"revised_prompt"`
			FileID          string `json:"file_id"`
			GenID           string `json:"gen_id"`
			ConversationID  string `json:"conversation_id"`
			ParentMessageID string `json:"parent_message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode CPA response: %w", err)
	}

	results := make([]handler.ImageResult, 0, len(payload.Data))
	for _, item := range payload.Data {
		imageURL := strings.TrimSpace(item.URL)
		if imageURL == "" && strings.TrimSpace(item.B64JSON) != "" {
			imageURL = encodeCPAImageDataURLFromBase64(strings.TrimSpace(item.B64JSON), "image/png")
		}
		if imageURL == "" {
			continue
		}
		results = append(results, handler.ImageResult{
			URL:            imageURL,
			FileID:         strings.TrimSpace(item.FileID),
			GenID:          strings.TrimSpace(item.GenID),
			ConversationID: strings.TrimSpace(item.ConversationID),
			ParentMsgID:    strings.TrimSpace(item.ParentMessageID),
			RevisedPrompt:  strings.TrimSpace(item.RevisedPrompt),
		})
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("cpa did not return image output")
	}
	return results, nil
}

func (c *cpaImageClient) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func summarizeCPAError(body []byte) string {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
			return strings.TrimSpace(payload.Error.Message)
		}
		if strings.TrimSpace(payload.Message) != "" {
			return strings.TrimSpace(payload.Message)
		}
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty error response"
	}
	return trimmed
}

func detectCPAImageMIME(data []byte) string {
	if len(data) == 0 {
		return "image/png"
	}
	return http.DetectContentType(data)
}

func encodeCPAImageDataURLFromBase64(encoded, mimeType string) string {
	trimmedMimeType := strings.TrimSpace(mimeType)
	if trimmedMimeType == "" {
		trimmedMimeType = "image/png"
	}
	return "data:" + trimmedMimeType + ";base64," + strings.TrimSpace(encoded)
}

func decodeCPAImageDataURL(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "data:image/") {
		return nil, fmt.Errorf("not an image data url")
	}
	index := strings.Index(trimmed, ",")
	if index < 0 {
		return nil, fmt.Errorf("invalid image data url")
	}
	payload, err := base64.StdEncoding.DecodeString(trimmed[index+1:])
	if err != nil {
		return nil, fmt.Errorf("decode image data url: %w", err)
	}
	return payload, nil
}

const cpaResponsesMainModel = "gpt-5.4-mini"

func normalizeCPAImageClientRouteStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex_responses":
		return "codex_responses"
	case "auto":
		return "auto"
	default:
		return "images_api"
	}
}

func (c *cpaImageClient) shouldUseResponsesRoute() bool {
	return c != nil && c.routeStrategy == "codex_responses"
}

func (c *cpaImageClient) shouldFallbackToResponses(err error) bool {
	if c == nil || c.routeStrategy != "auto" || err == nil {
		return false
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}

	if strings.Contains(message, "stream disconnected before completion") ||
		strings.Contains(message, "upstream did not return image output") ||
		strings.Contains(message, "invalid sse data json") {
		return true
	}

	for _, status := range []string{"404", "405", "422", "500", "502", "503", "504"} {
		if strings.Contains(message, "cpa returned "+status) {
			return true
		}
	}
	return false
}

func (c *cpaImageClient) generateViaResponses(ctx context.Context, prompt, size, quality, background string) ([]handler.ImageResult, error) {
	payload := c.buildResponsesRequest(prompt, nil, nil, size, quality, background)
	return c.executeResponsesRequest(ctx, payload)
}

func (c *cpaImageClient) editViaResponses(ctx context.Context, prompt string, images [][]byte, mask []byte, size, quality string) ([]handler.ImageResult, error) {
	payload := c.buildResponsesRequest(prompt, images, mask, size, quality, "")
	return c.executeResponsesRequest(ctx, payload)
}

func (c *cpaImageClient) buildResponsesRequest(prompt string, images [][]byte, mask []byte, size, quality, background string) map[string]any {
	content := make([]map[string]any, 0, 1+len(images))
	content = append(content, map[string]any{
		"type": "input_text",
		"text": strings.TrimSpace(prompt),
	})
	for _, image := range images {
		if len(image) == 0 {
			continue
		}
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": encodeCPAImageDataURL(image, detectCPAImageMIME(image)),
		})
	}

	action := "generate"
	if len(images) > 0 {
		action = "edit"
	}
	tool := map[string]any{
		"type":          "image_generation",
		"action":        action,
		"model":         cpaFixedImageModel,
		"output_format": "png",
	}
	if trimmedSize := strings.TrimSpace(size); trimmedSize != "" {
		tool["size"] = trimmedSize
	}
	if trimmedQuality := strings.TrimSpace(quality); trimmedQuality != "" {
		tool["quality"] = trimmedQuality
	}
	if trimmedBackground := strings.TrimSpace(background); trimmedBackground != "" {
		tool["background"] = trimmedBackground
	}
	if len(mask) > 0 {
		tool["input_image_mask"] = map[string]any{
			"image_url": encodeCPAImageDataURL(mask, detectCPAImageMIME(mask)),
		}
	}

	return map[string]any{
		"instructions":        "",
		"stream":              true,
		"reasoning":           map[string]any{"effort": "medium", "summary": "auto"},
		"parallel_tool_calls": true,
		"include":             []string{"reasoning.encrypted_content"},
		"model":               cpaResponsesMainModel,
		"store":               false,
		"tool_choice":         map[string]any{"type": "image_generation"},
		"input": []map[string]any{
			{
				"type":    "message",
				"role":    "user",
				"content": content,
			},
		},
		"tools": []any{tool},
	}
}

func (c *cpaImageClient) executeResponsesRequest(ctx context.Context, body map[string]any) ([]handler.ImageResult, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal CPA responses request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/responses", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("create CPA responses request: %w", err)
	}
	c.setAuth(req)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cpa responses request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		if readErr != nil {
			return nil, fmt.Errorf("read CPA responses error: %w", readErr)
		}
		return nil, fmt.Errorf("cpa returned %d: %s", resp.StatusCode, summarizeCPAError(body))
	}

	c.setLastRoute("codex_responses")
	return c.parseResponsesSSE(resp.Body)
}

func (c *cpaImageClient) parseResponsesSSE(reader io.Reader) ([]handler.ImageResult, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), maxCPAResponsesSSELineBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var event cpaResponsesCompletedEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if event.Type != "response.completed" {
			continue
		}

		results := make([]handler.ImageResult, 0, len(event.Response.Output))
		for _, item := range event.Response.Output {
			if item.Type != "image_generation_call" {
				continue
			}
			result := strings.TrimSpace(item.Result)
			if result == "" {
				continue
			}

			imageURL := result
			if !strings.HasPrefix(strings.ToLower(imageURL), "data:image/") {
				imageURL = encodeCPAImageDataURLFromBase64(result, mimeTypeFromOutputFormat(item.OutputFormat))
			}
			results = append(results, handler.ImageResult{
				URL:           imageURL,
				RevisedPrompt: strings.TrimSpace(item.RevisedPrompt),
			})
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("cpa did not return image output")
		}
		return results, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read CPA responses stream: %w", err)
	}
	return nil, fmt.Errorf("cpa did not return image output")
}

type cpaResponsesCompletedEvent struct {
	Type     string `json:"type"`
	Response struct {
		Output []struct {
			Type          string `json:"type"`
			Result        string `json:"result"`
			RevisedPrompt string `json:"revised_prompt"`
			OutputFormat  string `json:"output_format"`
		} `json:"output"`
	} `json:"response"`
}

func mimeTypeFromOutputFormat(outputFormat string) string {
	if outputFormat == "" {
		return "image/png"
	}
	if strings.Contains(outputFormat, "/") {
		return outputFormat
	}
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func encodeCPAImageDataURL(data []byte, mimeType string) string {
	return encodeCPAImageDataURLFromBase64(base64.StdEncoding.EncodeToString(data), mimeType)
}
