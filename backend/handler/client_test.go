package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestParseSSENoAsyncDoesNotPoll(t *testing.T) {
	client := &ChatGPTClient{
		accessToken: "token",
		oaiDeviceID: "device",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				t.Fatalf("unexpected polling request: %s", req.URL.String())
				return nil, nil
			}),
		},
	}

	stream := strings.Join([]string{
		`data: {"conversation_id":"conv-1","message":{"id":"tool-1","author":{"role":"tool"},"status":"finished_successfully","content":{"content_type":"text","parts":["still working"]}}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	_, err := client.parseSSE(context.Background(), strings.NewReader(stream), conversationRequestContext{
		ConversationID:     "conv-1",
		SubmittedMessageID: "user-1",
	})
	if err == nil {
		t.Fatal("expected parseSSE to fail without images")
	}
	if !strings.Contains(err.Error(), "no images generated") {
		t.Fatalf("expected no-images error, got %v", err)
	}
}

func TestFetchConversationImagesRestrictsToSubmittedBranch(t *testing.T) {
	conversationJSON := `{
		"mapping": {
			"old-user": {
				"message": {
					"id": "old-user",
					"author": {"role": "user"},
					"status": "finished_successfully",
					"content": {"content_type": "text", "parts": ["old prompt"]}
				},
				"children": ["old-tool"]
			},
			"old-tool": {
				"parent": "old-user",
				"message": {
					"id": "old-tool",
					"author": {"role": "tool"},
					"status": "finished_successfully",
					"content": {
						"content_type": "multimodal_text",
						"parts": [
							{
								"content_type": "image_asset_pointer",
								"asset_pointer": "sediment://file-old",
								"metadata": {"dalle": {"gen_id": "gen-old", "prompt": "old prompt"}}
							}
						]
					}
				}
			},
			"new-user": {
				"message": {
					"id": "new-user",
					"author": {"role": "user"},
					"status": "finished_successfully",
					"content": {"content_type": "text", "parts": ["new prompt"]}
				},
				"children": ["new-tool"]
			},
			"new-tool": {
				"parent": "new-user",
				"message": {
					"id": "new-tool",
					"author": {"role": "tool"},
					"status": "finished_successfully",
					"content": {
						"content_type": "multimodal_text",
						"parts": [
							{
								"content_type": "image_asset_pointer",
								"asset_pointer": "sediment://file-new",
								"metadata": {"dalle": {"gen_id": "gen-new", "prompt": "new prompt"}}
							}
						]
					}
				}
			}
		}
	}`

	client := &ChatGPTClient{
		accessToken: "token",
		oaiDeviceID: "device",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/conversation/conv-1"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(conversationJSON)),
					}, nil
				case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/attachment/file-new/download"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"download_url":"https://files.example/new.png"}`)),
					}, nil
				case req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/attachment/file-old/download"):
					t.Fatalf("old branch attachment should not be requested: %s", req.URL.String())
					return nil, nil
				default:
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}
			}),
		},
	}

	images, err := client.fetchConversationImages(context.Background(), "conv-1", "new-user")
	if err != nil {
		t.Fatalf("fetchConversationImages returned error: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected exactly one image, got %d", len(images))
	}
	if images[0].FileID != "file-new" {
		t.Fatalf("expected file-new, got %s", images[0].FileID)
	}
	if images[0].GenID != "gen-new" {
		t.Fatalf("expected gen-new, got %s", images[0].GenID)
	}
	if images[0].ParentMsgID != "new-tool" {
		t.Fatalf("expected parent message new-tool, got %s", images[0].ParentMsgID)
	}
}

func TestResolveImageUpstreamModel(t *testing.T) {
	tests := []struct {
		name          string
		requested     string
		accountType   string
		expectedModel string
	}{
		{name: "default request falls back to gpt image 2 behavior", requested: "", accountType: "", expectedModel: "auto"},
		{name: "gpt image 1 uses auto for free", requested: "gpt-image-1", accountType: "Free", expectedModel: "auto"},
		{name: "gpt image 1 uses gpt 5 4 mini for paid", requested: "gpt-image-1", accountType: "Plus", expectedModel: "gpt-5.4-mini"},
		{name: "gpt image 2 uses auto for free", requested: "gpt-image-2", accountType: "Free", expectedModel: "auto"},
		{name: "gpt image 2 uses auto when account type missing", requested: "gpt-image-2", accountType: "", expectedModel: "auto"},
		{name: "gpt image 2 uses gpt 5 4 mini for paid", requested: "gpt-image-2", accountType: "Pro", expectedModel: "gpt-5.4-mini"},
		{name: "explicit upstream model is preserved", requested: "gpt-5.4-mini", accountType: "Free", expectedModel: "gpt-5.4-mini"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := ResolveImageUpstreamModel(tt.requested, tt.accountType); actual != tt.expectedModel {
				t.Fatalf("expected %s, got %s", tt.expectedModel, actual)
			}
		})
	}
}

