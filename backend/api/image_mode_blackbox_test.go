package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"chatgpt2api/handler"
	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/config"
)

const imageModeCompatEnv = "RUN_IMAGE_MODE_COMPAT_TESTS"

type imageModeCompatScenario struct {
	name             string
	imageMode        string
	wantImageMode    string
	accountType      string
	freeRoute        string
	freeModel        string
	paidRoute        string
	paidModel        string
	cpaRouteStrategy string
	wantRoute        string
	wantDirection    string
	wantUpstream     string
	wantToolModel    string
	wantFactory      string
	wantCPASubroute  string
}

type compatFactoryRecorder struct {
	officialCalls  int
	responsesCalls int
	cpaCalls       int
	lastFactory    string
	lastModel      string
	callSequence   []string
}

type compatStubWorkflowClient struct {
	factory     string
	token       string
	cpaRoute    string
	model       string
	recorder    *compatFactoryRecorder
	generateErr error
	editErr     error
	inpaintErr  error
}

func (c *compatStubWorkflowClient) record(operation, model string) {
	if c == nil || c.recorder == nil {
		return
	}
	c.recorder.lastFactory = c.factory
	c.recorder.lastModel = model
	c.recorder.callSequence = append(c.recorder.callSequence, fmt.Sprintf("%s:%s:%s", c.factory, c.token, operation))
}

func (c *compatStubWorkflowClient) DownloadBytes(url string) ([]byte, error) {
	return []byte("stub:" + url), nil
}

func (c *compatStubWorkflowClient) DownloadAsBase64(ctx context.Context, url string) (string, error) {
	_ = ctx
	return base64.StdEncoding.EncodeToString([]byte("stub-image:" + url)), nil
}

func (c *compatStubWorkflowClient) GenerateImage(ctx context.Context, prompt, model string, n int, size, quality, background string) ([]handler.ImageResult, error) {
	_ = ctx
	_ = prompt
	_ = n
	_ = size
	_ = quality
	_ = background
	c.record("generate", model)
	if c.generateErr != nil {
		return nil, c.generateErr
	}
	return []handler.ImageResult{
		{
			URL:           "stub://generated",
			RevisedPrompt: "stub",
		},
	}, nil
}

func (c *compatStubWorkflowClient) EditImageByUpload(ctx context.Context, prompt, model string, images [][]byte, mask []byte, size, quality string) ([]handler.ImageResult, error) {
	_ = ctx
	_ = prompt
	_ = images
	_ = mask
	_ = size
	_ = quality
	c.record("edit", model)
	if c.editErr != nil {
		return nil, c.editErr
	}
	return []handler.ImageResult{
		{
			URL:           "stub://edited",
			RevisedPrompt: "stub",
		},
	}, nil
}

func (c *compatStubWorkflowClient) InpaintImageByMask(ctx context.Context, prompt string, model string, originalFileID string, originalGenID string, conversationID string, parentMessageID string, mask []byte, size string, quality string) ([]handler.ImageResult, error) {
	_ = ctx
	_ = prompt
	_ = originalFileID
	_ = originalGenID
	_ = conversationID
	_ = parentMessageID
	_ = mask
	_ = size
	_ = quality
	c.record("selection-edit", model)
	if c.inpaintErr != nil {
		return nil, c.inpaintErr
	}
	return []handler.ImageResult{
		{
			URL:           "stub://inpaint",
			RevisedPrompt: "stub",
		},
	}, nil
}

func (c *compatStubWorkflowClient) LastRoute() string {
	return c.cpaRoute
}

func (c *compatStubWorkflowClient) LastModelLabel() string {
	return c.model
}

type parallelGenerateWorkflowClient struct {
	token     string
	active    *int32
	maxActive *int32
	delay     time.Duration
}

func (c *parallelGenerateWorkflowClient) DownloadBytes(url string) ([]byte, error) {
	return []byte("parallel:" + url), nil
}

func (c *parallelGenerateWorkflowClient) DownloadAsBase64(ctx context.Context, url string) (string, error) {
	_ = ctx
	return base64.StdEncoding.EncodeToString([]byte("parallel:" + url)), nil
}

