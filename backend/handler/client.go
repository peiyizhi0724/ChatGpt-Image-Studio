package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"chatgpt2api/internal/outboundproxy"

	"github.com/google/uuid"
)

const (
	baseURL              = "https://chatgpt.com/backend-api"
	defaultUpstreamModel = "gpt-5.4-mini"
)

const (
	defaultRequestTimeout = 30 * time.Second
	defaultSSETimeout     = 10 * time.Minute
	defaultPollInterval   = 3 * time.Second
	defaultPollMaxWait    = 10 * time.Minute
)

type ImageRequestConfig struct {
	RequestTimeout      time.Duration
	SSETimeout          time.Duration
	PollInterval        time.Duration
	PollMaxWait         time.Duration
	NetworkRetryHook    NetworkRetryHook
	NetworkRetryCount   int
	NetworkRetryBackoff time.Duration
}

func normalizeImageRequestConfig(cfg ImageRequestConfig) ImageRequestConfig {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.SSETimeout <= 0 {
		cfg.SSETimeout = defaultSSETimeout
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.PollMaxWait <= 0 {
		cfg.PollMaxWait = defaultPollMaxWait
	}
	if cfg.PollMaxWait < cfg.SSETimeout {
		cfg.PollMaxWait = cfg.SSETimeout
	}
	if cfg.NetworkRetryCount < 0 {
		cfg.NetworkRetryCount = 0
	}
	if cfg.NetworkRetryBackoff <= 0 {
		cfg.NetworkRetryBackoff = defaultNetworkRetryBackoff
	}
	return cfg
}

// ImageResult represents a single generated image.
type ImageResult struct {
	URL            string `json:"url"`
	FileID         string `json:"file_id"`
	GenID          string `json:"gen_id"`
	ConversationID string `json:"conversation_id"`
	ParentMsgID    string `json:"parent_message_id"`
	RevisedPrompt  string `json:"revised_prompt"`
}

type ChatGPTClient struct {
	accessToken          string
	cookies              string
	oaiDeviceID          string
	oaiSessionID         string
	userAgent            string
	secCHUA              string
	secCHUAMobile        string
	secCHUAPlatform      string
	httpClient           *http.Client
	streamClient         *http.Client
	proxyURL             string
	pollInterval         time.Duration
	pollMaxWait          time.Duration
	lastImageRoute       string
	networkRetryAttempts int
	networkRetryBackoff  time.Duration
	beforeNetworkRetry   NetworkRetryHook
}

func NewChatGPTClient(accessToken, cookies string) *ChatGPTClient {
	return NewChatGPTClientWithProxy(accessToken, cookies, "")
}

func NewChatGPTClientWithProxy(accessToken, cookies, proxyURL string) *ChatGPTClient {
	return NewChatGPTClientWithProxyAndConfig(accessToken, cookies, proxyURL, ImageRequestConfig{})
}

func NewChatGPTClientWithProxyAndConfig(accessToken, cookies, proxyURL string, requestConfig ImageRequestConfig) *ChatGPTClient {
	authData := map[string]any{}
	if strings.TrimSpace(cookies) != "" {
		authData["cookies"] = strings.TrimSpace(cookies)
	}
	return NewChatGPTClientWithProxyAndAuthData(accessToken, proxyURL, authData, requestConfig)
}

func NewChatGPTClientWithProxyAndAuthData(accessToken, proxyURL string, authData map[string]any, requestConfig ImageRequestConfig) *ChatGPTClient {
	requestConfig = normalizeImageRequestConfig(requestConfig)
	cookies := firstString(authData, "cookies", "cookie")
	userAgent := stringOrDefault(authData, "user-agent", defaultUserAgent)
	deviceID := firstString(authData, "oai-device-id", "oai_device_id", "device_id")
	if strings.TrimSpace(deviceID) == "" {
		deviceID = uuid.NewString()
	}
	return &ChatGPTClient{
		accessToken:     accessToken,
		cookies:         cookies,
		oaiDeviceID:     deviceID,
		oaiSessionID:    firstString(authData, "oai-session-id", "oai_session_id", "session_id"),
		userAgent:       userAgent,
		secCHUA:         stringOrDefault(authData, "sec-ch-ua", `"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`),
		secCHUAMobile:   stringOrDefault(authData, "sec-ch-ua-mobile", "?0"),
		secCHUAPlatform: stringOrDefault(authData, "sec-ch-ua-platform", `"Windows"`),
		proxyURL:        strings.TrimSpace(proxyURL),
		httpClient: &http.Client{
			Timeout:   requestConfig.RequestTimeout,
			Transport: newChromeTransport(proxyURL),
		},
		streamClient: &http.Client{
			Timeout:   requestConfig.SSETimeout + 30*time.Second,
			Transport: newChromeTransport(proxyURL),
		},
		pollInterval:         requestConfig.PollInterval,
		pollMaxWait:          requestConfig.PollMaxWait,
		networkRetryAttempts: requestConfig.NetworkRetryCount,
		networkRetryBackoff:  requestConfig.NetworkRetryBackoff,
		beforeNetworkRetry:   requestConfig.NetworkRetryHook,
	}
}

func ResolveImageUpstreamModel(requestedModel, accountType string) string {
	return ResolveImageUpstreamModelWithDefaults(requestedModel, accountType, "auto", defaultUpstreamModel)
}

func ResolveImageUpstreamModelWithDefaults(requestedModel, accountType, freeModel, paidModel string) string {
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		model = "gpt-image-2"
	}

	freeModel = normalizeImageRouteModel(freeModel, "auto")
	paidModel = normalizeImageRouteModel(paidModel, defaultUpstreamModel)

	switch model {
	case "gpt-image-1":
		if strings.TrimSpace(accountType) == "" || strings.EqualFold(strings.TrimSpace(accountType), "Free") {
			return freeModel
		}
		return paidModel
	case "gpt-image-2":
		if strings.TrimSpace(accountType) == "" || strings.EqualFold(strings.TrimSpace(accountType), "Free") {
			return freeModel
		}
		return paidModel
	default:
		return model
	}
}

