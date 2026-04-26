package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatgpt2api/internal/config"
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

func TestNormalizeCompatRequestedModel(t *testing.T) {
	if got := normalizeCompatRequestedModel("gpt-image-1", "gpt-image-2"); got != "gpt-image-1" {
		t.Fatalf("normalizeCompatRequestedModel(gpt-image-1) = %q", got)
	}
	if got := normalizeCompatRequestedModel("gpt-5", "gpt-image-2"); got != "gpt-image-2" {
		t.Fatalf("normalizeCompatRequestedModel(gpt-5) = %q, want %q", got, "gpt-image-2")
	}
}
