package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCPAImageClientGenerateImageUsesCodexResponsesStrategy(t *testing.T) {
	var seenAuth string
	var seenPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/responses")
		}
		seenAuth = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &seenPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		encoded := base64.StdEncoding.EncodeToString([]byte("image"))
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"created_at":1,"output":[{"type":"image_generation_call","result":"`+encoded+`","revised_prompt":"revised","output_format":"png"}]}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := newCPAImageClient(server.URL, "test-key", 30*time.Second, "codex_responses")
	results, err := client.GenerateImage(context.Background(), "draw a cat", cpaFixedImageModel, 1, "1536x1024", "high", "transparent")
	if err != nil {
		t.Fatalf("GenerateImage() returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if !strings.HasPrefix(results[0].URL, "data:image/png;base64,") {
		t.Fatalf("result URL = %q, want data URL", results[0].URL)
	}
	if got := client.LastRoute(); got != "codex_responses" {
		t.Fatalf("LastRoute() = %q, want %q", got, "codex_responses")
	}
	if got := client.LastModelLabel(); got != "gpt-5.4-mini (tool: gpt-image-2)" {
		t.Fatalf("LastModelLabel() = %q, want %q", got, "gpt-5.4-mini (tool: gpt-image-2)")
	}
	if got := client.ImageToolModel(); got != cpaFixedImageModel {
		t.Fatalf("ImageToolModel() = %q, want %q", got, cpaFixedImageModel)
	}
	if seenAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want bearer key", seenAuth)
	}
	if got := seenPayload["model"]; got != "gpt-5.4-mini" {
		t.Fatalf("payload model = %v, want %q", got, "gpt-5.4-mini")
	}
	tools, ok := seenPayload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("payload tools = %#v, want one tool", seenPayload["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tool = %#v, want object", tools[0])
	}
	if got := tool["model"]; got != cpaFixedImageModel {
		t.Fatalf("tool.model = %v, want %q", got, cpaFixedImageModel)
	}
	if got := tool["action"]; got != "generate" {
		t.Fatalf("tool.action = %v, want %q", got, "generate")
	}
	if got := tool["size"]; got != "1536x1024" {
		t.Fatalf("tool.size = %v, want %q", got, "1536x1024")
	}
	if got := tool["quality"]; got != "high" {
		t.Fatalf("tool.quality = %v, want %q", got, "high")
	}
	if got := tool["background"]; got != "transparent" {
		t.Fatalf("tool.background = %v, want %q", got, "transparent")
	}
}

func TestCPAImageClientEditUsesCodexResponsesMaskField(t *testing.T) {
	var seenTool map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/responses")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		tools := payload["tools"].([]any)
		seenTool = tools[0].(map[string]any)

		w.Header().Set("Content-Type", "text/event-stream")
		encoded := base64.StdEncoding.EncodeToString([]byte("image"))
		_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"created_at":1,"output":[{"type":"image_generation_call","result":"`+encoded+`","output_format":"png"}]}}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := newCPAImageClient(server.URL, "test-key", 30*time.Second, "codex_responses")
	_, err := client.EditImageByUpload(context.Background(), "edit cat", cpaFixedImageModel, [][]byte{[]byte("source-image")}, []byte("mask-image"), "1536x1024", "high")
	if err != nil {
		t.Fatalf("EditImageByUpload() returned error: %v", err)
	}
	if got := seenTool["action"]; got != "edit" {
		t.Fatalf("tool.action = %v, want %q", got, "edit")
	}
	if got := seenTool["size"]; got != "1536x1024" {
		t.Fatalf("tool.size = %v, want %q", got, "1536x1024")
	}
	if got := seenTool["quality"]; got != "high" {
		t.Fatalf("tool.quality = %v, want %q", got, "high")
	}
	maskField, ok := seenTool["input_image_mask"].(map[string]any)
	if !ok {
		t.Fatalf("tool.input_image_mask = %#v, want object", seenTool["input_image_mask"])
	}
	if _, ok := maskField["image_url"].(string); !ok {
		t.Fatalf("tool.input_image_mask.image_url missing: %#v", seenTool)
	}
}

func TestCPAImageClientEditUsesImagesAPIMultipartImageMIMETypes(t *testing.T) {
	var seenImageContentType string
	var seenMaskContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/images/edits")
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader() returned error: %v", err)
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart() returned error: %v", err)
			}
			switch part.FormName() {
			case "image":
				seenImageContentType = part.Header.Get("Content-Type")
			case "mask":
				seenMaskContentType = part.Header.Get("Content-Type")
			}
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"aW1hZ2U="}]}`)
	}))
	defer server.Close()

	client := newCPAImageClient(server.URL, "test-key", 30*time.Second, "images_api")
	jpegImage := []byte{0xFF, 0xD8, 0xFF, 0x00}
	pngMask := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x00, 0x00, 0x00}
	_, err := client.EditImageByUpload(context.Background(), "edit cat", cpaFixedImageModel, [][]byte{jpegImage}, pngMask, "1536x1024", "high")
	if err != nil {
		t.Fatalf("EditImageByUpload() returned error: %v", err)
	}
	if seenImageContentType != "image/jpeg" {
		t.Fatalf("image Content-Type = %q, want %q", seenImageContentType, "image/jpeg")
	}
	if seenMaskContentType != "image/png" {
		t.Fatalf("mask Content-Type = %q, want %q", seenMaskContentType, "image/png")
	}
}

func TestCPAImageClientAutoFallsBackToCodexResponses(t *testing.T) {
	var imagesAPICalls int
	var responsesCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			imagesAPICalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"message":"stream disconnected before completion"}}`)
		case "/v1/responses":
			responsesCalls++
			w.Header().Set("Content-Type", "text/event-stream")
			encoded := base64.StdEncoding.EncodeToString([]byte("image"))
			_, _ = io.WriteString(w, `data: {"type":"response.completed","response":{"created_at":1,"output":[{"type":"image_generation_call","result":"`+encoded+`","output_format":"png"}]}}`+"\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newCPAImageClient(server.URL, "test-key", 30*time.Second, "auto")
	results, err := client.GenerateImage(context.Background(), "draw a cat", cpaFixedImageModel, 1, "1024x1024", "", "")
	if err != nil {
		t.Fatalf("GenerateImage() returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if imagesAPICalls != 1 || responsesCalls != 1 {
		t.Fatalf("images_api calls = %d, responses calls = %d, want 1/1", imagesAPICalls, responsesCalls)
	}
	if got := client.LastRoute(); got != "codex_responses" {
		t.Fatalf("LastRoute() = %q, want %q", got, "codex_responses")
	}
}