func (c *parallelGenerateWorkflowClient) GenerateImage(ctx context.Context, prompt, model string, n int, size, quality, background string) ([]handler.ImageResult, error) {
	_ = prompt
	_ = model
	_ = n
	_ = size
	_ = quality
	_ = background

	active := atomic.AddInt32(c.active, 1)
	for {
		maxActive := atomic.LoadInt32(c.maxActive)
		if active <= maxActive || atomic.CompareAndSwapInt32(c.maxActive, maxActive, active) {
			break
		}
	}
	defer atomic.AddInt32(c.active, -1)
	if c.delay > 0 {
		timer := time.NewTimer(c.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return []handler.ImageResult{
		{
			URL:           "stub://parallel/" + c.token,
			RevisedPrompt: "parallel",
		},
	}, nil
}

func (c *parallelGenerateWorkflowClient) EditImageByUpload(ctx context.Context, prompt, model string, images [][]byte, mask []byte, size, quality string) ([]handler.ImageResult, error) {
	_ = ctx
	_ = prompt
	_ = model
	_ = images
	_ = mask
	_ = size
	_ = quality
	return nil, fmt.Errorf("not implemented")
}

func (c *parallelGenerateWorkflowClient) InpaintImageByMask(ctx context.Context, prompt string, model string, originalFileID string, originalGenID string, conversationID string, parentMessageID string, mask []byte, size string, quality string) ([]handler.ImageResult, error) {
	_ = ctx
	_ = prompt
	_ = model
	_ = originalFileID
	_ = originalGenID
	_ = conversationID
	_ = parentMessageID
	_ = mask
	_ = size
	_ = quality
	return nil, fmt.Errorf("not implemented")
}

func TestImageGenerationsRunConcurrentlyAcrossAvailableAccounts(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "parallel-1.json",
				accessToken: "token-parallel-1",
				accountType: "Free",
				priority:    30,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "parallel-2.json",
				accessToken: "token-parallel-2",
				accountType: "Free",
				priority:    20,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "parallel-3.json",
				accessToken: "token-parallel-3",
				accountType: "Free",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
		},
	})

	var active int32
	var maxActive int32
	server.officialClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = proxyURL
		_ = authData
		_ = requestConfig
		recorder.officialCalls++
		return &parallelGenerateWorkflowClient{
			token:     accessToken,
			active:    &active,
			maxActive: &maxActive,
			delay:     150 * time.Millisecond,
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"parallel prompt","n":3,"response_format":"b64_json"}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(payload.Data))
	}
	if recorder.officialCalls != 3 {
		t.Fatalf("official client calls = %d, want 3", recorder.officialCalls)
	}
	if got := atomic.LoadInt32(&maxActive); got < 2 {
		t.Fatalf("max concurrent generate calls = %d, want at least 2", got)
	}
}

func TestImageGenerationsStaySerialWhenOnlyOneAccountIsAvailable(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{})

	var active int32
	var maxActive int32
	server.officialClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = proxyURL
		_ = authData
		_ = requestConfig
		recorder.officialCalls++
		return &parallelGenerateWorkflowClient{
			token:     accessToken,
			active:    &active,
			maxActive: &maxActive,
			delay:     120 * time.Millisecond,
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"serial prompt","n":3,"response_format":"b64_json"}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 3 {
		t.Fatalf("len(data) = %d, want 3", len(payload.Data))
	}
	if recorder.officialCalls != 3 {
		t.Fatalf("official client calls = %d, want 3", recorder.officialCalls)
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("max concurrent generate calls = %d, want 1 with a single available account", got)
	}
}