func normalizeImageRouteModel(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

// GenerateImage creates a new image from a text prompt.
func (c *ChatGPTClient) GenerateImage(ctx context.Context, prompt, model string, n int, size, quality, background string) ([]ImageResult, error) {
	fullPrompt := prompt
	if size != "" && size != "auto" && size != "1024x1024" {
		fullPrompt = fmt.Sprintf("Generate an image with size %s. %s", size, prompt)
	}
	if quality == "hd" || quality == "high" {
		fullPrompt = fmt.Sprintf("Generate a high-quality, detailed image: %s", fullPrompt)
	}
	if background == "transparent" {
		fullPrompt = fullPrompt + " The image must have a transparent background (PNG with alpha channel)."
	}

	body := c.buildConversationBody(fullPrompt, model, "", "", nil)
	fBody := cloneConversationBody(body)
	fBody["client_prepare_state"] = "none"
	fBody["supported_encodings"] = []string{"v1"}

	images, err := c.doFConversation(ctx, fBody)
	if err == nil {
		c.setLastImageRoute("f-conversation")
		return images, nil
	}
	if !shouldFallbackFromFConversation(err) {
		return nil, err
	}

	images, err = c.doConversation(ctx, body)
	if err == nil {
		c.setLastImageRoute("conversation")
	}
	return images, err
}

// DownloadBytes fetches a URL using authenticated headers and returns its raw bytes.
func (c *ChatGPTClient) DownloadBytes(url string) ([]byte, error) {
	resp, err := c.doRequestWithRetry(context.Background(), c.httpClient, "download request", func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("download returned %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

// DownloadAsBase64 fetches a URL and returns its content as base64.
func (c *ChatGPTClient) DownloadAsBase64(ctx context.Context, url string) (string, error) {
	data, err := c.DownloadBytes(url)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// UploadedFile holds the result of a file upload to ChatGPT.
type UploadedFile struct {
	FileID      string
	DownloadURL string
	SizeBytes   int
	Width       int
	Height      int
	MIMEType    string
}

type uploadOptions struct {
	useCase             string
	timezoneOffsetMin   int
	resetRateLimits     bool
	processUploadStream bool
}

// UploadFile uploads an image to ChatGPT's backend for use in conversations.
// It performs a 3-step process: pre-upload → blob upload → confirm.
func (c *ChatGPTClient) UploadFile(ctx context.Context, data []byte, filename, mimeType string) (*UploadedFile, error) {
	return c.uploadFile(ctx, data, filename, mimeType, uploadOptions{
		useCase:             "multimodal",
		processUploadStream: false,
	})
}

func (c *ChatGPTClient) uploadFile(ctx context.Context, data []byte, filename, mimeType string, options uploadOptions) (*UploadedFile, error) {
	if mimeType == "" {
		mimeType = "image/png"
	}
	if strings.TrimSpace(options.useCase) == "" {
		options.useCase = "multimodal"
	}

	// Step 1: Pre-upload to get upload URL and file ID
	prePayload := map[string]any{
		"file_name": filename,
		"file_size": len(data),
		"use_case":  options.useCase,
	}
	if options.processUploadStream {
		prePayload["timezone_offset_min"] = options.timezoneOffsetMin
		prePayload["reset_rate_limits"] = options.resetRateLimits
	} else {
		prePayload["mime_type"] = mimeType
	}
	preBody, _ := json.Marshal(prePayload)
	preResp, err := c.doRequestWithRetry(ctx, c.httpClient, "pre-upload request", func() (*http.Request, error) {
		preReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/files", bytes.NewReader(preBody))
		if err != nil {
			return nil, err
		}
		c.setHeaders(preReq)
		return preReq, nil
	})
	if err != nil {
		return nil, fmt.Errorf("pre-upload request: %w", err)
	}
	defer preResp.Body.Close()

	if preResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(preResp.Body, 1024))
		return nil, fmt.Errorf("pre-upload returned %d: %s", preResp.StatusCode, string(body))
	}

	var preResult struct {
		Status    string `json:"status"`
		UploadURL string `json:"upload_url"`
		FileID    string `json:"file_id"`
	}
	if err := json.NewDecoder(preResp.Body).Decode(&preResult); err != nil {
		return nil, fmt.Errorf("decode pre-upload: %w", err)
	}
	if preResult.UploadURL == "" || preResult.FileID == "" {
		return nil, fmt.Errorf("pre-upload returned empty upload_url or file_id")
	}

	log.Printf("[upload] pre-upload ok: file_id=%s", preResult.FileID)

	// Step 2: Upload to Azure Blob Storage
	// Use a plain HTTP client (no uTLS needed for Azure)
	plainTransport, err := outboundproxy.NewHTTPTransport(c.proxyURL)
	if err != nil {
		return nil, fmt.Errorf("create upload transport: %w", err)
	}
	plainClient := &http.Client{
		Timeout:   60 * time.Second,
		Transport: plainTransport,
	}
	uploadResp, err := c.doRequestWithRetry(ctx, plainClient, "blob upload request", func() (*http.Request, error) {
		uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPut, preResult.UploadURL, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		uploadReq.Header.Set("x-ms-blob-type", "BlockBlob")
		uploadReq.Header.Set("Content-Type", mimeType)
		return uploadReq, nil
	})
	if err != nil {
		return nil, fmt.Errorf("blob upload request: %w", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(uploadResp.Body, 1024))
		return nil, fmt.Errorf("blob upload returned %d: %s", uploadResp.StatusCode, string(body))
	}

	log.Printf("[upload] blob upload ok: %d bytes", len(data))

	// Step 3: Confirm upload
	if options.processUploadStream {
		if err := c.processUploadStream(ctx, preResult.FileID, options.useCase, filename); err != nil {
			return nil, err
		}
		w, h := detectImageSize(data)
		return &UploadedFile{
			FileID:      preResult.FileID,
			DownloadURL: "",
			SizeBytes:   len(data),
			Width:       w,
			Height:      h,
			MIMEType:    mimeType,
		}, nil
	}

	confirmResp, err := c.doRequestWithRetry(ctx, c.httpClient, "confirm upload request", func() (*http.Request, error) {
		confirmReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/files/"+preResult.FileID+"/uploaded", bytes.NewReader([]byte("{}")))
		if err != nil {
			return nil, err
		}
		c.setHeaders(confirmReq)
		return confirmReq, nil
	})
	if err != nil {
		return nil, fmt.Errorf("confirm upload request: %w", err)
	}
	defer confirmResp.Body.Close()

	if confirmResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(confirmResp.Body, 1024))
		return nil, fmt.Errorf("confirm upload returned %d: %s", confirmResp.StatusCode, string(body))
	}

	var confirmResult struct {
		Status      string `json:"status"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.NewDecoder(confirmResp.Body).Decode(&confirmResult); err != nil {
		return nil, fmt.Errorf("decode confirm: %w", err)
	}

	log.Printf("[upload] confirmed: file_id=%s", preResult.FileID)

	w, h := detectImageSize(data)
	return &UploadedFile{
		FileID:      preResult.FileID,
		DownloadURL: confirmResult.DownloadURL,
		SizeBytes:   len(data),
		Width:       w,
		Height:      h,
		MIMEType:    mimeType,
	}, nil
}

func (c *ChatGPTClient) processUploadStream(ctx context.Context, fileID, useCase, filename string) error {
	processBody, _ := json.Marshal(map[string]any{
		"file_id":             fileID,
		"use_case":            useCase,
		"index_for_retrieval": false,
		"file_name":           filename,
	})
	processResp, err := c.doRequestWithRetry(ctx, c.streamClient, "process upload stream request", func() (*http.Request, error) {
		processReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/files/process_upload_stream", bytes.NewReader(processBody))
		if err != nil {
			return nil, err
		}
		c.setHeaders(processReq)
		processReq.Header.Set("Accept", "text/event-stream")
		processReq.Header.Set("x-openai-target-path", "/backend-api/files/process_upload_stream")
		processReq.Header.Set("x-openai-target-route", "/backend-api/files/process_upload_stream")
		return processReq, nil
	})
	if err != nil {
		return fmt.Errorf("process upload stream request: %w", err)
	}
	defer processResp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(processResp.Body, 1<<20))
	if processResp.StatusCode != http.StatusOK {
		return fmt.Errorf("process upload stream returned %d: %s", processResp.StatusCode, string(body))
	}
	if readErr != nil {
		return fmt.Errorf("read process upload stream: %w", readErr)
	}
	if len(body) > 0 && !bytes.Contains(body, []byte("file.processing.completed")) && !bytes.Contains(body, []byte("file_ready")) {
		log.Printf("[upload] process upload stream returned without completion marker for file_id=%s", fileID)
	}
	log.Printf("[upload] process upload stream ok: file_id=%s", fileID)
	return nil
}

func (c *ChatGPTClient) UploadMaskForInpainting(ctx context.Context, data []byte, filename string) (*UploadedFile, error) {
	return c.uploadFile(ctx, data, filename, detectMIME(data), uploadOptions{
		useCase:             "dalle_agent",
		timezoneOffsetMin:   -480,
		resetRateLimits:     false,
		processUploadStream: true,
	})
}

// detectImageSize reads width/height from PNG or JPEG headers.
func detectImageSize(data []byte) (int, int) {
	if len(data) < 24 {
		return 0, 0
	}
	// PNG: width at bytes 16-19, height at bytes 20-23 (big-endian in IHDR)
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		w := int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
		h := int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
		return w, h
	}
	// JPEG: scan for SOF0 (0xFFC0) marker
	if data[0] == 0xFF && data[1] == 0xD8 {
		i := 2
		for i < len(data)-9 {
			if data[i] != 0xFF {
				i++
				continue
			}
			marker := data[i+1]
			if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
				h := int(data[i+5])<<8 | int(data[i+6])
				w := int(data[i+7])<<8 | int(data[i+8])
				return w, h
			}
			segLen := int(data[i+2])<<8 | int(data[i+3])
			i += 2 + segLen
		}
	}
	return 0, 0
}

// EditImageByUpload uploads images to ChatGPT, then sends an edit conversation.
// images is a list of image byte slices. mask is optional (nil = no mask).
func (c *ChatGPTClient) EditImageByUpload(ctx context.Context, prompt, model string, images [][]byte, mask []byte, size, quality string) ([]ImageResult, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("at least one image is required")
	}

	// Upload all images
	var uploads []*UploadedFile
	for i, imgData := range images {
		filename := fmt.Sprintf("image_%d.png", i)
		uploaded, err := c.UploadFile(ctx, imgData, filename, detectMIME(imgData))
		if err != nil {
			return nil, fmt.Errorf("upload image %d: %w", i, err)
		}
		uploads = append(uploads, uploaded)
	}

	// Upload mask if provided
	var maskUpload *UploadedFile
	if mask != nil {
		var err error
		maskUpload, err = c.UploadFile(ctx, mask, "mask.png", detectMIME(mask))
		if err != nil {
			return nil, fmt.Errorf("upload mask: %w", err)
		}
	}

	fullPrompt := prompt
	if strings.TrimSpace(size) != "" && size != "auto" && size != "1024x1024" {
		fullPrompt = fmt.Sprintf("Edit and output the image with size %s. %s", size, prompt)
	}
	if quality == "hd" || quality == "high" {
		fullPrompt = fmt.Sprintf("Generate a high-quality, detailed edited image: %s", fullPrompt)
	}

	body := c.buildMultimodalBody(fullPrompt, model, uploads, maskUpload)
	return c.doConversation(ctx, body)
}

func (c *ChatGPTClient) InpaintImageByMask(
	ctx context.Context,
	prompt string,
	model string,
	originalFileID string,
	originalGenID string,
	conversationID string,
	parentMessageID string,
	mask []byte,
) ([]ImageResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if strings.TrimSpace(originalFileID) == "" {
		return nil, fmt.Errorf("original file id is required")
	}
	if mask == nil || len(mask) == 0 {
		return nil, fmt.Errorf("mask is required")
	}

	maskUpload, err := c.UploadMaskForInpainting(ctx, mask, "mask.png")
	if err != nil {
		return nil, fmt.Errorf("upload mask for inpainting: %w", err)
	}

	dalleOp := map[string]any{
		"type":             "inpainting",
		"original_file_id": originalFileID,
		"mask_file_id":     maskUpload.FileID,
	}
	if strings.TrimSpace(originalGenID) != "" {
		dalleOp["original_gen_id"] = originalGenID
	}

	body := c.buildConversationBody(prompt, model, conversationID, parentMessageID, dalleOp)
	body["client_prepare_state"] = "sent"
	body["supported_encodings"] = []string{"v1"}
	return c.doConversation(ctx, body)
}

// detectMIME sniffs the MIME type from image bytes.
func detectMIME(data []byte) string {
	if len(data) >= 8 {
		// PNG: 89 50 4E 47
		if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
			return "image/png"
		}
		// JPEG: FF D8 FF
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			return "image/jpeg"
		}
		// WebP: RIFF....WEBP
		if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
			len(data) >= 12 && data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
			return "image/webp"
		}
	}
	return "image/png"
}

func (c *ChatGPTClient) buildMultimodalBody(prompt, model string, uploads []*UploadedFile, maskUpload *UploadedFile) map[string]any {
	msgID := uuid.NewString()
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultUpstreamModel
	}

	// Build parts: text prompt + image asset pointers
	parts := []any{prompt}
	attachments := []any{}

	for i, up := range uploads {
		imgPart := map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": "file-service://" + up.FileID,
			"size_bytes":    up.SizeBytes,
			"mime_type":     up.MIMEType,
		}
		if up.Width > 0 && up.Height > 0 {
			imgPart["width"] = up.Width
			imgPart["height"] = up.Height
		}
		parts = append(parts, imgPart)

		name := fmt.Sprintf("image_%d.png", i)
		attachments = append(attachments, map[string]any{
			"id":       up.FileID,
			"name":     name,
			"size":     up.SizeBytes,
			"mimeType": up.MIMEType,
			"width":    up.Width,
			"height":   up.Height,
		})
	}

	if maskUpload != nil {
		maskPart := map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": "file-service://" + maskUpload.FileID,
			"size_bytes":    maskUpload.SizeBytes,
			"mime_type":     maskUpload.MIMEType,
		}
		if maskUpload.Width > 0 && maskUpload.Height > 0 {
			maskPart["width"] = maskUpload.Width
			maskPart["height"] = maskUpload.Height
		}
		parts = append(parts, maskPart)

		attachments = append(attachments, map[string]any{
			"id":       maskUpload.FileID,
			"name":     "mask.png",
			"size":     maskUpload.SizeBytes,
			"mimeType": maskUpload.MIMEType,
			"width":    maskUpload.Width,
			"height":   maskUpload.Height,
		})
	}

	metadata := map[string]any{
		"attachments":  attachments,
		"system_hints": []string{"picture_v2"},
		"serialization_metadata": map[string]any{
			"custom_symbol_offsets": []any{},
		},
	}

	msg := map[string]any{
		"id":     msgID,
		"author": map[string]any{"role": "user"},
		"content": map[string]any{
			"content_type": "multimodal_text",
			"parts":        parts,
		},
		"metadata": metadata,
	}

	return map[string]any{
		"action":                   "next",
		"messages":                 []any{msg},
		"parent_message_id":        "client-created-root",
		"model":                    model,
		"timezone_offset_min":      420,
		"timezone":                 "America/Los_Angeles",
		"conversation_mode":        map[string]any{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":             []string{"picture_v2"},
		"supports_buffering":       true,
		"supported_encodings":      []string{},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      true,
			"time_since_loaded": 1000,
			"page_height":       717,
			"page_width":        1200,
			"pixel_ratio":       2,
			"screen_height":     878,
			"screen_width":      1352,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}
}

func (c *ChatGPTClient) buildConversationBody(prompt, model, conversationID, parentMsgID string, dalleOp map[string]any) map[string]any {
	msgID := uuid.NewString()
	if parentMsgID == "" {
		parentMsgID = "client-created-root"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultUpstreamModel
	}

	metadata := map[string]any{
		"system_hints": []string{"picture_v2"},
		"serialization_metadata": map[string]any{
			"custom_symbol_offsets": []any{},
		},
	}
	if dalleOp != nil {
		metadata["dalle"] = map[string]any{
			"from_client": map[string]any{
				"operation": dalleOp,
			},
		}
	}

	msg := map[string]any{
		"id":     msgID,
		"author": map[string]any{"role": "user"},
		"content": map[string]any{
			"content_type": "text",
			"parts":        []string{prompt},
		},
		"metadata": metadata,
	}

	body := map[string]any{
		"action":                   "next",
		"messages":                 []any{msg},
		"parent_message_id":        parentMsgID,
		"model":                    model,
		"timezone_offset_min":      420,
		"timezone":                 "America/Los_Angeles",
		"conversation_mode":        map[string]any{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":             []string{"picture_v2"},
		"supports_buffering":       true,
		"supported_encodings":      []string{},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      true,
			"time_since_loaded": 1000,
			"page_height":       717,
			"page_width":        1200,
			"pixel_ratio":       2,
			"screen_height":     878,
			"screen_width":      1352,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}

	if conversationID != "" {
		body["conversation_id"] = conversationID
	}

	return body
}

func (c *ChatGPTClient) doConversation(ctx context.Context, body map[string]any) ([]ImageResult, error) {
	return c.doConversationRequest(ctx, body, "/conversation", "conversation")
}

func (c *ChatGPTClient) doFConversation(ctx context.Context, body map[string]any) ([]ImageResult, error) {
	return c.doConversationRequest(ctx, body, "/f/conversation", "f conversation")
}

func (c *ChatGPTClient) doConversationRequest(ctx context.Context, body map[string]any, path, routeLabel string) ([]ImageResult, error) {
	requestContext := extractConversationRequestContext(body)

	// Step 1: Get sentinel chat-requirements token + PoW challenge
	chatToken, proofToken, err := c.getSentinelTokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("sentinel tokens: %w", err)
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	resp, err := c.doRequestWithRetry(ctx, c.streamClient, routeLabel+" request", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("openai-sentinel-chat-requirements-token", chatToken)
		if proofToken != "" {
			req.Header.Set("openai-sentinel-proof-token", proofToken)
		}
		return req, nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s request: %w", routeLabel, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("%s returned %d: %s", routeLabel, resp.StatusCode, string(respBody))
	}

	return c.parseSSE(ctx, resp.Body, requestContext)
}

// getSentinelTokens fetches the chat-requirements token and solves PoW if needed.
func (c *ChatGPTClient) getSentinelTokens(ctx context.Context) (chatToken, proofToken string, err error) {
	reqToken := generateRequirementsTokenForUserAgent(c.userAgent)

	reqBody, _ := json.Marshal(map[string]string{"p": reqToken})
	resp, err := c.doRequestWithRetry(ctx, c.httpClient, "chat-requirements request", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/sentinel/chat-requirements", bytes.NewReader(reqBody))
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		return req, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("chat-requirements request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", "", fmt.Errorf("chat-requirements returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token       string `json:"token"`
		ProofOfWork struct {
			Required   bool   `json:"required"`
			Seed       string `json:"seed"`
			Difficulty string `json:"difficulty"`
		} `json:"proofofwork"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("decode chat-requirements: %w", err)
	}

	chatToken = result.Token
	log.Printf("[sentinel] got chat-requirements token, pow_required=%v", result.ProofOfWork.Required)

	if result.ProofOfWork.Required {
		log.Printf("[sentinel] solving PoW: seed=%s difficulty=%s", result.ProofOfWork.Seed, result.ProofOfWork.Difficulty)
		proofToken, err = solvePoWForUserAgent(result.ProofOfWork.Seed, result.ProofOfWork.Difficulty, c.userAgent)
		if err != nil {
			return "", "", fmt.Errorf("solve PoW: %w", err)
		}
		log.Printf("[sentinel] PoW solved, token prefix: %s...", proofToken[:20])
	}

	return chatToken, proofToken, nil
}

func (c *ChatGPTClient) parseSSE(ctx context.Context, reader io.Reader, requestContext conversationRequestContext) ([]ImageResult, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var (
		conversationID string
		asyncMode      bool
		images         []ImageResult
	)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		if !strings.HasPrefix(data, "{") {
			continue
		}

		// Parse as generic JSON first to handle all event types
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			continue
		}

		// Extract conversation_id from any event that has it
		if rawCID, ok := raw["conversation_id"]; ok {
			var cid string
			if json.Unmarshal(rawCID, &cid) == nil && cid != "" {
				conversationID = cid
			}
		}

		// Detect async image generation
		if rawAS, ok := raw["async_status"]; ok {
			var status int
			if json.Unmarshal(rawAS, &status) == nil && status > 0 {
				asyncMode = true
				log.Printf("[sse] async_status=%d, will poll after stream ends", status)
			}
		}

		// Try to parse as a message event (old format: {"message": {...}, "conversation_id": "..."})
		var event sseEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Message == nil {
			continue
		}

		msg := event.Message
		if requestContext.ParentMessageID != "" && msg.ID == requestContext.ParentMessageID {
			continue
		}
		// Extract images from multimodal_text parts (sync case)
		images = append(images, c.extractImages(ctx, msg, conversationID)...)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("SSE read error: %w", err)
	}
	if strings.TrimSpace(conversationID) == "" {
		conversationID = strings.TrimSpace(requestContext.ConversationID)
	}

	// If images were found inline, return immediately
	if len(images) > 0 {
		return images, nil
	}

	// If async mode, poll the conversation until images appear
	if asyncMode && conversationID != "" {
		log.Printf("[poll] image generation is async, polling conversation %s", conversationID)
		return c.pollForImages(ctx, conversationID, requestContext.SubmittedMessageID)
	}

	return nil, fmt.Errorf("no images generated — the model may have refused the request")
}

