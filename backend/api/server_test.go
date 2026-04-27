package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/config"
	"chatgpt2api/internal/imagehistory"
)

func TestShouldUseOfficialResponses(t *testing.T) {
	tests := []struct {
		name              string
		preferredAccount  bool
		responsesEligible bool
		configuredRoute   string
		want              bool
	}{
		{
			name:              "paid account with eligible request uses responses",
			responsesEligible: true,
			configuredRoute:   "responses",
			want:              true,
		},
		{
			name:              "paid account with ineligible payload stays legacy",
			responsesEligible: false,
			configuredRoute:   "responses",
			want:              false,
		},
		{
			name:              "preferred source account stays legacy",
			preferredAccount:  true,
			responsesEligible: true,
			configuredRoute:   "responses",
			want:              false,
		},
		{
			name:              "legacy route stays legacy",
			responsesEligible: true,
			configuredRoute:   "legacy",
			want:              false,
		},
		{
			name:              "unknown route falls back to legacy",
			responsesEligible: true,
			configuredRoute:   "something-else",
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseOfficialResponses(tt.preferredAccount, tt.responsesEligible, tt.configuredRoute); got != tt.want {
				t.Fatalf("shouldUseOfficialResponses() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfiguredImageRoute(t *testing.T) {
	server := &Server{
		cfg: &config.Config{
			ChatGPT: config.ChatGPTConfig{
				FreeImageRoute: "responses",
				PaidImageRoute: "legacy",
			},
		},
	}

	if got := server.configuredImageRoute("Free"); got != "responses" {
		t.Fatalf("configuredImageRoute(Free) = %q, want %q", got, "responses")
	}
	if got := server.configuredImageRoute("Plus"); got != "legacy" {
		t.Fatalf("configuredImageRoute(Plus) = %q, want %q", got, "legacy")
	}
}

func TestMigrateImageFilesSkipsNestedTargetDirectory(t *testing.T) {
	rootDir := t.TempDir()
	cfg := config.New(rootDir)
	if err := cfg.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	server := NewServer(cfg, nil, nil, nil)

	oldDir := filepath.Join(rootDir, "data", "tmp", "image")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(oldDir) returned error: %v", err)
	}
	sourcePath := filepath.Join(oldDir, "sample.png")
	if err := os.WriteFile(sourcePath, []byte("image"), 0o644); err != nil {
		t.Fatalf("WriteFile(sourcePath) returned error: %v", err)
	}

	previous := configPayload{}
	previous.Storage.ImageDir = "data/tmp/image"
	next := configPayload{}
	next.Storage.ImageDir = "data/tmp/image/nested"

	if err := server.migrateImageFilesIfNeeded(previous, next); err != nil {
		t.Fatalf("migrateImageFilesIfNeeded() returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "data", "tmp", "image", "nested", "sample.png")); err != nil {
		t.Fatalf("expected migrated file in nested dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "data", "tmp", "image", "nested", "nested", "sample.png")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no recursive nested file, got err=%v", err)
	}
}

func TestResolveImageFilePathFallsBackToOtherDataDirectories(t *testing.T) {
	rootDir := t.TempDir()
	cfg := config.New(rootDir)
	if err := cfg.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	cfg.Storage.ImageDir = "data/new-images"
	server := NewServer(cfg, nil, nil, nil)

	legacyDir := filepath.Join(rootDir, "data", "old-images")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(legacyDir) returned error: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, "kept.png")
	if err := os.WriteFile(legacyPath, []byte("image"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacyPath) returned error: %v", err)
	}

	got := server.resolveImageFilePath("kept.png")
	if !strings.EqualFold(filepath.Clean(got), filepath.Clean(legacyPath)) {
		t.Fatalf("resolveImageFilePath() = %q, want %q", got, legacyPath)
	}
}

func TestImportImageConversationsIntoSQLiteTarget(t *testing.T) {
	rootDir := t.TempDir()
	cfg := config.New(rootDir)
	if err := cfg.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	cfg.App.AuthKey = "test-auth"
	server := NewServer(cfg, nil, nil, nil)

	body := map[string]any{
		"items": []map[string]any{
			{
				"id":        "conv-1",
				"title":     "生成",
				"mode":      "generate",
				"prompt":    "test",
				"model":     "gpt-image-2",
				"count":     1,
				"createdAt": "2026-04-26T00:00:00Z",
				"status":    "success",
				"turns": []map[string]any{
					{
						"id":        "turn-1",
						"title":     "生成",
						"mode":      "generate",
						"prompt":    "test",
						"model":     "gpt-image-2",
						"count":     1,
						"createdAt": "2026-04-26T00:00:00Z",
						"status":    "success",
						"images": []map[string]any{
							{
								"id":       "img-1",
								"status":   "success",
								"b64_json": "aW1hZ2U=",
							},
						},
					},
				},
			},
		},
		"storage": map[string]any{
			"backend":                  "sqlite",
			"imageDir":                 "data/import-images",
			"sqlitePath":               "data/import-history.sqlite",
			"imageConversationStorage": "server",
			"imageDataStorage":         "server",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/image/conversations/import", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+cfg.App.AuthKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	verifyCfg := config.New(rootDir)
	verifyCfg.Storage.Backend = "sqlite"
	verifyCfg.Storage.ImageDir = "data/import-images"
	verifyCfg.Storage.SQLitePath = "data/import-history.sqlite"
	store, err := imagehistory.NewStore(verifyCfg)
	if err != nil {
		t.Fatalf("NewStore(verify sqlite) returned error: %v", err)
	}
	defer store.Close()

	items, err := store.List(req.Context())
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "conv-1" {
		t.Fatalf("imported items = %#v", items)
	}
}

func TestConfiguredImageModeTreatsLegacyMixAsStudio(t *testing.T) {
	server := &Server{
		cfg: &config.Config{
			ChatGPT: config.ChatGPTConfig{
				ImageMode: "mix",
			},
		},
	}

	if got := server.configuredImageMode(); got != "studio" {
		t.Fatalf("configuredImageMode() = %q, want %q", got, "studio")
	}
}

func TestHandleCreateAccountsRejectsOutsideStudioMode(t *testing.T) {
	rootDir := t.TempDir()
	cfg := config.New(rootDir)
	if err := cfg.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	cfg.App.AuthKey = "test-auth"
	cfg.ChatGPT.ImageMode = "cpa"

	store, err := accounts.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	server := NewServer(cfg, store, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts", strings.NewReader(`{"tokens":["token-1"]}`))
	req.Header.Set("Authorization", "Bearer "+cfg.App.AuthKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Studio") {
		t.Fatalf("body = %s, want Studio mode error", rec.Body.String())
	}
}

func TestHandleRefreshAllAccountsWithEmptyStoreFinishesImmediately(t *testing.T) {
	rootDir := t.TempDir()
	cfg := config.New(rootDir)
	if err := cfg.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	cfg.App.AuthKey = "test-auth"

	store, err := accounts.NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore() returned error: %v", err)
	}
	defer store.Close()

	server := NewServer(cfg, store, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/accounts/refresh-all", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+cfg.App.AuthKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Progress *accountRefreshRunResult `json:"progress"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() returned error: %v", err)
	}
	if payload.Progress == nil {
		t.Fatal("progress should not be nil")
	}
	if payload.Progress.Running {
		t.Fatalf("progress.Running = true, want false")
	}
	if payload.Progress.Total != 0 || payload.Progress.Processed != 0 {
		t.Fatalf("progress = %#v, want empty finished run", payload.Progress)
	}
}

func TestResolveImageUpstreamModelFromConfig(t *testing.T) {
	server := &Server{
		cfg: &config.Config{
			ChatGPT: config.ChatGPTConfig{
				FreeImageModel: "auto",
				PaidImageModel: "gpt-5.4",
			},
		},
	}

	if got := server.resolveImageUpstreamModel("gpt-image-1", "Plus"); got != "gpt-5.4" {
		t.Fatalf("resolveImageUpstreamModel() = %q, want %q", got, "gpt-5.4")
	}
	if got := server.resolveImageUpstreamModel("gpt-image-2", "Free"); got != "auto" {
		t.Fatalf("resolveImageUpstreamModel() = %q, want %q", got, "auto")
	}
}

func TestResolveImageAcquireError(t *testing.T) {
	lastErr := errors.New("refresh failed")
	noAvailableErr := errors.New("read dir failed")

	tests := []struct {
		name             string
		mode             string
		err              error
		lastRetryableErr error
		wantMessage      string
		wantCode         string
	}{
		{
			name:        "cpa mode still maps empty pool when helper is used",
			mode:        "cpa",
			err:         accounts.ErrNoAvailableImageAuth,
			wantMessage: "当前没有可用的图片账号用于 CPA 模式",
			wantCode:    "no_cpa_image_accounts",
		},
		{
			name:             "retry exhaustion keeps last real error",
			mode:             "cpa",
			err:              accounts.ErrNoAvailableImageAuth,
			lastRetryableErr: lastErr,
			wantMessage:      lastErr.Error(),
		},
		{
			name:        "non sentinel error passes through",
			mode:        "cpa",
			err:         noAvailableErr,
			wantMessage: noAvailableErr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveImageAcquireError(tt.mode, tt.err, tt.lastRetryableErr)
			if got == nil {
				t.Fatal("resolveImageAcquireError() returned nil")
			}
			if got.Error() != tt.wantMessage {
				t.Fatalf("resolveImageAcquireError() error = %q, want %q", got.Error(), tt.wantMessage)
			}
			if tt.wantCode != "" && requestErrorCode(got) != tt.wantCode {
				t.Fatalf("resolveImageAcquireError() code = %q, want %q", requestErrorCode(got), tt.wantCode)
			}
		})
	}
}

func TestNormalizeGenerateImageSize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty size uses default upstream behavior",
			input: "",
			want:  "",
		},
		{
			name:  "supported landscape size passes through",
			input: "1536x1024",
			want:  "1536x1024",
		},
		{
			name:  "uppercase separator is normalized",
			input: "1024X1536",
			want:  "1024x1536",
		},
		{
			name:  "unsupported large size now passes through normalized",
			input: "8192x8192",
			want:  "8192x8192",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGenerateImageSize(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeGenerateImageSize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsImageRateLimitError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "http 429", err: errors.New("backend-api failed: HTTP 429"), want: true},
		{name: "too many requests", err: errors.New("Too Many Requests"), want: true},
		{name: "rate limit", err: errors.New("rate limit exceeded"), want: true},
		{name: "quota exceeded", err: errors.New("image generation quota exceeded"), want: true},
		{name: "non rate error", err: errors.New("internal server error"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isImageRateLimitError(tt.err); got != tt.want {
				t.Fatalf("isImageRateLimitError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTransientImageStreamError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "responses sse internal error", err: errors.New("responses SSE read error: stream error: stream ID 1; INTERNAL_ERROR; received from peer"), want: true},
		{name: "unexpected eof", err: errors.New("SSE read error: unexpected EOF"), want: true},
		{name: "http2 connection lost", err: errors.New("http2: client connection lost"), want: true},
		{name: "non transient", err: errors.New("no images generated"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientImageStreamError(tt.err); got != tt.want {
				t.Fatalf("isTransientImageStreamError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStudioPaidResolutionUsesPaidAccount(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Plus",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "free.json",
				accessToken: "token-free-priority",
				accountType: "Free",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "paid.json",
				accessToken: "token-paid",
				accountType: "Plus",
				priority:    1,
				quota:       5,
				status:      "正常",
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"test prompt","size":"2560x1440","quality":"high","response_format":"b64_json"}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	entries := server.reqLogs.list(1)
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.AccountType != "Plus" {
		t.Fatalf("account type = %q, want %q", entry.AccountType, "Plus")
	}
	if entry.Size != "2560x1440" {
		t.Fatalf("log size = %q, want %q", entry.Size, "2560x1440")
	}
	if entry.Quality != "high" {
		t.Fatalf("log quality = %q, want %q", entry.Quality, "high")
	}
	if entry.ImageToolModel != "gpt-5.4-mini" {
		t.Fatalf("log image tool model = %q, want %q", entry.ImageToolModel, "gpt-5.4-mini")
	}
	if entry.PromptLength != 11 {
		t.Fatalf("log prompt length = %d, want 11", entry.PromptLength)
	}
	if recorder.lastFactory != "responses" {
		t.Fatalf("last factory = %q, want %q", recorder.lastFactory, "responses")
	}
	if got := recorder.callSequence[len(recorder.callSequence)-1]; !strings.Contains(got, "token-paid") {
		t.Fatalf("call sequence = %v, want paid token selected", recorder.callSequence)
	}
}

func TestStudioRateLimitedAccountRetriesWithNextAccount(t *testing.T) {
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
				fileName:    "limited.json",
				accessToken: "token-limited",
				accountType: "Free",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "fallback.json",
				accessToken: "token-fallback",
				accountType: "Free",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
		},
		behavior: compatClientBehavior{
			officialGenerateErrors: map[string]error{
				"token-limited": errors.New("backend-api failed: HTTP 429 too many requests"),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"test prompt","response_format":"b64_json"}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if len(recorder.callSequence) != 2 {
		t.Fatalf("call sequence = %v, want two attempts", recorder.callSequence)
	}
	if !strings.Contains(recorder.callSequence[0], "token-limited") || !strings.Contains(recorder.callSequence[1], "token-fallback") {
		t.Fatalf("call sequence = %v, want limited then fallback", recorder.callSequence)
	}

	limitedAccount, err := server.getStore().GetAccountByToken("token-limited")
	if err != nil {
		t.Fatalf("GetAccountByToken(limited) returned error: %v", err)
	}
	if limitedAccount.Status != "限流" {
		t.Fatalf("limited account status = %q, want %q", limitedAccount.Status, "限流")
	}
	if limitedAccount.Quota != 0 {
		t.Fatalf("limited account quota = %d, want 0", limitedAccount.Quota)
	}
}

func TestStudioResponsesRateLimitedAccountRetriesWithNextAccount(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Plus",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "limited-paid.json",
				accessToken: "token-limited-paid",
				accountType: "Plus",
				priority:    100,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "fallback-paid.json",
				accessToken: "token-fallback-paid",
				accountType: "Plus",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
		},
		behavior: compatClientBehavior{
			responsesGenerateErrors: map[string]error{
				"token-limited-paid": errors.New("responses failed: HTTP 429 too many requests"),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"test prompt","size":"2560x1440","quality":"high","response_format":"b64_json"}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if recorder.lastFactory != "responses" {
		t.Fatalf("last factory = %q, want %q", recorder.lastFactory, "responses")
	}
	if len(recorder.callSequence) != 2 {
		t.Fatalf("call sequence = %v, want two attempts", recorder.callSequence)
	}
	if !strings.Contains(recorder.callSequence[0], "token-limited-paid") || !strings.Contains(recorder.callSequence[1], "token-fallback-paid") {
		t.Fatalf("call sequence = %v, want limited then fallback responses account", recorder.callSequence)
	}

	limitedAccount, err := server.getStore().GetAccountByToken("token-limited-paid")
	if err != nil {
		t.Fatalf("GetAccountByToken(limited paid) returned error: %v", err)
	}
	if limitedAccount.Status != "限流" {
		t.Fatalf("limited paid account status = %q, want %q", limitedAccount.Status, "限流")
	}
	if limitedAccount.Quota != 0 {
		t.Fatalf("limited paid account quota = %d, want 0", limitedAccount.Quota)
	}
}

func TestStudioPaidResolutionFallsBackOutsideSelectedFreeOnlyGroup(t *testing.T) {
	server, recorder := newImageModeCompatTestServerWithOptions(t, imageModeCompatScenario{
		imageMode:   "studio",
		accountType: "Plus",
		freeRoute:   "legacy",
		freeModel:   "auto",
		paidRoute:   "responses",
		paidModel:   "gpt-5.4-mini",
	}, compatTestServerOptions{
		accounts: []compatSeedAccount{
			{
				fileName:    "free-1.json",
				accessToken: "token-free-1",
				accountType: "Free",
				priority:    10,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "free-2.json",
				accessToken: "token-free-2",
				accountType: "Free",
				priority:    9,
				quota:       5,
				status:      "正常",
			},
			{
				fileName:    "paid-1.json",
				accessToken: "token-paid-1",
				accountType: "Plus",
				priority:    8,
				quota:       5,
				status:      "正常",
			},
		},
	})

	policyHeader := base64.RawURLEncoding.EncodeToString([]byte(`{
		"enabled": true,
		"sortMode": "imported_at",
		"groupSize": 2,
		"enabledGroupIndexes": [0],
		"reserveMode": "daily_first_seen_percent",
		"reservePercent": 20
	}`))

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"test prompt","size":"2560x1440","quality":"high","response_format":"b64_json"}`))
	req.Header.Set("Authorization", "Bearer "+server.cfg.App.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(imageAccountPolicyHeader, policyHeader)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if recorder.lastFactory != "responses" {
		t.Fatalf("last factory = %q, want responses", recorder.lastFactory)
	}
	if got := recorder.callSequence[len(recorder.callSequence)-1]; !strings.Contains(got, "token-paid-1") {
		t.Fatalf("call sequence = %v, want paid fallback selected", recorder.callSequence)
	}
	entries := server.reqLogs.list(1)
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].RoutingPolicyApplied {
		t.Fatalf("expected fallback outside selected groups to skip policy-applied log flag, got %#v", entries[0])
	}
}