func TestImageModeCompatibilityBlackBox(t *testing.T) {
	if os.Getenv(imageModeCompatEnv) == "" {
		t.Skipf("set %s=1 to run optional image mode compatibility tests", imageModeCompatEnv)
	}

	scenarios := []imageModeCompatScenario{
		{
			name:          "studio free uses official legacy route",
			imageMode:     "studio",
			accountType:   "Free",
			freeRoute:     "legacy",
			freeModel:     "auto",
			paidRoute:     "responses",
			paidModel:     "gpt-5.4-mini",
			wantRoute:     "legacy",
			wantDirection: "official",
			wantUpstream:  "auto",
			wantToolModel: "auto",
			wantFactory:   "official",
		},
		{
			name:          "studio paid uses official responses route",
			imageMode:     "studio",
			accountType:   "Plus",
			freeRoute:     "legacy",
			freeModel:     "auto",
			paidRoute:     "responses",
			paidModel:     "gpt-5.4-mini",
			wantRoute:     "responses",
			wantDirection: "official",
			wantUpstream:  "gpt-5.4-mini",
			wantToolModel: "gpt-5.4-mini",
			wantFactory:   "responses",
		},
		{
			name:             "cpa mode always uses cpa fixed model",
			imageMode:        "cpa",
			accountType:      "Free",
			freeRoute:        "legacy",
			freeModel:        "auto",
			paidRoute:        "responses",
			paidModel:        "gpt-5.4-mini",
			cpaRouteStrategy: "images_api",
			wantRoute:        "cpa",
			wantDirection:    "cpa",
			wantUpstream:     "gpt-image-2",
			wantToolModel:    "gpt-image-2",
			wantFactory:      "cpa",
			wantCPASubroute:  "images_api",
		},
		{
			name:             "legacy mix free is coerced to studio official route",
			imageMode:        "mix",
			wantImageMode:    "studio",
			accountType:      "Free",
			freeRoute:        "responses",
			freeModel:        "auto",
			paidRoute:        "responses",
			paidModel:        "gpt-5.4-mini",
			cpaRouteStrategy: "images_api",
			wantRoute:        "responses",
			wantDirection:    "official",
			wantUpstream:     "auto",
			wantToolModel:    "auto",
			wantFactory:      "responses",
		},
		{
			name:             "legacy mix paid is coerced to studio official responses",
			imageMode:        "mix",
			wantImageMode:    "studio",
			accountType:      "Pro",
			freeRoute:        "legacy",
			freeModel:        "auto",
			paidRoute:        "responses",
			paidModel:        "gpt-5.4",
			cpaRouteStrategy: "images_api",
			wantRoute:        "responses",
			wantDirection:    "official",
			wantUpstream:     "gpt-5.4",
			wantToolModel:    "gpt-5.4",
			wantFactory:      "responses",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			server, recorder := newImageModeCompatTestServer(t, scenario)

			requestBody := `{"prompt":"test prompt","response_format":"b64_json"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(requestBody))
			req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
			req.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}

			var payload struct {
				Data []map[string]any `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(payload.Data) != 1 {
				t.Fatalf("len(data) = %d, want 1", len(payload.Data))
			}
			if _, ok := payload.Data[0]["b64_json"].(string); !ok {
				t.Fatalf("response data = %#v, want b64_json", payload.Data[0])
			}

			entries := server.reqLogs.list(1)
			if len(entries) != 1 {
				t.Fatalf("log entries = %d, want 1", len(entries))
			}
			entry := entries[0]
			wantImageMode := scenario.imageMode
			if scenario.wantImageMode != "" {
				wantImageMode = scenario.wantImageMode
			}
			if entry.ImageMode != wantImageMode {
				t.Fatalf("imageMode = %q, want %q", entry.ImageMode, wantImageMode)
			}
			if entry.Route != scenario.wantRoute {
				t.Fatalf("route = %q, want %q", entry.Route, scenario.wantRoute)
			}
			if entry.Direction != scenario.wantDirection {
				t.Fatalf("direction = %q, want %q", entry.Direction, scenario.wantDirection)
			}
			if entry.UpstreamModel != scenario.wantUpstream {
				t.Fatalf("upstreamModel = %q, want %q", entry.UpstreamModel, scenario.wantUpstream)
			}
			if scenario.wantCPASubroute != "" && entry.CPASubroute != scenario.wantCPASubroute {
				t.Fatalf("cpaSubroute = %q, want %q", entry.CPASubroute, scenario.wantCPASubroute)
			}
			if recorder.lastFactory != scenario.wantFactory {
				t.Fatalf("last factory = %q, want %q", recorder.lastFactory, scenario.wantFactory)
			}
			if recorder.lastModel != scenario.wantToolModel {
				t.Fatalf("factory tool model = %q, want %q", recorder.lastModel, scenario.wantToolModel)
			}

			switch scenario.wantFactory {
			case "official":
				if recorder.officialCalls != 1 || recorder.responsesCalls != 0 || recorder.cpaCalls != 0 {
					t.Fatalf("factory counts = %+v, want official only", recorder)
				}
			case "responses":
				if recorder.officialCalls != 0 || recorder.responsesCalls != 1 || recorder.cpaCalls != 0 {
					t.Fatalf("factory counts = %+v, want responses only", recorder)
				}
			case "cpa":
				if recorder.officialCalls != 0 || recorder.responsesCalls != 0 || recorder.cpaCalls != 1 {
					t.Fatalf("factory counts = %+v, want cpa only", recorder)
				}
			default:
				t.Fatalf("unexpected expected factory %q", scenario.wantFactory)
			}
		})
	}
}

