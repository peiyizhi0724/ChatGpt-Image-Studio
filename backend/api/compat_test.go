package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/config"
	"chatgpt2api/internal/imagehistory"
)

func TestExtractCompatPromptAndImagesFromMessages(t *testing.T) {
	messages := []compatChatMessage{
		{
			Role: "system",
			Content: []any{
				map[string]any{"type": "text", "text": "系统提示"},
			},
		},
		{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "画一只橘猫"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,abc"}},
			},
		},
		{
			Role:    "assistant",
			Content: "这条不应该参与提取",
		},
	}

	prompt, images := extractCompatPromptAndImagesFromMessages(messages)
	if prompt != "系统提示\n\n画一只橘猫" {
		t.Fatalf("prompt = %q, want %q", prompt, "系统提示\n\n画一只橘猫")
	}
	if len(images) != 1 || images[0] != "data:image/png;base64,abc" {
		t.Fatalf("images = %#v, want one data url image", images)
	}
}

func TestExtractCompatPromptAndImagesFromResponsesInput(t *testing.T) {
	input := []any{
		map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "input_text", "text": "这段历史 assistant 文本不应该进入新 prompt"},
			},
		},
		map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "请生成夜景海报"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64,xyz"},
			},
		},
		map[string]any{
			"role": "tool",
			"content": []any{
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64,tool-image"},
			},
		},
	}

	prompt, images := extractCompatPromptAndImages(input)
	if prompt != "请生成夜景海报" {
		t.Fatalf("prompt = %q, want %q", prompt, "请生成夜景海报")
	}
	if len(images) != 1 || images[0] != "data:image/png;base64,xyz" {
		t.Fatalf("images = %#v, want one data url image", images)
	}
}

func TestResolveCompatRemoteURLRejectsNonSameOriginLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", nil)
	req.Host = "example.com"

	if _, err := resolveCompatRemoteURL("http://127.0.0.1:7000/v1/files/image/test.png", req); err == nil {
		t.Fatal("expected loopback url to be rejected when request host is different")
	}
}

func TestResolveCompatRemoteURLAllowsSameOriginAbsoluteURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", nil)
	req.Host = "example.com"

	got, err := resolveCompatRemoteURL("http://example.com/v1/files/image/test.png", req)
	if err != nil {
		t.Fatalf("resolveCompatRemoteURL returned error: %v", err)
	}
	if got != "http://example.com/v1/files/image/test.png" {
		t.Fatalf("resolveCompatRemoteURL = %q, want same url", got)
	}
}

func TestHandleImageResponsesReturns400ForInvalidImageInput(t *testing.T) {
	server := &Server{cfg: &config.Config{}}
	body := `{
		"model":"gpt-image-2",
		"input":[
			{
				"role":"user",
				"content":[
					{"type":"input_text","text":"请编辑这张图"},
					{"type":"input_image","image_url":"http://127.0.0.1:7000/secret"}
				]
			}
		],
		"tools":[{"type":"image_generation"}]
	}`

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/responses", strings.NewReader(body))
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	server.handleImageResponses(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid_image_input") {
		t.Fatalf("body = %s, want invalid_image_input code", rec.Body.String())
	}
}