// extractImages extracts image results from a single SSE message.
func (c *ChatGPTClient) extractImages(ctx context.Context, msg *sseMessage, conversationID string) []ImageResult {
	// Images can appear in "assistant" or "tool" messages, never in "user" or "system"
	if msg.Author.Role == "user" || msg.Author.Role == "system" {
		return nil
	}
	if msg.Content.ContentType != "multimodal_text" {
		return nil
	}
	if msg.Status != "finished_successfully" {
		return nil
	}

	var images []ImageResult
	for _, rawPart := range msg.Content.Parts {
		var part sseImagePart
		if err := json.Unmarshal(rawPart, &part); err != nil {
			continue
		}
		if part.ContentType != "image_asset_pointer" || part.AssetPointer == "" {
			continue
		}

		fileID := extractFileID(part.AssetPointer)
		if fileID == "" {
			continue
		}

		log.Printf("[extract] found image: pointer=%s gen_id=%s", part.AssetPointer, part.Metadata.Dalle.GenID)

		// sediment:// uses attachment API, file-service:// uses files API
		var downloadURL string
		var err error
		if strings.HasPrefix(part.AssetPointer, "sediment://") {
			downloadURL, err = c.getAttachmentURL(ctx, fileID, conversationID)
		} else {
			downloadURL, err = c.getDownloadURL(ctx, fileID, conversationID)
		}
		if err != nil {
			log.Printf("warning: failed to get download URL for %s: %v", fileID, err)
			continue
		}

		images = append(images, ImageResult{
			URL:            downloadURL,
			FileID:         fileID,
			GenID:          part.Metadata.Dalle.GenID,
			ConversationID: conversationID,
			ParentMsgID:    msg.ID,
			RevisedPrompt:  part.Metadata.Dalle.Prompt,
		})
	}
	return images
}