func TestImageModeCompatibilityBlackBoxEditsAndSelection(t *testing.T) {
	if os.Getenv(imageModeCompatEnv) == "" {
		t.Skipf("set %s=1 to run optional image mode compatibility tests", imageModeCompatEnv)
	}

	selectionEditErr := newRequestError("source_context_missing", "CPA 路由不支持上下文选区编辑，将自动回退为源图加遮罩编辑")

	scenarios := []struct {
		name            string
		scenario        imageModeCompatScenario
		requestBuilder  func(t *testing.T, server *Server) *http.Request
		wantStatus      int
		wantErrorCode   string
		wantOperation   string
		wantRoute       string
		wantDirection   string
		wantUpstream    string
		wantToolModel   string
		wantFactory     string
		wantCPASubroute string
		behavior        compatClientBehavior
	}{
		{
			name: "studio paid edit uses responses route",
			scenario: imageModeCompatScenario{
				imageMode:   "studio",
				accountType: "Plus",
				freeRoute:   "legacy",
				freeModel:   "auto",
				paidRoute:   "responses",
				paidModel:   "gpt-5.4-mini",
			},
			requestBuilder: func(t *testing.T, server *Server) *http.Request {
				t.Helper()
				return newCompatMultipartRequest(t, "/v1/images/edits", map[string]string{
					"prompt":          "edit prompt",
					"model":           "gpt-image-2",
					"response_format": "b64_json",
				}, map[string][][]byte{
					"image": {[]byte("edit-image")},
				}, server.cfg.App.APIKey)
			},
			wantStatus:    http.StatusOK,
			wantOperation: "edit",
			wantRoute:     "responses",
			wantDirection: "official",
			wantUpstream:  "gpt-5.4-mini",
			wantToolModel: "gpt-5.4-mini",
			wantFactory:   "responses",
		},
		{
			name: "studio paid selection edit keeps preferred account on official client",
			scenario: imageModeCompatScenario{
				imageMode:   "studio",
				accountType: "Plus",
				freeRoute:   "legacy",
				freeModel:   "auto",
				paidRoute:   "responses",
				paidModel:   "gpt-5.4-mini",
			},
			requestBuilder: func(t *testing.T, server *Server) *http.Request {
				t.Helper()
				accountID := compatPrimaryAccountID(t, server)
				return newCompatMultipartRequest(t, "/v1/images/edits", map[string]string{
					"prompt":            "selection prompt",
					"model":             "gpt-image-2",
					"response_format":   "b64_json",
					"original_file_id":  "file-1",
					"original_gen_id":   "gen-1",
					"conversation_id":   "conv-1",
					"parent_message_id": "msg-1",
					"source_account_id": accountID,
				}, map[string][][]byte{
					"mask": {[]byte("selection-mask")},
				}, server.cfg.App.APIKey)
			},
			wantStatus:    http.StatusOK,
			wantOperation: "selection-edit",
			wantRoute:     "responses",
			wantDirection: "official",
			wantUpstream:  "gpt-5.4-mini",
			wantToolModel: "gpt-5.4-mini",
			wantFactory:   "official",
		},
		{
			name: "cpa selection edit returns source context error",
			scenario: imageModeCompatScenario{
				imageMode:        "cpa",
				accountType:      "Free",
				freeRoute:        "legacy",
				freeModel:        "auto",
				paidRoute:        "responses",
				paidModel:        "gpt-5.4-mini",
				cpaRouteStrategy: "images_api",
				wantUpstream:     "gpt-image-2",
			},
			requestBuilder: func(t *testing.T, server *Server) *http.Request {
				t.Helper()
				accountID := compatPrimaryAccountID(t, server)
				return newCompatMultipartRequest(t, "/v1/images/edits", map[string]string{
					"prompt":            "selection prompt",
					"model":             "gpt-image-2",
					"response_format":   "b64_json",
					"original_file_id":  "file-1",
					"original_gen_id":   "gen-1",
					"conversation_id":   "conv-1",
					"parent_message_id": "msg-1",
					"source_account_id": accountID,
				}, map[string][][]byte{
					"mask": {[]byte("selection-mask")},
				}, server.cfg.App.APIKey)
			},
			wantStatus:      http.StatusBadGateway,
			wantErrorCode:   "source_context_missing",
			wantOperation:   "selection-edit",
			wantRoute:       "cpa",
			wantDirection:   "cpa",
			wantUpstream:    "gpt-image-2",
			wantToolModel:   "gpt-image-2",
			wantFactory:     "cpa",
			wantCPASubroute: "",
			behavior: compatClientBehavior{
				cpaInpaintErr: selectionEditErr,
			},
		},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			server, recorder := newImageModeCompatTestServerWithOptions(t, tt.scenario, compatTestServerOptions{
				behavior: tt.behavior,
			})

			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, tt.requestBuilder(t, server))

			assertCompatResponse(t, rec, tt.wantStatus, tt.wantErrorCode)
			entries := server.reqLogs.list(1)
			if len(entries) != 1 {
				t.Fatalf("log entries = %d, want 1", len(entries))
			}
			entry := entries[0]
			if entry.Operation != tt.wantOperation {
				t.Fatalf("operation = %q, want %q", entry.Operation, tt.wantOperation)
			}
			if entry.Route != tt.wantRoute {
				t.Fatalf("route = %q, want %q", entry.Route, tt.wantRoute)
			}
			if entry.Direction != tt.wantDirection {
				t.Fatalf("direction = %q, want %q", entry.Direction, tt.wantDirection)
			}
			if entry.UpstreamModel != tt.wantUpstream {
				t.Fatalf("upstreamModel = %q, want %q", entry.UpstreamModel, tt.wantUpstream)
			}
			if tt.wantCPASubroute != "" && entry.CPASubroute != tt.wantCPASubroute {
				t.Fatalf("cpaSubroute = %q, want %q", entry.CPASubroute, tt.wantCPASubroute)
			}
			if recorder.lastFactory != tt.wantFactory {
				t.Fatalf("last factory = %q, want %q", recorder.lastFactory, tt.wantFactory)
			}
			if recorder.lastModel != tt.wantToolModel {
				t.Fatalf("factory tool model = %q, want %q", recorder.lastModel, tt.wantToolModel)
			}
		})
	}
}