func TestNormalizeThirdPartyImageResponseFormatForThirdPartyKey(t *testing.T) {
	server := &Server{
		cfg: &config.Config{
			App: config.AppConfig{
				APIKey:  "third-party-key",
				AuthKey: "ui-key",
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	req.Header.Set("Authorization", "Bearer third-party-key")
	if got := server.normalizeThirdPartyImageResponseFormat(req, ""); got != "b64_json" {
		t.Fatalf("normalizeThirdPartyImageResponseFormat(empty) = %q, want %q", got, "b64_json")
	}
	if got := server.normalizeThirdPartyImageResponseFormat(req, "url"); got != "b64_json" {
		t.Fatalf("normalizeThirdPartyImageResponseFormat(url) = %q, want %q", got, "b64_json")
	}
	if got := server.normalizeThirdPartyImageResponseFormat(req, "b64_json"); got != "b64_json" {
		t.Fatalf("normalizeThirdPartyImageResponseFormat(b64_json) = %q, want %q", got, "b64_json")
	}

	uiReq := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	uiReq.Header.Set("Authorization", "Bearer ui-key")
	if got := server.normalizeThirdPartyImageResponseFormat(uiReq, ""); got != "" {
		t.Fatalf("normalizeThirdPartyImageResponseFormat(ui empty) = %q, want empty string", got)
	}
	if got := server.normalizeThirdPartyImageResponseFormat(uiReq, "url"); got != "url" {
		t.Fatalf("normalizeThirdPartyImageResponseFormat(ui url) = %q, want %q", got, "url")
	}
}

func TestHandleImageEditsSelectionEditRequiresSourceAccountID(t *testing.T) {
	server, _ := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Plus",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	req := newCompatMultipartRequest(t, "/v1/images/edits", map[string]string{
		"prompt":            "selection prompt",
		"model":             "gpt-image-2",
		"response_format":   "b64_json",
		"original_file_id":  "file-1",
		"original_gen_id":   "gen-1",
		"conversation_id":   "conv-1",
		"parent_message_id": "msg-1",
	}, map[string][][]byte{
		"mask": {[]byte("selection-mask")},
	}, server.cfg.App.APIKey)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "source_account_id is required") {
		t.Fatalf("body = %s, want missing source_account_id message", rec.Body.String())
	}
}

func TestImageGenerationPreservesPaidResolutionErrorCode(t *testing.T) {
	server := newCompatFreeOnlyStudioServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{
		"prompt":"test prompt",
		"size":"3840x2160",
		"quality":"high",
		"response_format":"b64_json"
	}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error.Code != "paid_resolution_requires_paid_account" {
		t.Fatalf("error code = %q, want %q", payload.Error.Code, "paid_resolution_requires_paid_account")
	}
}