// pollForImages polls GET /backend-api/conversation/{id} until image results appear.
func (c *ChatGPTClient) pollForImages(ctx context.Context, conversationID, rootMessageID string) ([]ImageResult, error) {
	deadline := time.Now().Add(c.pollMaxWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.pollInterval):
		}

		images, err := c.fetchConversationImages(ctx, conversationID, rootMessageID)
		if err != nil {
			log.Printf("[poll] error fetching conversation: %v", err)
			continue
		}
		if len(images) > 0 {
			return images, nil
		}
		log.Printf("[poll] still waiting for images...")
	}
	return nil, fmt.Errorf("timed out waiting for async image generation")
}

// fetchConversationImages fetches the full conversation and extracts any image results.
func (c *ChatGPTClient) fetchConversationImages(ctx context.Context, conversationID, rootMessageID string) ([]ImageResult, error) {
	url := fmt.Sprintf("%s/conversation/%s", baseURL, conversationID)
	resp, err := c.doRequestWithRetry(ctx, c.httpClient, "GET conversation request", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GET conversation returned %d: %s", resp.StatusCode, string(body))
	}

	var conv struct {
		Mapping map[string]struct {
			Message  *sseMessage `json:"message"`
			Parent   string      `json:"parent"`
			Children []string    `json:"children"`
		} `json:"mapping"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&conv); err != nil {
		return nil, fmt.Errorf("decode conversation: %w", err)
	}

	var images []ImageResult
	seen := map[string]struct{}{}
	var visit func(string)
	visit = func(nodeID string) {
		if nodeID == "" {
			return
		}
		if _, ok := seen[nodeID]; ok {
			return
		}
		node, ok := conv.Mapping[nodeID]
		if !ok {
			return
		}
		seen[nodeID] = struct{}{}
		if node.Message != nil {
			log.Printf("[poll] message role=%s status=%s content_type=%s parts=%d",
				node.Message.Author.Role, node.Message.Status,
				node.Message.Content.ContentType, len(node.Message.Content.Parts))
			images = append(images, c.extractImages(ctx, node.Message, conversationID)...)
		}
		for _, childID := range node.Children {
			visit(childID)
		}
	}

	if rootMessageID != "" {
		visit(rootMessageID)
		return images, nil
	}

	for nodeID := range conv.Mapping {
		visit(nodeID)
	}

	return images, nil
}

// getAttachmentURL fetches the download URL for sediment:// assets via the attachment API.
func (c *ChatGPTClient) getAttachmentURL(ctx context.Context, fileID, conversationID string) (string, error) {
	url := fmt.Sprintf("%s/conversation/%s/attachment/%s/download", baseURL, conversationID, fileID)
	resp, err := c.doRequestWithRetry(ctx, c.httpClient, "attachment download request", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		DownloadURL string `json:"download_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.DownloadURL == "" {
		return "", fmt.Errorf("empty download_url for attachment %s", fileID)
	}
	return result.DownloadURL, nil
}