func TestImageModeCompatibilityBlackBoxRetry(t *testing.T) {
	if os.Getenv(imageModeCompatEnv) == "" {
		t.Skipf("set %s=1 to run optional image mode compatibility tests", imageModeCompatEnv)
	}

	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Free",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "priority-first.json",
				accessToken: "token-first",
				accountType: "Free",
				priority:    100,
				quota:       5,
			},
			{
				fileName:    "fallback-second.json",
				accessToken: "token-second",
				accountType: "Free",
				priority:    10,
				quota:       5,
			},
		},
		behavior: compatClientBehavior{
			officialGenerateErrors: map[string]error{
				"token-first": fmt.Errorf("HTTP 401 unauthorized"),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"retry prompt","response_format":"b64_json"}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	assertCompatResponse(t, rec, http.StatusOK, "")

	if got, want := recorder.officialCalls, 2; got != want {
		t.Fatalf("officialCalls = %d, want %d", got, want)
	}
	if len(recorder.callSequence) != 2 {
		t.Fatalf("callSequence = %#v, want 2 entries", recorder.callSequence)
	}
	if recorder.callSequence[0] != "official:token-first:generate" {
		t.Fatalf("first attempt = %q, want token-first generate", recorder.callSequence[0])
	}
	if recorder.callSequence[1] != "official:token-second:generate" {
		t.Fatalf("second attempt = %q, want token-second generate", recorder.callSequence[1])
	}

	entries := server.reqLogs.list(2)
	if len(entries) != 2 {
		t.Fatalf("log entries = %d, want 2", len(entries))
	}
	if !entries[0].Success || entries[1].Success {
		t.Fatalf("log success flags = [%v %v], want [true false]", entries[0].Success, entries[1].Success)
	}
	if entries[0].AccountFile != "fallback-second.json" {
		t.Fatalf("success account = %q, want fallback-second.json", entries[0].AccountFile)
	}
	if entries[1].AccountFile != "priority-first.json" {
		t.Fatalf("failure account = %q, want priority-first.json", entries[1].AccountFile)
	}
}