func newCompatFreeOnlyStudioServer(t *testing.T) *Server {
	t.Helper()

	rootDir := t.TempDir()
	cfg := config.New(rootDir)
	if err := cfg.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.App.APIKey = "test-image-key"
	cfg.App.AuthKey = "test-ui-key"
	cfg.App.ImageFormat = "b64_json"
	cfg.ChatGPT.Model = "gpt-image-2"
	cfg.ChatGPT.ImageMode = "studio"
	cfg.ChatGPT.FreeImageRoute = "responses"
	cfg.ChatGPT.FreeImageModel = "auto"
	cfg.ChatGPT.PaidImageRoute = "responses"
	cfg.ChatGPT.PaidImageModel = "gpt-5.4-mini"

	authDir := cfg.ResolvePath(cfg.Storage.AuthDir)
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("create auth dir: %v", err)
	}
	statePath := cfg.ResolvePath(cfg.Storage.StateFile)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	if err := writeCompatTestJSON(filepath.Join(authDir, "free.json"), map[string]any{
		"type":         "codex",
		"access_token": "token-free",
		"email":        "free@example.com",
		"priority":     0,
	}); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	if err := writeCompatTestJSON(statePath, map[string]any{
		"accounts": map[string]any{
			"free.json": map[string]any{
				"type":        "Free",
				"status":      "正常",
				"quota":       5,
				"quota_known": true,
				"priority":    0,
				"limits_progress": []map[string]any{
					{
						"feature_name": "image_gen",
						"remaining":    5,
						"reset_after":  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	store, err := accounts.NewStore(cfg)
	if err != nil {
		t.Fatalf("new account store: %v", err)
	}
	return NewServer(cfg, store, nil, nil)
}

func writeCompatTestJSON(path string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func TestBuildCompatResponsesResponse(t *testing.T) {
	payload := map[string]any{
		"created": int64(1710000000),
		"data": []map[string]any{
			{
				"b64_json":       "ZmFrZQ==",
				"revised_prompt": "修订提示词",
			},
		},
	}

	resp := buildCompatResponsesResponse("gpt-5", payload)
	if got := stringValue(resp["object"]); got != "response" {
		t.Fatalf("object = %q, want %q", got, "response")
	}
	if got := stringValue(resp["model"]); got != "gpt-5" {
		t.Fatalf("model = %q, want %q", got, "gpt-5")
	}
	output, ok := resp["output"].([]map[string]any)
	if !ok {
		t.Fatalf("output type = %T, want []map[string]any", resp["output"])
	}
	if len(output) != 1 {
		t.Fatalf("len(output) = %d, want 1", len(output))
	}
	if got := stringValue(output[0]["type"]); got != "image_generation_call" {
		t.Fatalf("output[0].type = %q, want %q", got, "image_generation_call")
	}
	if got := stringValue(output[0]["result"]); got != "ZmFrZQ==" {
		t.Fatalf("output[0].result = %q, want %q", got, "ZmFrZQ==")
	}
}

func TestCompatTaskPayloadKeepsPartialSuccess(t *testing.T) {
	payload, err := compatTaskPayload(&imageTaskView{
		ID:        "compat-task-1",
		Status:    imageTaskStatusFailed,
		CreatedAt: "2026-04-27T10:00:00Z",
		Images: []imagehistory.Image{
			{
				ID:              "img-ok",
				Status:          "success",
				URL:             "/v1/files/image/success.png",
				RevisedPrompt:   "ok",
				SourceAccountID: "account-1",
			},
			{
				ID:     "img-fail",
				Status: "error",
				Error:  "temporary upstream failure",
			},
		},
		Error: "temporary upstream failure",
	})
	if err != nil {
		t.Fatalf("compatTaskPayload() returned error: %v", err)
	}

	data, ok := payload["data"].([]map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want []map[string]any", payload["data"])
	}
	if len(data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(data))
	}
	if got := stringValue(data[0]["url"]); got != "/v1/files/image/success.png" {
		t.Fatalf("data[0].url = %q, want cached file url", got)
	}

	taskErrors, ok := payload["errors"].([]map[string]any)
	if !ok {
		t.Fatalf("errors type = %T, want []map[string]any", payload["errors"])
	}
	if len(taskErrors) != 1 {
		t.Fatalf("len(errors) = %d, want 1", len(taskErrors))
	}
	if got := stringValue(taskErrors[0]["error"]); got != "temporary upstream failure" {
		t.Fatalf("errors[0].error = %q, want propagated failure", got)
	}
}

func TestCompatTaskPayloadReturnsErrorWhenAllUnitsFail(t *testing.T) {
	_, err := compatTaskPayload(&imageTaskView{
		ID:        "compat-task-2",
		Status:    imageTaskStatusFailed,
		CreatedAt: "2026-04-27T10:00:00Z",
		Images: []imagehistory.Image{
			{
				ID:     "img-fail",
				Status: "error",
				Error:  "all failed",
			},
		},
		Error: "all failed",
	})
	if err == nil {
		t.Fatal("compatTaskPayload() error = nil, want failure")
	}
	if err.Error() != "all failed" {
		t.Fatalf("compatTaskPayload() error = %q, want propagated task error", err.Error())
	}
}

func TestNormalizeCompatRequestedModel(t *testing.T) {
	if got := normalizeCompatRequestedModel("gpt-image-1", "gpt-image-2"); got != "gpt-image-1" {
		t.Fatalf("normalizeCompatRequestedModel(gpt-image-1) = %q", got)
	}
	if got := normalizeCompatRequestedModel("gpt-5", "gpt-image-2"); got != "gpt-image-2" {
		t.Fatalf("normalizeCompatRequestedModel(gpt-5) = %q, want %q", got, "gpt-image-2")
	}
}