func TestBuildConversationBodyUsesProvidedModel(t *testing.T) {
	client := &ChatGPTClient{}

	body := client.buildConversationBody("draw a cat", "auto", "", "", nil)
	if got := body["model"]; got != "auto" {
		t.Fatalf("expected model auto, got %v", got)
	}

	body = client.buildConversationBody("draw a cat", "", "", "", nil)
	if got := body["model"]; got != defaultUpstreamModel {
		t.Fatalf("expected default model %s, got %v", defaultUpstreamModel, got)
	}
}

func TestGenerateImagePrefersFConversation(t *testing.T) {
	client := &ChatGPTClient{
		accessToken: "token",
		oaiDeviceID: "device",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/sentinel/chat-requirements"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"token":"sentinel","proofofwork":{"required":false}}`)),
					}, nil
				case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/attachment/file-f/download"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"download_url":"https://files.example/f.png"}`)),
					}, nil
				default:
					t.Fatalf("unexpected http request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}
			}),
		},
		streamClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost || !strings.HasSuffix(req.URL.Path, "/f/conversation") {
					t.Fatalf("unexpected stream request: %s %s", req.Method, req.URL.String())
				}
				stream := strings.Join([]string{
					`data: {"conversation_id":"conv-f","message":{"id":"tool-f","author":{"role":"tool"},"status":"finished_successfully","content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"sediment://file-f","metadata":{"dalle":{"gen_id":"gen-f","prompt":"prompt"}}}]}}}`,
					"",
					`data: [DONE]`,
					"",
				}, "\n")
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(stream)),
				}, nil
			}),
		},
	}

	images, err := client.GenerateImage(context.Background(), "draw a cat", "auto", 1, "1024x1024", "", "")
	if err != nil {
		t.Fatalf("GenerateImage returned error: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("GenerateImage len = %d, want 1", len(images))
	}
	if got := client.LastRoute(); got != "f-conversation" {
		t.Fatalf("LastRoute() = %q, want %q", got, "f-conversation")
	}
}