type compatSeedAccount struct {
	fileName    string
	accessToken string
	accountType string
	priority    int
	quota       int
	status      string
}

type compatClientBehavior struct {
	officialGenerateErrors  map[string]error
	officialEditErrors      map[string]error
	officialInpaintErrors   map[string]error
	responsesGenerateErrors map[string]error
	responsesEditErrors     map[string]error
	responsesInpaintErrors  map[string]error
	cpaGenerateErrors       map[string]error
	cpaEditErrors           map[string]error
	cpaInpaintErr           error
}

type compatTestServerOptions struct {
	accounts []compatSeedAccount
	behavior compatClientBehavior
}

func newImageModeCompatTestServer(t *testing.T, scenario imageModeCompatScenario) (*Server, *compatFactoryRecorder) {
	return newImageModeCompatTestServerWithOptions(t, scenario, compatTestServerOptions{})
}

func newImageModeCompatTestServerWithOptions(t *testing.T, scenario imageModeCompatScenario, options compatTestServerOptions) (*Server, *compatFactoryRecorder) {
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
	cfg.ChatGPT.ImageMode = scenario.imageMode
	cfg.ChatGPT.FreeImageRoute = scenario.freeRoute
	cfg.ChatGPT.FreeImageModel = scenario.freeModel
	cfg.ChatGPT.PaidImageRoute = scenario.paidRoute
	cfg.ChatGPT.PaidImageModel = scenario.paidModel
	cfg.CPA.BaseURL = "http://127.0.0.1:8317"
	cfg.CPA.APIKey = "test-cpa-key"
	cfg.CPA.RequestTimeout = 60
	cfg.CPA.RouteStrategy = scenario.cpaRouteStrategy

	if err := seedImageModeCompatAccounts(cfg, scenario.accountType, options.accounts); err != nil {
		t.Fatalf("seed compat account: %v", err)
	}

	store, err := accounts.NewStore(cfg)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	server := NewServer(cfg, store, nil)
	recorder := &compatFactoryRecorder{}
	server.officialClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = accessToken
		_ = proxyURL
		_ = authData
		_ = requestConfig
		recorder.officialCalls++
		return &compatStubWorkflowClient{
			factory:     "official",
			token:       accessToken,
			recorder:    recorder,
			generateErr: options.behavior.officialGenerateErrors[accessToken],
			editErr:     options.behavior.officialEditErrors[accessToken],
			inpaintErr:  options.behavior.officialInpaintErrors[accessToken],
		}
	}
	server.responsesClientFactory = func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
		_ = accessToken
		_ = proxyURL
		_ = authData
		_ = requestConfig
		recorder.responsesCalls++
		return &compatStubWorkflowClient{
			factory:     "responses",
			token:       accessToken,
			recorder:    recorder,
			generateErr: options.behavior.responsesGenerateErrors[accessToken],
			editErr:     options.behavior.responsesEditErrors[accessToken],
			inpaintErr:  options.behavior.responsesInpaintErrors[accessToken],
		}
	}
	server.cpaClientFactory = func(baseURL, apiKey string, timeout time.Duration, routeStrategy string) cpaRouteAwareImageWorkflowClient {
		_ = baseURL
		_ = apiKey
		_ = timeout
		recorder.cpaCalls++
		return &compatStubWorkflowClient{
			factory:     "cpa",
			token:       "cpa",
			cpaRoute:    firstNonEmpty(scenario.wantCPASubroute, routeStrategy, "images_api"),
			model:       scenario.wantUpstream,
			recorder:    recorder,
			generateErr: options.behavior.cpaGenerateErrors["cpa"],
			editErr:     options.behavior.cpaEditErrors["cpa"],
			inpaintErr:  options.behavior.cpaInpaintErr,
		}
	}
	return server, recorder
}