func (c *ChatGPTClient) getDownloadURL(ctx context.Context, fileID, conversationID string) (string, error) {
	url := fmt.Sprintf("%s/files/download/%s?conversation_id=%s&inline=false", baseURL, fileID, conversationID)
	resp, err := c.doRequestWithRetry(ctx, c.httpClient, "file download url request", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		c.setHeaders(req)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		DownloadURL string `json:"download_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.DownloadURL == "" {
		return "", fmt.Errorf("empty download_url for file %s", fileID)
	}
	return result.DownloadURL, nil
}

func (c *ChatGPTClient) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OAI-Device-Id", c.oaiDeviceID)
	if strings.TrimSpace(c.oaiSessionID) != "" {
		req.Header.Set("OAI-Session-Id", c.oaiSessionID)
	}
	req.Header.Set("OAI-Language", "zh-CN")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("Sec-CH-UA", c.secCHUA)
	req.Header.Set("Sec-CH-UA-Mobile", c.secCHUAMobile)
	req.Header.Set("Sec-CH-UA-Platform", c.secCHUAPlatform)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("User-Agent", c.userAgent)
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}
}

func (c *ChatGPTClient) LastRoute() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.lastImageRoute)
}

func (c *ChatGPTClient) setLastImageRoute(route string) {
	if c == nil {
		return
	}
	c.lastImageRoute = strings.TrimSpace(route)
}

func cloneConversationBody(body map[string]any) map[string]any {
	if len(body) == 0 {
		return map[string]any{}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return map[string]any{}
	}
	cloned := map[string]any{}
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}

func shouldFallbackFromFConversation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	return strings.Contains(message, "f conversation request:") ||
		strings.Contains(message, "f conversation returned 5")
}

func extractFileID(pointer string) string {
	for _, prefix := range []string{"file-service://", "sediment://"} {
		if strings.HasPrefix(pointer, prefix) {
			return strings.TrimPrefix(pointer, prefix)
		}
	}
	return ""
}

type conversationRequestContext struct {
	ConversationID     string
	SubmittedMessageID string
	ParentMessageID    string
}

func extractConversationRequestContext(body map[string]any) conversationRequestContext {
	requestContext := conversationRequestContext{
		ConversationID:  strings.TrimSpace(stringValue(body["conversation_id"])),
		ParentMessageID: strings.TrimSpace(stringValue(body["parent_message_id"])),
	}

	rawMessages, ok := body["messages"].([]any)
	if !ok || len(rawMessages) == 0 {
		return requestContext
	}
	firstMessage, ok := rawMessages[0].(map[string]any)
	if !ok {
		return requestContext
	}
	requestContext.SubmittedMessageID = strings.TrimSpace(stringValue(firstMessage["id"]))
	return requestContext
}

// SSE types

type sseEvent struct {
	ConversationID string      `json:"conversation_id"`
	Message        *sseMessage `json:"message"`
}

type sseMessage struct {
	ID     string `json:"id"`
	Author struct {
		Role string `json:"role"`
	} `json:"author"`
	Status  string `json:"status"`
	Content struct {
		ContentType string            `json:"content_type"`
		Parts       []json.RawMessage `json:"parts"`
	} `json:"content"`
}

type sseImagePart struct {
	ContentType  string `json:"content_type"`
	AssetPointer string `json:"asset_pointer"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Metadata     struct {
		Dalle struct {
			GenID  string `json:"gen_id"`
			Prompt string `json:"prompt"`
		} `json:"dalle"`
	} `json:"metadata"`
}