func TestGenerateImageFallsBackToConversationWhenFConversationFails(t *testing.T) {
	client := &ChatGPTClient{
		accessToken: "token",
		oaiDeviceID: "device",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/sentinel/chat-requirements"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"token":"sentinel","proofofwork":{"required":false}}`)),
					}, nil
				case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/attachment/file-c/download"):
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"download_url":"https://files.example/c.png"}`)),
					}, nil
				default:
					t.Fatalf("unexpected http request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}
			}),
		},
		streamClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/f/conversation"):
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`{"error":"boom"}`)),
					}, nil
				case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/conversation"):
					stream := strings.Join([]string{
						`data: {"conversation_id":"conv-c","message":{"id":"tool-c","author":{"role":"tool"},"status":"finished_successfully","content":{"content_type":"multimodal_text","parts":[{"content_type":"image_asset_pointer","asset_pointer":"sediment://file-c","metadata":{"dalle":{"gen_id":"gen-c","prompt":"prompt"}}}]}}}`,
						"",
						`data: [DONE]`,
						"",
					}, "\n")
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(stream)),
					}, nil
				default:
					t.Fatalf("unexpected stream request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}
			}),
		},
	}

	images, err := client.GenerateImage(context.Background(), "draw a cat", "auto", 1, "1024x1024", "", "")
	if err != nil {
		t.Fatalf("GenerateImage returned error: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("GenerateImage len = %d, want 1", len(images))
	}
	if got := client.LastRoute(); got != "conversation" {
		t.Fatalf("LastRoute() = %q, want %q", got, "conversation")
	}
}

func TestShouldFallbackFromFConversation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "request error falls back", err: io.ErrUnexpectedEOF, want: false},
		{name: "labeled request error falls back", err: errors.New("f conversation request: dial tcp timeout"), want: true},
		{name: "5xx response falls back", err: errors.New("f conversation returned 500: boom"), want: true},
		{name: "sse read error does not fall back", err: errors.New("SSE read error: unexpected EOF"), want: false},
		{name: "internal error marker alone does not fall back", err: errors.New("internal_error"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldFallbackFromFConversation(tt.err); got != tt.want {
				t.Fatalf("shouldFallbackFromFConversation(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestNewChatGPTClientWithProxyAndConfigUsesProvidedTimeouts(t *testing.T) {
	requestConfig := ImageRequestConfig{
		RequestTimeout: 12 * time.Second,
		SSETimeout:     90 * time.Second,
		PollInterval:   5 * time.Second,
		PollMaxWait:    42 * time.Second,
	}

	client := NewChatGPTClientWithProxyAndConfig("token", "cookie", "http://proxy.local", requestConfig)
	if client.httpClient.Timeout != requestConfig.RequestTimeout {
		t.Fatalf("request timeout = %v, want %v", client.httpClient.Timeout, requestConfig.RequestTimeout)
	}
	if client.streamClient.Timeout != requestConfig.SSETimeout+30*time.Second {
		t.Fatalf("stream timeout = %v, want %v", client.streamClient.Timeout, requestConfig.SSETimeout+30*time.Second)
	}
	if client.pollInterval != requestConfig.PollInterval {
		t.Fatalf("poll interval = %v, want %v", client.pollInterval, requestConfig.PollInterval)
	}
	if client.pollMaxWait != requestConfig.SSETimeout {
		t.Fatalf("poll max wait = %v, want %v", client.pollMaxWait, requestConfig.SSETimeout)
	}
}

func TestNewChatGPTClientWithProxyAndAuthDataUsesAuthHeaders(t *testing.T) {
	client := NewChatGPTClientWithProxyAndAuthData("token", "http://proxy.local", map[string]any{
		"cookies":            "__Secure-next-auth.session-token=abc",
		"oai-device-id":      "device-123",
		"oai-session-id":     "session-456",
		"user-agent":         "Mozilla/5.0 TestBrowser",
		"sec-ch-ua":          `"Google Chrome";v="147", "Chromium";v="147"`,
		"sec-ch-ua-mobile":   "?1",
		"sec-ch-ua-platform": `"Android"`,
	}, ImageRequestConfig{})

	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/sentinel/chat-requirements", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	client.setHeaders(req)

	if got := req.Header.Get("Cookie"); got != "__Secure-next-auth.session-token=abc" {
		t.Fatalf("cookie = %q, want auth cookie", got)
	}
	if got := req.Header.Get("OAI-Device-Id"); got != "device-123" {
		t.Fatalf("device id = %q, want auth device id", got)
	}
	if got := req.Header.Get("OAI-Session-Id"); got != "session-456" {
		t.Fatalf("session id = %q, want auth session id", got)
	}
	if got := req.Header.Get("User-Agent"); got != "Mozilla/5.0 TestBrowser" {
		t.Fatalf("user-agent = %q, want auth user-agent", got)
	}
	if got := req.Header.Get("Sec-CH-UA"); got != `"Google Chrome";v="147", "Chromium";v="147"` {
		t.Fatalf("sec-ch-ua = %q, want auth sec-ch-ua", got)
	}
	if got := req.Header.Get("Sec-CH-UA-Mobile"); got != "?1" {
		t.Fatalf("sec-ch-ua-mobile = %q, want auth sec-ch-ua-mobile", got)
	}
	if got := req.Header.Get("Sec-CH-UA-Platform"); got != `"Android"` {
		t.Fatalf("sec-ch-ua-platform = %q, want auth sec-ch-ua-platform", got)
	}
}

func TestGetSentinelTokensRetriesNetworkError(t *testing.T) {
	attempts := 0
	hookCalls := 0
	client := &ChatGPTClient{
		accessToken:          "token",
		oaiDeviceID:          "device",
		networkRetryAttempts: 1,
		networkRetryBackoff:  time.Millisecond,
		beforeNetworkRetry: func(ctx context.Context, info NetworkRetryInfo) error {
			hookCalls++
			if info.Operation != "chat-requirements request" {
				t.Fatalf("unexpected operation %q", info.Operation)
			}
			return nil
		},
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					return nil, io.EOF
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"token":"sentinel","proofofwork":{"required":false}}`)),
				}, nil
			}),
		},
	}

	token, proof, err := client.getSentinelTokens(context.Background())
	if err != nil {
		t.Fatalf("getSentinelTokens() returned error: %v", err)
	}
	if token != "sentinel" {
		t.Fatalf("token = %q, want sentinel", token)
	}
	if proof != "" {
		t.Fatalf("proof = %q, want empty", proof)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if hookCalls != 1 {
		t.Fatalf("hookCalls = %d, want 1", hookCalls)
	}
}

func TestGetSentinelTokensDoesNotRetryHTTP403(t *testing.T) {
	attempts := 0
	client := &ChatGPTClient{
		accessToken:          "token",
		oaiDeviceID:          "device",
		networkRetryAttempts: 2,
		networkRetryBackoff:  time.Millisecond,
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"detail":"forbidden"}`)),
				}, nil
			}),
		},
	}

	_, _, err := client.getSentinelTokens(context.Background())
	if err == nil {
		t.Fatal("expected getSentinelTokens() to fail")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if !strings.Contains(err.Error(), "chat-requirements returned 403") {
		t.Fatalf("unexpected error: %v", err)
	}
}