func seedImageModeCompatAccounts(cfg *config.Config, accountType string, accountsToSeed []compatSeedAccount) error {
	authDir := cfg.ResolvePath(cfg.Storage.AuthDir)
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		return err
	}
	statePath := cfg.ResolvePath(cfg.Storage.StateFile)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}

	if len(accountsToSeed) == 0 {
		accountsToSeed = []compatSeedAccount{
			{
				fileName:    "compat.json",
				accessToken: "token-compat",
				accountType: accountType,
				priority:    0,
				quota:       5,
				status:      "正常",
			},
		}
	}

	stateAccounts := make(map[string]any, len(accountsToSeed))
	for index, account := range accountsToSeed {
		fileName := firstNonEmpty(account.fileName, fmt.Sprintf("compat-%d.json", index+1))
		authPayload := map[string]any{
			"type":         "codex",
			"source_kind":  accounts.AccountSourceKindAuthFile,
			"access_token": firstNonEmpty(account.accessToken, fmt.Sprintf("token-%d", index+1)),
			"email":        fmt.Sprintf("compat-%d@example.com", index+1),
			"priority":     account.priority,
		}
		if err := writeCompatJSON(filepath.Join(authDir, fileName), authPayload); err != nil {
			return err
		}

		quota := account.quota
		if quota <= 0 {
			quota = 5
		}
		stateAccounts[fileName] = map[string]any{
			"type":        firstNonEmpty(account.accountType, accountType, "Free"),
			"status":      firstNonEmpty(account.status, "正常"),
			"quota":       quota,
			"quota_known": true,
			"priority":    account.priority,
			"limits_progress": []map[string]any{
				{
					"feature_name": "image_gen",
					"remaining":    quota,
					"reset_after":  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				},
			},
		}
	}

	statePayload := map[string]any{
		"accounts": stateAccounts,
	}
	return writeCompatJSON(statePath, statePayload)
}

func compatPrimaryAccountID(t *testing.T, server *Server) string {
	t.Helper()
	items, err := server.store.ListAccounts()
	if err != nil {
		t.Fatalf("list accounts: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no compat account seeded")
	}
	return items[0].ID
}

func newCompatMultipartRequest(t *testing.T, path string, fields map[string]string, files map[string][][]byte, apiKey string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	for fieldName, payloads := range files {
		for index, payload := range payloads {
			part, err := writer.CreateFormFile(fieldName, fmt.Sprintf("%s-%d.png", fieldName, index+1))
			if err != nil {
				t.Fatalf("create form file %s: %v", fieldName, err)
			}
			if _, err := part.Write(payload); err != nil {
				t.Fatalf("write form file %s: %v", fieldName, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func assertCompatResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantErrorCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, wantStatus, rec.Body.String())
	}

	if wantErrorCode == "" {
		var payload struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode success response: %v", err)
		}
		if len(payload.Data) == 0 {
			t.Fatalf("response data is empty: %s", rec.Body.String())
		}
		if _, ok := payload.Data[0]["b64_json"].(string); !ok {
			t.Fatalf("response data = %#v, want b64_json", payload.Data[0])
		}
		return
	}

	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error.Code != wantErrorCode {
		t.Fatalf("error code = %q, want %q", payload.Error.Code, wantErrorCode)
	}
}

func writeCompatJSON(path string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
