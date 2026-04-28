package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatgpt2api/handler"
	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/buildinfo"
	"chatgpt2api/internal/cliproxy"
	"chatgpt2api/internal/config"
	"chatgpt2api/internal/mailer"
	"chatgpt2api/internal/middleware"
	"chatgpt2api/internal/newapi"
	"chatgpt2api/internal/portalstore"
	"chatgpt2api/internal/sub2api"
	"chatgpt2api/internal/users"
	"chatgpt2api/internal/verification"
)

type Server struct {
	cfg                    *config.Config
	runtimeMu              sync.RWMutex
	store                  *accounts.Store
	userStore              *users.Store
	syncClient             *cliproxy.Client
	syncRunMu              sync.RWMutex
	syncRunCache           map[string]*sourceSyncRunResult
	accountRefreshMu       sync.RWMutex
	accountRefreshRun      *accountRefreshRunResult
	staticDir              string
	portalStaticDir        string
	portalStore            *portalstore.Store
	portalStoreErr         error
	reqLogs                *imageRequestLogStore
	mailer                 *mailer.Sender
	verifier               *verification.Store
	imageAdmission         *imageAdmissionController
	officialClientFactory  func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient
	responsesClientFactory func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient
	cpaClientFactory       func(baseURL, apiKey string, timeout time.Duration, routeStrategy string) cpaRouteAwareImageWorkflowClient
	newAPIClientFactory    func(cfg *config.Config) *newapi.Client
	sub2apiClientFactory   func(cfg *config.Config) *sub2api.Client
	sourceClientMu         sync.Mutex
	cachedNewAPIClient     *newapi.Client
	cachedNewAPIKey        string
	cachedSub2APIClient    *sub2api.Client
	cachedSub2APIKey       string
}

type requestError struct {
	code    string
	message string
}

type accountRefreshRunResult struct {
	OK         bool   `json:"ok"`
	Running    bool   `json:"running"`
	Error      string `json:"error,omitempty"`
	Total      int    `json:"total"`
	Processed  int    `json:"processed"`
	Refreshed  int    `json:"refreshed"`
	Failed     int    `json:"failed"`
	Current    string `json:"current,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

const cpaFixedImageModel = "gpt-image-2"
const maxBulkAccountRefreshWorkers = 4

func (e *requestError) Error() string {
	return firstNonEmpty(e.message, e.code)
}

func NewServer(cfg *config.Config, store *accounts.Store, userStore *users.Store, syncClient *cliproxy.Client) *Server {
	codeTTLMinutes := max(1, cfg.Mail.CodeTTLMinutes)
	resendIntervalSeconds := max(1, cfg.Mail.ResendInterval)
	portalStore, portalStoreErr := portalstore.NewStore(cfg)
	if portalStoreErr != nil {
		slog.Warn("portal store init failed", slog.Any("error", portalStoreErr))
	}

	return &Server{
		cfg:             cfg,
		store:           store,
		userStore:       userStore,
		syncClient:      syncClient,
		syncRunCache:    map[string]*sourceSyncRunResult{},
		staticDir:       cfg.ResolvePath(cfg.Server.StaticDir),
		portalStaticDir: cfg.ResolvePath("portal-static"),
		portalStore:     portalStore,
		portalStoreErr:  portalStoreErr,
		reqLogs:         newImageRequestLogStore(),
		mailer:          mailer.NewSender(cfg.Mail),
		verifier: verification.NewStore(
			time.Duration(codeTTLMinutes)*time.Minute,
			time.Duration(resendIntervalSeconds)*time.Second,
		),
		imageAdmission: newImageAdmissionController(),
		officialClientFactory: func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
			return handler.NewChatGPTClientWithProxyAndAuthData(accessToken, proxyURL, authData, requestConfig)
		},
		responsesClientFactory: func(accessToken, proxyURL string, authData map[string]any, requestConfig handler.ImageRequestConfig) imageWorkflowClient {
			return handler.NewResponsesClientWithProxyAndConfig(accessToken, proxyURL, authData, requestConfig)
		},
		cpaClientFactory: func(baseURL, apiKey string, timeout time.Duration, routeStrategy string) cpaRouteAwareImageWorkflowClient {
			return newCPAImageClient(baseURL, apiKey, timeout, routeStrategy)
		},
		newAPIClientFactory: func(cfg *config.Config) *newapi.Client {
			timeout := time.Duration(max(10, cfg.NewAPI.RequestTimeout)) * time.Second
			return newapi.New(
				cfg.NewAPI.BaseURL,
				cfg.NewAPI.Username,
				cfg.NewAPI.Password,
				cfg.NewAPI.AccessToken,
				cfg.NewAPI.UserID,
				cfg.NewAPI.SessionCookie,
				timeout,
				cfg.SyncProxyURL(),
			)
		},
		sub2apiClientFactory: func(cfg *config.Config) *sub2api.Client {
			timeout := time.Duration(max(10, cfg.Sub2API.RequestTimeout)) * time.Second
			return sub2api.New(
				cfg.Sub2API.BaseURL,
				cfg.Sub2API.Email,
				cfg.Sub2API.Password,
				cfg.Sub2API.APIKey,
				cfg.Sub2API.GroupID,
				timeout,
				cfg.SyncProxyURL(),
			)
		},
	}
}

func (s *Server) getStore() *accounts.Store {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.store
}

func (s *Server) getSyncClient() *cliproxy.Client {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.syncClient
}

func (s *Server) getStaticDir() string {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.staticDir
}

func (s *Server) swapRuntime(store *accounts.Store, syncClient *cliproxy.Client, staticDir string) *accounts.Store {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	previous := s.store
	s.store = store
	s.syncClient = syncClient
	s.staticDir = staticDir
	return previous
}

func (s *Server) buildSyncClientFromConfig() *cliproxy.Client {
	timeout := time.Duration(max(10, s.cfg.Sync.RequestTimeout)) * time.Second
	return cliproxy.New(s.cfg.Sync.Enabled, s.cfg.Sync.BaseURL, s.cfg.Sync.ManagementKey, s.cfg.Sync.ProviderType, timeout, s.cfg.SyncProxyURL())
}

func (s *Server) getNewAPIClient() *newapi.Client {
	key := newAPIClientCacheKey(s.cfg)
	s.sourceClientMu.Lock()
	defer s.sourceClientMu.Unlock()
	if s.cachedNewAPIClient != nil && s.cachedNewAPIKey == key {
		return s.cachedNewAPIClient
	}
	client := s.newAPIClientFactory(s.cfg)
	s.cachedNewAPIClient = client
	s.cachedNewAPIKey = key
	return client
}

func (s *Server) getSub2APIClient() *sub2api.Client {
	key := sub2APIClientCacheKey(s.cfg)
	s.sourceClientMu.Lock()
	defer s.sourceClientMu.Unlock()
	if s.cachedSub2APIClient != nil && s.cachedSub2APIKey == key {
		return s.cachedSub2APIClient
	}
	client := s.sub2apiClientFactory(s.cfg)
	s.cachedSub2APIClient = client
	s.cachedSub2APIKey = key
	return client
}

func (s *Server) getAccountRefreshRun() *accountRefreshRunResult {
	s.accountRefreshMu.RLock()
	defer s.accountRefreshMu.RUnlock()
	if s.accountRefreshRun == nil {
		return nil
	}
	copy := *s.accountRefreshRun
	return &copy
}

func (s *Server) setAccountRefreshRun(run *accountRefreshRunResult) {
	s.accountRefreshMu.Lock()
	defer s.accountRefreshMu.Unlock()
	if run == nil {
		s.accountRefreshRun = nil
		return
	}
	copy := *run
	s.accountRefreshRun = &copy
}

func (s *Server) finishAccountRefreshRun(run *accountRefreshRunResult) {
	if run == nil {
		return
	}
	run.Running = false
	run.Current = ""
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	run.UpdatedAt = run.FinishedAt
	s.setAccountRefreshRun(run)
}

func newAPIClientCacheKey(cfg *config.Config) string {
	values := []string{
		cfg.NewAPI.BaseURL,
		cfg.NewAPI.Username,
		cfg.NewAPI.Password,
		cfg.NewAPI.AccessToken,
		strconv.Itoa(cfg.NewAPI.UserID),
		cfg.NewAPI.SessionCookie,
		strconv.Itoa(cfg.NewAPI.RequestTimeout),
		cfg.SyncProxyURL(),
	}
	return strings.Join(values, "\x00")
}

func sub2APIClientCacheKey(cfg *config.Config) string {
	values := []string{
		cfg.Sub2API.BaseURL,
		cfg.Sub2API.Email,
		cfg.Sub2API.Password,
		cfg.Sub2API.APIKey,
		cfg.Sub2API.GroupID,
		strconv.Itoa(cfg.Sub2API.RequestTimeout),
		cfg.SyncProxyURL(),
	}
	return strings.Join(values, "\x00")
}

func (s *Server) reloadRuntimeDependencies(previous configPayload) error {
	nextStaticDir := s.cfg.ResolvePath(s.cfg.Server.StaticDir)
	nextSyncClient := s.buildSyncClientFromConfig()
	currentStore := s.getStore()
	nextStore := currentStore
	nextMailer := mailer.NewSender(s.cfg.Mail)
	nextVerifier := verification.NewStore(
		time.Duration(max(1, s.cfg.Mail.CodeTTLMinutes))*time.Minute,
		time.Duration(max(1, s.cfg.Mail.ResendInterval))*time.Second,
	)

	if storageSettingsChanged(previous, s.buildConfigPayload()) {
		reloadedStore, err := accounts.NewStore(s.cfg)
		if err != nil {
			return err
		}
		snapshot, err := currentStore.Snapshot()
		if err != nil {
			_ = reloadedStore.Close()
			return err
		}
		if err := reloadedStore.ReplaceAllData(snapshot); err != nil {
			_ = reloadedStore.Close()
			return err
		}
		nextStore = reloadedStore
	}
	if err := s.migrateImageFilesIfNeeded(previous, s.buildConfigPayload()); err != nil {
		if nextStore != currentStore && nextStore != nil {
			_ = nextStore.Close()
		}
		return err
	}

	previousStore := s.swapRuntime(nextStore, nextSyncClient, nextStaticDir)
	s.mailer = nextMailer
	s.verifier = nextVerifier
	if previousStore != nil && previousStore != nextStore {
		_ = previousStore.Close()
	}
	return nil
}

func (s *Server) migrateImageFilesIfNeeded(previous, next configPayload) error {
	oldDir := s.cfg.ResolvePath(previous.Storage.ImageDir)
	newDir := s.cfg.ResolvePath(next.Storage.ImageDir)
	if strings.EqualFold(filepath.Clean(oldDir), filepath.Clean(newDir)) {
		return nil
	}
	info, err := os.Stat(oldDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return err
	}
	normalizedNewDir := filepath.Clean(newDir)
	return filepath.Walk(oldDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if strings.HasPrefix(filepath.Clean(path)+string(os.PathSeparator), normalizedNewDir+string(os.PathSeparator)) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(oldDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(newDir, rel)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(targetPath); err == nil {
			return os.Remove(path)
		}
		if err := os.Rename(path, targetPath); err == nil {
			return nil
		}
		if err := copyFile(path, targetPath); err != nil {
			return err
		}
		return os.Remove(path)
	})
}

func copyFile(sourcePath, targetPath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return err
	}
	return nil
}

func storageSettingsChanged(previous, next configPayload) bool {
	return previous.Storage.Backend != next.Storage.Backend ||
		previous.Storage.ConfigBackend != next.Storage.ConfigBackend ||
		previous.Storage.AuthDir != next.Storage.AuthDir ||
		previous.Storage.StateFile != next.Storage.StateFile ||
		previous.Storage.SyncStateDir != next.Storage.SyncStateDir ||
		previous.Storage.SQLitePath != next.Storage.SQLitePath ||
		previous.Storage.ImageDir != next.Storage.ImageDir ||
		previous.Storage.ImageStorage != next.Storage.ImageStorage ||
		previous.Storage.ImageConversationStorage != next.Storage.ImageConversationStorage ||
		previous.Storage.ImageDataStorage != next.Storage.ImageDataStorage ||
		previous.Storage.RedisAddr != next.Storage.RedisAddr ||
		previous.Storage.RedisPassword != next.Storage.RedisPassword ||
		previous.Storage.RedisDB != next.Storage.RedisDB ||
		previous.Storage.RedisPrefix != next.Storage.RedisPrefix ||
		previous.Sync.ProviderType != next.Sync.ProviderType
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /auth/login", http.HandlerFunc(s.handleLogin))
	mux.Handle("GET /version", http.HandlerFunc(s.handleVersion))
	mux.Handle("GET /health", http.HandlerFunc(handleHealth))

	mux.Handle("POST /portal/api/register", http.HandlerFunc(s.handlePortalRegister))
	mux.Handle("POST /portal/api/register/code", http.HandlerFunc(s.handlePortalRegisterCode))
	mux.Handle("POST /portal/api/password/code", http.HandlerFunc(s.handlePortalPasswordCode))
	mux.Handle("POST /portal/api/password/reset", http.HandlerFunc(s.handlePortalPasswordReset))
	mux.Handle("POST /portal/api/login", http.HandlerFunc(s.handlePortalLogin))
	mux.Handle("POST /portal/api/logout", s.requirePortalUser(http.HandlerFunc(s.handlePortalLogout)))
	mux.Handle("GET /portal/api/me", s.requirePortalUser(http.HandlerFunc(s.handlePortalMe)))
	mux.Handle("PATCH /portal/api/me/profile", s.requirePortalUser(http.HandlerFunc(s.handlePortalUpdateProfile)))
	mux.Handle("POST /portal/api/me/password", s.requirePortalUser(http.HandlerFunc(s.handlePortalChangePassword)))
	mux.Handle("GET /portal/api/workspace/bootstrap", s.requirePortalUser(http.HandlerFunc(s.handlePortalWorkspaceBootstrap)))
	mux.Handle("GET /portal/api/workspace/accounts/{id}/quota", s.requirePortalUser(http.HandlerFunc(s.handlePortalAccountQuota)))
	mux.Handle("GET /portal/api/gallery/works", s.requirePortalUser(http.HandlerFunc(s.handlePortalGalleryWorks)))
	mux.Handle("POST /portal/api/gallery/works", s.requirePortalUser(http.HandlerFunc(s.handlePortalPublishGalleryWork)))
	mux.Handle("GET /portal/api/gallery/works/{id}", s.requirePortalUser(http.HandlerFunc(s.handlePortalGalleryWork)))
	mux.Handle("POST /portal/api/gallery/works/{id}/likes/toggle", s.requirePortalUser(http.HandlerFunc(s.handlePortalToggleGalleryLike)))
	mux.Handle("POST /portal/api/gallery/works/{id}/comments", s.requirePortalUser(http.HandlerFunc(s.handlePortalCreateGalleryComment)))
	mux.Handle("GET /portal/api/admin/users", s.requirePortalAdmin(http.HandlerFunc(s.handlePortalAdminUsers)))
	mux.Handle("PATCH /portal/api/admin/users/{id}", s.requirePortalAdmin(http.HandlerFunc(s.handlePortalAdminUpdateUser)))

	mux.Handle("GET /api/accounts", s.requireUIAuth(http.HandlerFunc(s.handleListAccounts)))
	mux.Handle("GET /api/accounts/{id}/quota", s.requireUIAuth(http.HandlerFunc(s.handleAccountQuota)))
	mux.Handle("POST /api/accounts", s.requireUIAuth(http.HandlerFunc(s.handleCreateAccounts)))
	mux.Handle("POST /api/accounts/import", s.requireUIAuth(http.HandlerFunc(s.handleImportAccounts)))
	mux.Handle("DELETE /api/accounts", s.requireUIAuth(http.HandlerFunc(s.handleDeleteAccounts)))
	mux.Handle("POST /api/accounts/refresh", s.requireUIAuth(http.HandlerFunc(s.handleRefreshAccounts)))
	mux.Handle("POST /api/accounts/refresh-all", s.requireUIAuth(http.HandlerFunc(s.handleRefreshAllAccounts)))
	mux.Handle("GET /api/accounts/refresh-progress", s.requireUIAuth(http.HandlerFunc(s.handleAccountRefreshProgress)))
	mux.Handle("POST /api/accounts/update", s.requireUIAuth(http.HandlerFunc(s.handleUpdateAccount)))
	mux.Handle("GET /api/config", s.requireUIAuth(http.HandlerFunc(s.handleGetConfig)))
	mux.Handle("GET /api/config/defaults", s.requireUIAuth(http.HandlerFunc(s.handleGetDefaultConfig)))
	mux.Handle("PUT /api/config", s.requireUIAuth(http.HandlerFunc(s.handleUpdateConfig)))
	mux.Handle("POST /api/proxy/test", s.requireUIAuth(http.HandlerFunc(s.handleProxyTest)))
	mux.Handle("POST /api/integration/test", s.requireUIAuth(http.HandlerFunc(s.handleIntegrationTest)))
	mux.Handle("POST /api/integration/newapi/token", s.requireUIAuth(http.HandlerFunc(s.handleNewAPITokenDiscover)))
	mux.Handle("POST /api/integration/sub2api/groups", s.requireUIAuth(http.HandlerFunc(s.handleSub2APIGroups)))
	mux.Handle("GET /api/requests", s.requireUIAuth(http.HandlerFunc(s.handleListRequestLogs)))
	mux.Handle("GET /api/startup/check", s.requireUIAuth(http.HandlerFunc(s.handleStartupCheck)))
	mux.Handle("GET /api/runtime/status", s.requireUIAuth(http.HandlerFunc(s.handleRuntimeStatus)))
	mux.Handle("GET /api/diagnostics/export", s.requireUIAuth(http.HandlerFunc(s.handleExportDiagnostics)))
	mux.Handle("POST /api/tools/admission-stress", s.requireUIAuth(http.HandlerFunc(s.handleAdmissionStress)))
	mux.Handle("GET /api/sync/status", s.requireUIAuth(http.HandlerFunc(s.handleSyncStatus)))
	mux.Handle("POST /api/sync/run", s.requireUIAuth(http.HandlerFunc(s.handleRunSync)))
	mux.Handle("GET /api/image/conversations", s.requireUIAuth(http.HandlerFunc(s.handleListImageConversations)))
	mux.Handle("DELETE /api/image/conversations", s.requireUIAuth(http.HandlerFunc(s.handleClearImageConversations)))
	mux.Handle("POST /api/image/conversations/import", s.requireUIAuth(http.HandlerFunc(s.handleImportImageConversations)))
	mux.Handle("GET /api/image/conversations/{id}", s.requireUIAuth(http.HandlerFunc(s.handleGetImageConversation)))
	mux.Handle("PUT /api/image/conversations/{id}", s.requireUIAuth(http.HandlerFunc(s.handleSaveImageConversation)))
	mux.Handle("DELETE /api/image/conversations/{id}", s.requireUIAuth(http.HandlerFunc(s.handleDeleteImageConversation)))

	mux.Handle("POST /v1/images/generations", s.requireImageAuth(http.HandlerFunc(s.handleImageGenerations)))
	mux.Handle("POST /v1/images/edits", s.requireImageAuth(http.HandlerFunc(s.handleImageEdits)))
	mux.Handle("POST /v1/chat/completions", s.requireImageAuth(http.HandlerFunc(s.handleImageChatCompletions)))
	mux.Handle("POST /v1/responses", s.requireImageAuth(http.HandlerFunc(s.handleImageResponses)))
	mux.Handle("GET /v1/models", s.requireImageAuth(http.HandlerFunc(s.handleModels)))
	mux.Handle("GET /v1/files/image/", http.HandlerFunc(s.handleImageFile))

	mux.Handle("/", http.HandlerFunc(s.handleWebApp))

	handler := middleware.RequestID(middleware.Logger(mux))
	return middleware.CORS(handler)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.hasExactBearer(r, s.cfg.App.AuthKey) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authorization is invalid"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": buildinfo.ResolveVersion(s.cfg.App.Version),
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   buildinfo.ResolveVersion(s.cfg.App.Version),
		"commit":    buildinfo.Commit,
		"buildTime": buildinfo.BuildTime,
	})
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	items, err := store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAccountQuota(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	accountID := strings.TrimSpace(r.PathValue("id"))
	if accountID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "account id is required"})
		return
	}

	account, err := s.findAccountByID(accountID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}

	refreshRequested := shouldRefreshAccountQuota(r)
	refreshed := false
	refreshError := ""
	if refreshRequested {
		_, refreshErrors, refreshErr := store.RefreshAccounts(r.Context(), []string{account.AccessToken})
		if refreshErr != nil {
			refreshError = refreshErr.Error()
		}
		if len(refreshErrors) > 0 {
			refreshError = firstNonEmpty(refreshErrors[0].Error, refreshError)
		}
		if refreshError == "" {
			if updated, updatedErr := store.GetAccountByToken(account.AccessToken); updatedErr == nil && updated != nil {
				account = *updated
			}
			refreshed = true
		}
	}

	imageGenRemaining, imageGenResetAfter := extractAccountQuota(account.LimitsProgress, "image_gen")
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                    account.ID,
		"email":                 account.Email,
		"status":                account.Status,
		"type":                  account.Type,
		"quota":                 account.Quota,
		"image_gen_remaining":   imageGenRemaining,
		"image_gen_reset_after": imageGenResetAfter,
		"refresh_requested":     refreshRequested,
		"refreshed":             refreshed,
		"refresh_error":         refreshError,
	})
}

func (s *Server) handleCreateAccounts(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	if s.configuredImageMode() != "studio" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "导入 Token 仅支持 Studio 模式"})
		return
	}
	var body struct {
		Tokens []string `json:"tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if len(nonEmptyStrings(body.Tokens)) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "tokens is required"})
		return
	}
	added, skipped, err := store.AddAccounts(body.Tokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	refreshed, refreshErrors, err := store.RefreshAccounts(r.Context(), body.Tokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	items, err := store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"added":     added,
		"skipped":   skipped,
		"refreshed": refreshed,
		"errors":    refreshErrors,
	})
}

func (s *Server) handleImportAccounts(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form"})
		return
	}

	files, err := readAuthFilesFromMultipart(r.MultipartForm)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "at least one auth json file is required"})
		return
	}

	imported, importedTokens, skipped, importFailures, err := store.ImportAuthFiles(files)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	refreshed := 0
	refreshErrors := []accounts.RefreshError{}
	if len(importedTokens) > 0 {
		refreshed, refreshErrors, err = store.RefreshAccounts(r.Context(), importedTokens)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	items, err := store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	status := http.StatusOK
	if len(importFailures) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, map[string]any{
		"items":          items,
		"imported":       imported,
		"imported_files": len(importedTokens),
		"duplicates":     skipped,
		"refreshed":      refreshed,
		"errors":         refreshErrors,
		"failed":         importFailures,
	})
}

func (s *Server) handleDeleteAccounts(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	var body struct {
		Tokens []string `json:"tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	removed, err := store.DeleteAccounts(body.Tokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	items, err := store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "removed": removed})
}

func (s *Server) handleRefreshAccounts(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	var body struct {
		AccessTokens []string `json:"access_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	refreshed, refreshErrors, err := store.RefreshAccounts(r.Context(), body.AccessTokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	items, err := store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"refreshed": refreshed,
		"errors":    refreshErrors,
	})
}

func (s *Server) handleRefreshAllAccounts(w http.ResponseWriter, r *http.Request) {
	if current := s.getAccountRefreshRun(); current != nil && current.Running {
		writeJSON(w, http.StatusOK, map[string]any{"progress": current, "alreadyRunning": true})
		return
	}

	items, err := s.getStore().ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	tokens := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.AccessToken) == "" {
			continue
		}
		tokens = append(tokens, item.AccessToken)
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)
	run := &accountRefreshRunResult{
		OK:        true,
		Running:   true,
		Total:     len(tokens),
		StartedAt: startedAt,
		UpdatedAt: startedAt,
	}
	s.setAccountRefreshRun(run)

	if len(tokens) == 0 {
		s.finishAccountRefreshRun(run)
		writeJSON(w, http.StatusOK, map[string]any{"progress": run})
		return
	}

	store := s.getStore()
	go func(tokens []string) {
		refreshed, refreshErrors, refreshErr := store.RefreshAccountsWithOptions(context.Background(), tokens, accounts.RefreshOptions{
			MaxWorkers: maxBulkAccountRefreshWorkers,
			Progress: func(progress accounts.RefreshProgress) {
				run.Refreshed = progress.Refreshed
				run.Failed = progress.Failed
				run.Processed = progress.Processed
				run.Current = progress.Current
				run.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				s.setAccountRefreshRun(run)
			},
		})
		if refreshErr != nil {
			run.OK = false
			run.Error = refreshErr.Error()
		} else if len(refreshErrors) > 0 {
			run.OK = false
			run.Error = firstNonEmpty(refreshErrors[0].Error, "")
		}
		run.Refreshed = refreshed
		run.Failed = len(refreshErrors)
		run.Processed = len(tokens)
		s.finishAccountRefreshRun(run)
	}(append([]string(nil), tokens...))

	writeJSON(w, http.StatusOK, map[string]any{"progress": run})
}

func (s *Server) handleAccountRefreshProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"progress": s.getAccountRefreshRun()})
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	store := s.getStore()
	var body struct {
		AccessToken string `json:"access_token"`
		Type        string `json:"type"`
		Status      string `json:"status"`
		Quota       *int   `json:"quota"`
		Note        string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	update := accounts.AccountUpdate{}
	if strings.TrimSpace(body.Type) != "" {
		update.Type = &body.Type
	}
	if strings.TrimSpace(body.Status) != "" {
		update.Status = &body.Status
	}
	if body.Quota != nil {
		update.Quota = body.Quota
	}
	if strings.TrimSpace(body.Note) != "" {
		update.Note = &body.Note
	}

	item, err := store.UpdateAccount(body.AccessToken, update)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	items, err := store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"item": item, "items": items})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	source := firstNonEmpty(r.URL.Query().Get("source"), "cpa")
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("progress_only")), "1") ||
		strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("progress_only")), "true") {
		writeJSON(w, http.StatusOK, buildSourceSyncProgressStatus(source, s.getSourceSyncRun(normalizeSyncSource(source))))
		return
	}
	status, err := s.buildSourceSyncStatus(r.Context(), source)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRunSync(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source    string `json:"source"`
		Direction string `json:"direction"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	source := firstNonEmpty(body.Source, r.URL.Query().Get("source"), "cpa")
	result, err := s.runSourceSync(r.Context(), source, body.Direction)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	status, statusErr := s.buildSourceSyncStatus(r.Context(), source)
	if statusErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"result": result})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result, "status": status})
}

func (s *Server) handleImageGenerations(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Model          string `json:"model"`
		Prompt         string `json:"prompt"`
		N              int    `json:"n"`
		Size           string `json:"size"`
		Quality        string `json:"quality"`
		Background     string `json:"background"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "prompt is required"})
		return
	}
	if req.N < 1 {
		req.N = 1
	}
	req.ResponseFormat = s.normalizeThirdPartyImageResponseFormat(r, req.ResponseFormat)

	payload, err := s.executeImageGeneration(r.Context(), imageGenerationRequest{
		Model:          req.Model,
		Prompt:         req.Prompt,
		N:              req.N,
		Size:           req.Size,
		Quality:        req.Quality,
		Background:     req.Background,
		ResponseFormat: req.ResponseFormat,
	}, r)
	if err != nil {
		writeImageRequestError(w, err)
		return
	}
	s.recordPortalUsage(r, 1, len(compatResponseDataItems(payload)))
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) normalizeThirdPartyImageResponseFormat(r *http.Request, requested string) string {
	responseFormat := strings.TrimSpace(requested)
	if responseFormat != "" && !strings.EqualFold(responseFormat, "url") {
		return responseFormat
	}
	if s.isThirdPartyImageAPIRequest(r) {
		return "b64_json"
	}
	return responseFormat
}

func (s *Server) handleImageEdits(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(int64(max(1, s.cfg.App.MaxUploadSizeMB)) << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form"})
		return
	}

	prompt := strings.TrimSpace(r.FormValue("prompt"))
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "prompt is required"})
		return
	}
	requestedModel := normalizeRequestedImageModel(r.FormValue("model"), s.cfg.ChatGPT.Model)
	responseFormat := firstNonEmpty(r.FormValue("response_format"), s.cfg.App.ImageFormat, "url")
	size := strings.TrimSpace(r.FormValue("size"))
	quality := strings.TrimSpace(r.FormValue("quality"))
	mask, err := readOptionalMultipartFile(r.MultipartForm, "mask")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	inpaintRequest := parseInpaintRequest(r)
	var data []map[string]any
	if inpaintRequest.originalFileID != "" && inpaintRequest.originalGenID != "" {
		if len(mask) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mask is required for selection edit"})
			return
		}
		data, err = s.withImageResultsWithMetadata(r.Context(), "selection-edit", responseFormat, inpaintRequest.sourceAccountID, requestedModel, false, newImageRequestMetadata(prompt, "", ""), func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
			return client.InpaintImageByMask(
				r.Context(),
				prompt,
				upstreamModel,
				inpaintRequest.originalFileID,
				inpaintRequest.originalGenID,
				inpaintRequest.conversationID,
				inpaintRequest.parentMessageID,
				mask,
			)
		}, r)
	} else {
		images, readErr := readImagesFromMultipart(r.MultipartForm)
		if readErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": readErr.Error()})
			return
		}
		if len(images) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "at least one image is required"})
			return
		}

		payload, execErr := s.executeImageEdit(r.Context(), imageEditRequest{
			Model:          requestedModel,
			Prompt:         prompt,
			Images:         images,
			Mask:           mask,
			Size:           size,
			Quality:        quality,
			ResponseFormat: responseFormat,
		}, r)
		if execErr != nil {
			err = execErr
		} else {
			data = compatResponseDataItems(payload)
		}
	}
	if err != nil {
		writeImageRequestError(w, err)
		return
	}
	s.recordPortalUsage(r, 1, len(data))
	writeJSON(w, http.StatusOK, map[string]any{"created": time.Now().Unix(), "data": data})
}

type imageRequestMetadata struct {
	size         string
	quality      string
	promptLength int
}

func (m imageRequestMetadata) applyTo(entry *imageRequestLogEntry) {
	if entry == nil {
		return
	}
	entry.Size = strings.TrimSpace(m.size)
	entry.Quality = strings.TrimSpace(m.quality)
	entry.PromptLength = m.promptLength
}

func newImageRequestMetadata(prompt, size, quality string) imageRequestMetadata {
	return imageRequestMetadata{
		size:         strings.TrimSpace(size),
		quality:      strings.TrimSpace(quality),
		promptLength: len([]rune(strings.TrimSpace(prompt))),
	}
}

func (s *Server) withImageResults(ctx context.Context, operation, responseFormat, preferredAccountID, requestedModel string, responsesEligible bool, run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error), r *http.Request) ([]map[string]any, error) {
	return s.withImageResultsWithMetadata(ctx, operation, responseFormat, preferredAccountID, requestedModel, responsesEligible, imageRequestMetadata{}, run, r)
}

func (s *Server) withImageResultsWithMetadata(ctx context.Context, operation, responseFormat, preferredAccountID, requestedModel string, responsesEligible bool, metadata imageRequestMetadata, run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error), r *http.Request) ([]map[string]any, error) {
	return s.withImageResultsFilteredWithMetadata(ctx, operation, responseFormat, preferredAccountID, requestedModel, responsesEligible, nil, metadata, run, r)
}

func (s *Server) withImageResultsFiltered(
	ctx context.Context,
	operation, responseFormat, preferredAccountID, requestedModel string,
	responsesEligible bool,
	allowAccount func(accounts.PublicAccount) bool,
	run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error),
	r *http.Request,
) ([]map[string]any, error) {
	return s.withImageResultsFilteredWithMetadata(ctx, operation, responseFormat, preferredAccountID, requestedModel, responsesEligible, allowAccount, imageRequestMetadata{}, run, r)
}

func (s *Server) withImageResultsFilteredWithMetadata(
	ctx context.Context,
	operation, responseFormat, preferredAccountID, requestedModel string,
	responsesEligible bool,
	allowAccount func(accounts.PublicAccount) bool,
	metadata imageRequestMetadata,
	run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error),
	r *http.Request,
) ([]map[string]any, error) {
	store := s.getStore()
	mode := s.configuredImageMode()
	if mode == "cpa" {
		return s.runPureCPAImageRequest(ctx, operation, responseFormat, requestedModel, strings.TrimSpace(preferredAccountID) != "", metadata, run, r)
	}
	policy, err := parseRequestImageAccountRoutingPolicy(r)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(preferredAccountID) != "" {
		authFile, account, releaseLease, err := store.FindImageAuthByIDWithLease(preferredAccountID)
		if err != nil {
			if errors.Is(err, accounts.ErrSourceAccountNotFound) {
				return nil, newRequestError("source_account_not_found", "原始图片所属账号不存在，请使用普通编辑重试")
			}
			return nil, err
		}
		data, _, err := s.runImageRequest(ctx, authFile, account, releaseLease, accounts.ImageAccountRoutingDecision{}, operation, responseFormat, true, requestedModel, responsesEligible, metadata, run, r)
		return data, err
	}

	attempted := map[string]struct{}{}
	var lastRetryableErr error
	for {
		var (
			authFile     *accounts.LocalAuth
			account      accounts.PublicAccount
			releaseLease func()
			decision     accounts.ImageAccountRoutingDecision
			err          error
		)
		if policy != nil {
			authFile, account, decision, releaseLease, err = store.AcquireImageAuthLeaseWithPolicyFilteredWithDisabledOption(attempted, allowAccount, s.allowDisabledStudioImageAccounts(), policy)
		} else {
			authFile, account, releaseLease, err = store.AcquireImageAuthLeaseFilteredWithDisabledOption(attempted, allowAccount, s.allowDisabledStudioImageAccounts())
		}
		if err != nil {
			return nil, resolveImageAcquireError(mode, err, lastRetryableErr)
		}
		attempted[authFile.AccessToken] = struct{}{}

		data, retryable, err := s.runImageRequest(ctx, authFile, account, releaseLease, decision, operation, responseFormat, false, requestedModel, responsesEligible, metadata, run, r)
		if retryable && len(attempted) < 64 {
			lastRetryableErr = err
			continue
		}
		return data, err
	}
}

func (s *Server) newOfficialWorkflowClient(accessToken string, authData map[string]any) imageWorkflowClient {
	if s != nil && s.officialClientFactory != nil {
		return s.officialClientFactory(accessToken, s.cfg.ChatGPTProxyURL(), authData, s.imageRequestConfig())
	}
	return handler.NewChatGPTClientWithProxyAndAuthData(accessToken, s.cfg.ChatGPTProxyURL(), authData, s.imageRequestConfig())
}

func (s *Server) newResponsesWorkflowClient(accessToken string, authData map[string]any) imageWorkflowClient {
	if s != nil && s.responsesClientFactory != nil {
		return s.responsesClientFactory(accessToken, s.cfg.ChatGPTProxyURL(), authData, s.imageRequestConfig())
	}
	return handler.NewResponsesClientWithProxyAndConfig(
		accessToken,
		s.cfg.ChatGPTProxyURL(),
		authData,
		s.imageRequestConfig(),
	)
}

func (s *Server) newCPAWorkflowClient() cpaRouteAwareImageWorkflowClient {
	timeout := time.Duration(max(10, s.cfg.CPAImageRequestTimeout())) * time.Second
	if s != nil && s.cpaClientFactory != nil {
		return s.cpaClientFactory(
			s.cfg.CPAImageBaseURL(),
			s.cfg.CPAImageAPIKey(),
			timeout,
			s.cfg.CPAImageRouteStrategy(),
		)
	}
	return newCPAImageClient(
		s.cfg.CPAImageBaseURL(),
		s.cfg.CPAImageAPIKey(),
		timeout,
		s.cfg.CPAImageRouteStrategy(),
	)
}

func resolveImageAcquireError(mode string, err, lastRetryableErr error) error {
	if errors.Is(err, accounts.ErrSelectedImageGroupsExhausted) {
		return newRequestError("selected_image_groups_exhausted", "当前选中的图片账号分组已经全部用尽，请调整分组或稍后重试")
	}
	if !errors.Is(err, accounts.ErrNoAvailableImageAuth) {
		return err
	}
	if lastRetryableErr != nil {
		return lastRetryableErr
	}
	if mode == "cpa" {
		return newRequestError("no_cpa_image_accounts", "当前没有可用的图片账号用于 CPA 模式")
	}
	return err
}

func (s *Server) runPureCPAImageRequest(
	ctx context.Context,
	operation string,
	responseFormat string,
	requestedModel string,
	preferredAccount bool,
	metadata imageRequestMetadata,
	run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error),
	r *http.Request,
) ([]map[string]any, error) {
	startedAt := time.Now()
	if !s.cfg.CPAImageConfigured() {
		err := newRequestError("cpa_image_not_configured", "CPA 图片接口还未配置，请先在配置管理中设置 CPA base_url 与 api_key")
		entry := imageRequestLogEntry{
			StartedAt:      startedAt.Format(time.RFC3339Nano),
			FinishedAt:     time.Now().Format(time.RFC3339Nano),
			Endpoint:       r.URL.Path,
			Operation:      operation,
			ImageMode:      "cpa",
			Direction:      "cpa",
			Route:          "cpa",
			CPASubroute:    s.cfg.CPAImageRouteStrategy(),
			RequestedModel: requestedModel,
			Preferred:      preferredAccount,
			Success:        false,
			Error:          err.Error(),
		}
		metadata.applyTo(&entry)
		s.logImageRequest(entry)
		return nil, err
	}

	admissionInfo, releaseAdmission, admissionErr := s.acquireImageAdmission(ctx)
	if admissionErr != nil {
		err := admissionErr
		if errors.Is(admissionErr, errImageAdmissionQueueFull) {
			err = newRequestError("image_queue_full", "当前图片请求排队已满，请稍后再试")
		} else if errors.Is(admissionErr, errImageAdmissionQueueTimeout) {
			err = newRequestError("image_queue_timeout", "当前图片请求排队超时，请稍后再试")
		}
		entry := imageRequestLogEntry{
			StartedAt:            startedAt.Format(time.RFC3339Nano),
			FinishedAt:           time.Now().Format(time.RFC3339Nano),
			Endpoint:             r.URL.Path,
			Operation:            operation,
			ImageMode:            "cpa",
			Direction:            "cpa",
			Route:                "cpa",
			CPASubroute:          s.cfg.CPAImageRouteStrategy(),
			RequestedModel:       requestedModel,
			Preferred:            preferredAccount,
			Success:              false,
			Error:                err.Error(),
			QueueWaitMS:          admissionInfo.QueueWaitMS,
			InflightCountAtStart: admissionInfo.InflightCountAtStart,
		}
		if requestErr, ok := err.(*requestError); ok {
			entry.ErrorCode = requestErr.code
		}
		metadata.applyTo(&entry)
		s.logImageRequest(entry)
		return nil, err
	}
	defer releaseAdmission()
	ctx = withImageAdmissionInfo(ctx, admissionInfo)

	client := s.newCPAWorkflowClient()
	upstreamModel := cpaFixedImageModel
	results, err := run(client, upstreamModel)
	cpaSubroute := client.LastRoute()
	if label := strings.TrimSpace(client.LastModelLabel()); label != "" {
		upstreamModel = label
	}
	if err != nil {
		admissionInfo := imageAdmissionFromContext(ctx)
		entry := imageRequestLogEntry{
			StartedAt:            startedAt.Format(time.RFC3339Nano),
			FinishedAt:           time.Now().Format(time.RFC3339Nano),
			Endpoint:             r.URL.Path,
			Operation:            operation,
			ImageMode:            "cpa",
			Direction:            "cpa",
			Route:                "cpa",
			CPASubroute:          cpaSubroute,
			RequestedModel:       requestedModel,
			UpstreamModel:        upstreamModel,
			Preferred:            preferredAccount,
			Success:              false,
			Error:                err.Error(),
			QueueWaitMS:          admissionInfo.QueueWaitMS,
			InflightCountAtStart: admissionInfo.InflightCountAtStart,
		}
		if requestErr, ok := err.(*requestError); ok {
			entry.ErrorCode = requestErr.code
		}
		metadata.applyTo(&entry)
		s.logImageRequest(entry)
		return nil, err
	}

	admissionInfo = imageAdmissionFromContext(ctx)
	entry := imageRequestLogEntry{
		StartedAt:            startedAt.Format(time.RFC3339Nano),
		FinishedAt:           time.Now().Format(time.RFC3339Nano),
		Endpoint:             r.URL.Path,
		Operation:            operation,
		ImageMode:            "cpa",
		Direction:            "cpa",
		Route:                "cpa",
		CPASubroute:          cpaSubroute,
		RequestedModel:       requestedModel,
		UpstreamModel:        upstreamModel,
		Preferred:            preferredAccount,
		Success:              true,
		QueueWaitMS:          admissionInfo.QueueWaitMS,
		InflightCountAtStart: admissionInfo.InflightCountAtStart,
	}
	metadata.applyTo(&entry)
	s.logImageRequest(entry)
	return buildImageResponse(r, client, results, responseFormat, "", s.cfg.ResolvePath(s.cfg.Storage.ImageDir)), nil
}

func (s *Server) runImageRequest(ctx context.Context, authFile *accounts.LocalAuth, account accounts.PublicAccount, releaseLease func(), routingDecision accounts.ImageAccountRoutingDecision, operation, responseFormat string, preferredAccount bool, requestedModel string, responsesEligible bool, metadata imageRequestMetadata, run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error), r *http.Request) ([]map[string]any, bool, error) {
	store := s.getStore()
	if releaseLease != nil {
		defer releaseLease()
	}
	startedAt := time.Now()
	now := time.Now()
	refreshRequired := account.SourceKind == accounts.AccountSourceKindToken || accounts.NeedsImageQuotaRefreshWithTTL(account, now, s.cfg.ImageQuotaRefreshTTL())
	if refreshRequired {
		_, refreshErrors, refreshErr := store.RefreshAccounts(ctx, []string{authFile.AccessToken})
		if refreshErr == nil {
			if refreshed, accountErr := store.GetAccountByToken(authFile.AccessToken); accountErr == nil && refreshed != nil {
				account = *refreshed
			}
		}
		if refreshErr != nil {
			if preferredAccount {
				return nil, false, newRequestError("source_account_quota_refresh_failed", "原始图片所属账号额度刷新失败，请稍后重试")
			}
			return nil, true, refreshErr
		}
		if len(refreshErrors) > 0 && isInvalidRefreshError(refreshErrors[0].Error) {
			store.MarkImageTokenAbnormal(authFile.AccessToken)
			if preferredAccount {
				return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
			}
			return nil, true, errors.New(refreshErrors[0].Error)
		}
		if len(refreshErrors) > 0 {
			if preferredAccount {
				return nil, false, newRequestError("source_account_quota_refresh_failed", firstNonEmpty(refreshErrors[0].Error, "原始图片所属账号额度刷新失败，请稍后重试"))
			}
			return nil, true, errors.New(firstNonEmpty(refreshErrors[0].Error, "image account quota refresh failed"))
		}
		if !isImageAccountUsable(account, s.allowDisabledStudioImageAccounts()) {
			if preferredAccount {
				return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
			}
			return nil, true, fmt.Errorf("image account is unavailable")
		}
	} else if !isImageAccountUsable(account, s.allowDisabledStudioImageAccounts()) {
		if preferredAccount {
			return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
		}
		return nil, true, fmt.Errorf("image account is unavailable")
	}
	if preferredAccount && !isImageAccountUsable(account, s.allowDisabledStudioImageAccounts()) {
		return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
	}
	if routingDecision.PolicyApplied && !store.ImageAccountAllowedForPolicy(authFile.AccessToken, account, &accounts.ImageAccountRoutingPolicy{
		Enabled:        true,
		SortMode:       routingDecision.SortMode,
		ReservePercent: routingDecision.ReservePercent,
		ReserveMode:    "daily_first_seen_percent",
	}) {
		return nil, true, newRequestError("image_account_reserved", "当前账号已触发分组保底阈值，正在切换下一个账号")
	}

	mode := s.configuredImageMode()
	admissionInfo, releaseAdmission, admissionErr := s.acquireImageAdmission(ctx)
	if admissionErr != nil {
		err := admissionErr
		if errors.Is(admissionErr, errImageAdmissionQueueFull) {
			err = newRequestError("image_queue_full", "当前图片请求排队已满，请稍后再试")
		} else if errors.Is(admissionErr, errImageAdmissionQueueTimeout) {
			err = newRequestError("image_queue_timeout", "当前图片请求排队超时，请稍后再试")
		}
		entry := imageRequestLogEntry{
			StartedAt:            startedAt.Format(time.RFC3339Nano),
			FinishedAt:           time.Now().Format(time.RFC3339Nano),
			Endpoint:             r.URL.Path,
			Operation:            operation,
			ImageMode:            mode,
			AccountType:          account.Type,
			AccountEmail:         account.Email,
			AccountFile:          authFile.Name,
			RequestedModel:       requestedModel,
			Preferred:            preferredAccount,
			Success:              false,
			Error:                err.Error(),
			LeaseAcquired:        releaseLease != nil,
			QueueWaitMS:          admissionInfo.QueueWaitMS,
			InflightCountAtStart: admissionInfo.InflightCountAtStart,
		}
		if requestErr, ok := err.(*requestError); ok {
			entry.ErrorCode = requestErr.code
		}
		applyImageRoutingLogFields(routingDecision, &entry)
		metadata.applyTo(&entry)
		s.logImageRequest(entry)
		return nil, false, err
	}
	defer releaseAdmission()
	ctx = withImageAdmissionInfo(ctx, admissionInfo)
	var (
		client         imageWorkflowClient
		upstreamModel  string
		route          string
		direction      string
		imageToolModel string
	)
	if shouldUseCPAImageRoute(mode) {
		if !s.cfg.CPAImageConfigured() {
			err := newRequestError("cpa_image_not_configured", "CPA 图片接口还未配置，请先在配置管理中设置 CPA base_url 与 api_key")
			entry := imageRequestLogEntry{
				StartedAt:            startedAt.Format(time.RFC3339Nano),
				FinishedAt:           time.Now().Format(time.RFC3339Nano),
				Endpoint:             r.URL.Path,
				Operation:            operation,
				ImageMode:            mode,
				Direction:            "cpa",
				Route:                "cpa",
				CPASubroute:          s.cfg.CPAImageRouteStrategy(),
				AccountType:          account.Type,
				AccountEmail:         account.Email,
				AccountFile:          authFile.Name,
				RequestedModel:       requestedModel,
				Preferred:            preferredAccount,
				Success:              false,
				Error:                err.Error(),
				LeaseAcquired:        releaseLease != nil,
				QueueWaitMS:          admissionInfo.QueueWaitMS,
				InflightCountAtStart: admissionInfo.InflightCountAtStart,
			}
			if requestErr, ok := err.(*requestError); ok {
				entry.ErrorCode = requestErr.code
			}
			applyImageRoutingLogFields(routingDecision, &entry)
			metadata.applyTo(&entry)
			s.logImageRequest(entry)
			return nil, false, err
		}
		client = s.newCPAWorkflowClient()
		upstreamModel = cpaFixedImageModel
		route = "cpa"
		direction = "cpa"
	} else {
		route = s.configuredImageRoute(account.Type)
		upstreamModel = s.resolveImageUpstreamModel(requestedModel, account.Type)
		direction = "official"
		if shouldUseOfficialResponses(preferredAccount, responsesEligible, route) {
			client = s.newResponsesWorkflowClient(authFile.AccessToken, authFile.Data)
		} else {
			client = s.newOfficialWorkflowClient(authFile.AccessToken, authFile.Data)
		}
	}
	if setter, ok := client.(interface{ SetRequestedImageModel(string) }); ok {
		setter.SetRequestedImageModel(requestedModel)
	}
	if toolModelProvider, ok := client.(interface{ ImageToolModel() string }); ok {
		imageToolModel = strings.TrimSpace(toolModelProvider.ImageToolModel())
	}
	results, err := run(client, upstreamModel)
	cpaSubroute := ""
	if cpaClient, ok := client.(cpaRouteAwareImageWorkflowClient); ok {
		cpaSubroute = cpaClient.LastRoute()
		if label := strings.TrimSpace(cpaClient.LastModelLabel()); label != "" {
			upstreamModel = label
		}
	}
	if route == "legacy" {
		if routeAwareClient, ok := client.(interface{ LastRoute() string }); ok {
			if actualRoute := strings.TrimSpace(routeAwareClient.LastRoute()); actualRoute != "" {
				route = actualRoute
			}
		}
	}
	if imageToolModel == "" {
		imageToolModel = strings.TrimSpace(resolveLoggedImageToolModel(requestedModel))
	}
	admissionInfo = imageAdmissionFromContext(ctx)
	if err != nil {
		store.RecordImageResult(authFile.AccessToken, false)
		entry := imageRequestLogEntry{
			StartedAt:            startedAt.Format(time.RFC3339Nano),
			FinishedAt:           time.Now().Format(time.RFC3339Nano),
			Endpoint:             r.URL.Path,
			Operation:            operation,
			ImageMode:            mode,
			Direction:            direction,
			Route:                route,
			CPASubroute:          cpaSubroute,
			AccountType:          account.Type,
			AccountEmail:         account.Email,
			AccountFile:          authFile.Name,
			RequestedModel:       requestedModel,
			UpstreamModel:        upstreamModel,
			ImageToolModel:       imageToolModel,
			Preferred:            preferredAccount,
			Success:              false,
			Error:                err.Error(),
			LeaseAcquired:        releaseLease != nil,
			QueueWaitMS:          admissionInfo.QueueWaitMS,
			InflightCountAtStart: admissionInfo.InflightCountAtStart,
		}
		if requestErr, ok := err.(*requestError); ok {
			entry.ErrorCode = requestErr.code
		}
		applyImageRoutingLogFields(routingDecision, &entry)
		metadata.applyTo(&entry)
		s.logImageRequest(entry)
		if isImageRateLimitError(err) {
			store.MarkImageAccountLimited(authFile.AccessToken)
			if preferredAccount {
				return nil, false, newRequestError("source_account_rate_limited", "原始图片所属账号当前已限流，请稍后重试或使用普通编辑")
			}
			return nil, true, err
		}
		if isInvalidImageTokenError(err) {
			store.MarkImageTokenAbnormal(authFile.AccessToken)
			if preferredAccount {
				return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
			}
			return nil, true, err
		}
		if preferredAccount && isConversationContextError(err) {
			return nil, false, newRequestError("source_context_missing", "原始图片对应会话已失效，请使用普通编辑重试")
		}
		return nil, false, err
	}

	store.RecordImageResult(authFile.AccessToken, true)
	entry := imageRequestLogEntry{
		StartedAt:            startedAt.Format(time.RFC3339Nano),
		FinishedAt:           time.Now().Format(time.RFC3339Nano),
		Endpoint:             r.URL.Path,
		Operation:            operation,
		ImageMode:            mode,
		Direction:            direction,
		Route:                route,
		CPASubroute:          cpaSubroute,
		AccountType:          account.Type,
		AccountEmail:         account.Email,
		AccountFile:          authFile.Name,
		RequestedModel:       requestedModel,
		UpstreamModel:        upstreamModel,
		ImageToolModel:       imageToolModel,
		Preferred:            preferredAccount,
		Success:              true,
		LeaseAcquired:        releaseLease != nil,
		QueueWaitMS:          admissionInfo.QueueWaitMS,
		InflightCountAtStart: admissionInfo.InflightCountAtStart,
	}
	applyImageRoutingLogFields(routingDecision, &entry)
	metadata.applyTo(&entry)
	s.logImageRequest(entry)
	return buildImageResponse(r, client, results, responseFormat, account.ID, s.cfg.ResolvePath(s.cfg.Storage.ImageDir)), false, nil
}

func normalizeRequestedImageModel(requested, fallback string) string {
	model := strings.TrimSpace(requested)
	if model != "" {
		return model
	}
	model = strings.TrimSpace(fallback)
	if model != "" {
		return model
	}
	return "gpt-image-2"
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models := s.availableModels()
	items := make([]map[string]any, 0, len(models))
	for index, model := range models {
		items = append(items, map[string]any{
			"id":       model,
			"object":   "model",
			"created":  1700000000 + index,
			"owned_by": "openai",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": items})
}

func (s *Server) availableModels() []string {
	seen := map[string]struct{}{}
	items := make([]string, 0)
	add := func(value string) {
		model := strings.TrimSpace(value)
		if model == "" {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		items = append(items, model)
	}

	add("gpt-image-2")
	add(strings.TrimSpace(s.cfg.ChatGPT.Model))

	accountsList, err := s.getStore().ListAccounts()
	hasFree := err != nil
	hasPaid := err != nil
	if err == nil {
		hasFree = false
		hasPaid = false
		for _, account := range accountsList {
			switch account.Type {
			case "Plus", "Pro", "Team":
				hasPaid = true
			case "Free":
				hasFree = true
			}
		}
	}

	if hasFree {
		add(s.cfg.ChatGPT.FreeImageModel)
	}
	if hasPaid {
		add(s.cfg.ChatGPT.PaidImageModel)
	}
	if strings.EqualFold(strings.TrimSpace(s.cfg.ChatGPT.FreeImageModel), "auto") {
		add("auto")
	}
	if s.cfg.ChatGPT.PaidImageRoute == "responses" || s.cfg.ChatGPT.FreeImageRoute == "responses" {
		add("gpt-5.4-mini")
		add("gpt-5.4")
		add("gpt-5.5")
		add("gpt-5-5-thinking")
	}
	return items
}

func (s *Server) handleWebApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	if target, ok := legacyPortalRedirectTarget(r.URL.Path); ok {
		http.Redirect(w, r, appendQuery(target, r.URL.RawQuery), http.StatusPermanentRedirect)
		return
	}

	if isAdminWebPath(r.URL.Path) {
		requestPath := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/admin"), "/")
		asset := resolveStaticAsset(s.getStaticDir(), requestPath)
		if asset == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, asset)
		return
	}

	if target, ok := legacyAdminRedirectTarget(r.URL.Path); ok {
		http.Redirect(w, r, appendQuery(target, r.URL.RawQuery), http.StatusPermanentRedirect)
		return
	}

	requestPath := strings.TrimPrefix(r.URL.Path, "/")
	asset := resolveStaticAsset(s.portalStaticDir, requestPath)
	if asset == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, asset)
}

func isAdminWebPath(path string) bool {
	return path == "/admin" || path == "/admin/" || strings.HasPrefix(path, "/admin/")
}

func legacyPortalRedirectTarget(path string) (string, bool) {
	if !(path == "/portal" || path == "/portal/" || strings.HasPrefix(path, "/portal/")) {
		return "", false
	}

	requestPath := strings.TrimPrefix(strings.TrimPrefix(path, "/portal"), "/")
	switch {
	case requestPath == "":
		return "/", true
	case requestPath == "admin/users" || strings.HasPrefix(requestPath, "admin/users/"):
		return "/users", true
	default:
		return "/" + requestPath, true
	}
}

func legacyAdminRedirectTarget(path string) (string, bool) {
	switch {
	case path == "/accounts" || strings.HasPrefix(path, "/accounts/"):
		return "/admin" + path, true
	case path == "/settings" || strings.HasPrefix(path, "/settings/"):
		return "/admin" + path, true
	case path == "/requests" || strings.HasPrefix(path, "/requests/"):
		return "/admin" + path, true
	case path == "/startup-check" || strings.HasPrefix(path, "/startup-check/"):
		return "/admin" + path, true
	case path == "/image" || strings.HasPrefix(path, "/image/"):
		return "/admin" + path, true
	case path == "/login.html" || strings.HasPrefix(path, "/login.html/"):
		return "/admin/login", true
	default:
		return "", false
	}
}

func appendQuery(path, rawQuery string) string {
	if strings.TrimSpace(rawQuery) == "" {
		return path
	}
	return path + "?" + rawQuery
}

func (s *Server) requireUIAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hasExactBearer(r, s.cfg.App.AuthKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authorization is invalid"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireImageAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.hasAnyBearer(r, append([]string{s.cfg.App.AuthKey}, parseKeys(s.cfg.App.APIKey)...)...) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.portalUserFromRequest(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authorization is invalid"})
	})
}

func (s *Server) hasAnyBearer(r *http.Request, keys ...string) bool {
	token := bearerFromRequest(r)
	if token == "" {
		return false
	}
	for _, key := range keys {
		if strings.TrimSpace(key) != "" && token == strings.TrimSpace(key) {
			return true
		}
	}
	return false
}

func (s *Server) hasExactBearer(r *http.Request, key string) bool {
	return strings.TrimSpace(key) != "" && bearerFromRequest(r) == strings.TrimSpace(key)
}

func (s *Server) isThirdPartyImageAPIRequest(r *http.Request) bool {
	token := bearerFromRequest(r)
	if token == "" || s == nil || s.cfg == nil {
		return false
	}
	if strings.TrimSpace(s.cfg.App.AuthKey) != "" && token == strings.TrimSpace(s.cfg.App.AuthKey) {
		return false
	}
	for _, key := range parseKeys(s.cfg.App.APIKey) {
		if token == key {
			return true
		}
	}
	return false
}

func bearerFromRequest(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func parseKeys(raw string) []string {
	result := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if cleaned := strings.TrimSpace(item); cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}

func resolveStaticAsset(staticDir, requestPath string) string {
	if strings.TrimSpace(staticDir) == "" {
		return ""
	}
	cleaned := strings.Trim(strings.TrimSpace(requestPath), "/")
	candidates := []string{}
	if cleaned == "" {
		candidates = append(candidates, filepath.Join(staticDir, "index.html"))
	} else {
		candidates = append(candidates,
			filepath.Join(staticDir, cleaned),
			filepath.Join(staticDir, cleaned, "index.html"),
			filepath.Join(staticDir, cleaned+".html"),
		)
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate
		}
	}
	if isStaticAssetRequest(cleaned) {
		return ""
	}
	indexPath := filepath.Join(staticDir, "index.html")
	if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
		return indexPath
	}
	return ""
}

func readImagesFromMultipart(form *multipart.Form) ([][]byte, error) {
	images := make([][]byte, 0)
	for _, key := range []string{"image", "image[]"} {
		files := form.File[key]
		for _, fileHeader := range files {
			data, err := readMultipartFile(fileHeader)
			if err != nil {
				return nil, err
			}
			images = append(images, data)
		}
	}

	for _, key := range []string{"image_base64", "imageBase64"} {
		if form.Value[key] == nil {
			continue
		}
		for _, raw := range form.Value[key] {
			decoded, err := decodeBase64Image(raw)
			if err != nil {
				return nil, err
			}
			images = append(images, decoded)
		}
	}
	return images, nil
}

func readAuthFilesFromMultipart(form *multipart.Form) ([]accounts.ImportedAuthFile, error) {
	if form == nil {
		return nil, nil
	}

	keys := make([]string, 0, len(form.File))
	for key := range form.File {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, nil
	}
	sort.Strings(keys)

	files := make([]accounts.ImportedAuthFile, 0)
	for _, key := range keys {
		for _, header := range form.File[key] {
			if header == nil {
				continue
			}
			data, err := readMultipartFile(header)
			if err != nil {
				return nil, err
			}
			files = append(files, accounts.ImportedAuthFile{
				Name: header.Filename,
				Data: data,
			})
		}
	}
	return files, nil
}

func readOptionalMultipartFile(form *multipart.Form, key string) ([]byte, error) {
	files := form.File[key]
	if len(files) == 0 {
		return nil, nil
	}
	return readMultipartFile(files[0])
}

type inpaintRequest struct {
	originalFileID  string
	originalGenID   string
	conversationID  string
	parentMessageID string
	sourceAccountID string
}

func parseInpaintRequest(r *http.Request) inpaintRequest {
	return inpaintRequest{
		originalFileID:  strings.TrimSpace(r.FormValue("original_file_id")),
		originalGenID:   strings.TrimSpace(r.FormValue("original_gen_id")),
		conversationID:  strings.TrimSpace(r.FormValue("conversation_id")),
		parentMessageID: strings.TrimSpace(r.FormValue("parent_message_id")),
		sourceAccountID: strings.TrimSpace(r.FormValue("source_account_id")),
	}
}

func readMultipartFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func decodeBase64Image(value string) ([]byte, error) {
	cleaned := strings.TrimSpace(value)
	if idx := strings.Index(cleaned, ","); idx >= 0 {
		cleaned = cleaned[idx+1:]
	}
	decoded, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 image")
	}
	return decoded, nil
}

func (s *Server) findAccountByID(accountID string) (accounts.PublicAccount, error) {
	items, err := s.getStore().ListAccounts()
	if err != nil {
		return accounts.PublicAccount{}, err
	}

	target := strings.TrimSpace(accountID)
	for _, item := range items {
		if item.ID == target {
			return item, nil
		}
	}
	return accounts.PublicAccount{}, fmt.Errorf("account not found")
}

func extractAccountQuota(limits []map[string]any, featureName string) (*int, string) {
	target := strings.TrimSpace(strings.ToLower(featureName))
	for _, item := range limits {
		if strings.TrimSpace(strings.ToLower(stringValue(item["feature_name"]))) != target {
			continue
		}

		var remaining *int
		switch typed := item["remaining"].(type) {
		case int:
			value := typed
			remaining = &value
		case int64:
			value := int(typed)
			remaining = &value
		case float64:
			value := int(typed)
			remaining = &value
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				value := int(parsed)
				remaining = &value
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				value := parsed
				remaining = &value
			}
		}

		return remaining, strings.TrimSpace(stringValue(item["reset_after"]))
	}

	return nil, ""
}

func shouldRefreshAccountQuota(r *http.Request) bool {
	value := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("refresh")))
	if value == "" {
		return true
	}
	switch value {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Server) portalGalleryStore() (*portalstore.Store, error) {
	if s == nil {
		return nil, fmt.Errorf("portal store is unavailable")
	}
	if s.portalStoreErr != nil {
		return nil, s.portalStoreErr
	}
	if s.portalStore == nil {
		return nil, fmt.Errorf("portal store is unavailable")
	}
	return s.portalStore, nil
}

func (s *Server) recordPortalUsage(r *http.Request, imageRequests, generatedImages int) {
	if r == nil {
		return
	}
	user, ok := s.portalUserFromRequest(r)
	if !ok {
		return
	}
	store, err := s.portalGalleryStore()
	if err != nil {
		slog.Warn("portal usage store unavailable", slog.Any("error", err))
		return
	}
	if err := store.IncrementUsage(r.Context(), user.ID, imageRequests, generatedImages, 0); err != nil {
		slog.Warn("record portal usage failed", slog.String("user_id", user.ID), slog.Any("error", err))
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func newRequestError(code, message string) error {
	return &requestError{
		code:    strings.TrimSpace(code),
		message: strings.TrimSpace(message),
	}
}

func requestErrorCode(err error) string {
	var typed *requestError
	if errors.As(err, &typed) {
		return typed.code
	}
	return ""
}

func writeImageRequestError(w http.ResponseWriter, err error) {
	if code := requestErrorCode(err); code != "" {
		writeAPIError(w, http.StatusBadGateway, code, err.Error())
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
}

func isInvalidImageTokenError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, token := range []string{"http 401", "status 401", "unauthorized", "invalid authentication", "invalid_token"} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func isImageRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, token := range []string{
		"http 429",
		" http 429",
		"http 429:",
		"http 429 ",
		"status 429",
		"too many requests",
		"rate limit",
		"rate_limit",
		"quota exceeded",
		"resource exhausted",
		"temporarily unavailable",
		"image generation limit",
		"image generation quota",
		"限流",
	} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func isConversationContextError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "conversation not found") ||
		strings.Contains(message, "conversation_not_found")
}

func isInvalidRefreshError(message string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(message)), "封号") ||
		strings.Contains(strings.ToLower(strings.TrimSpace(message)), "http 401")
}

func isImageAccountUsable(account accounts.PublicAccount, allowDisabled bool) bool {
	return (allowDisabled || account.Status != "禁用") &&
		account.Status != "异常" &&
		account.Status != "限流" &&
		account.Quota > 0
}

func (s *Server) allowDisabledStudioImageAccounts() bool {
	return s != nil &&
		s.cfg != nil &&
		s.configuredImageMode() == "studio" &&
		s.cfg.ChatGPT.StudioAllowDisabledImageAccounts
}

func (s *Server) configuredImageMode() string {
	if normalized, ok := config.NormalizeImageModeForAPI(s.cfg.ChatGPT.ImageMode); ok {
		return normalized
	}
	return "studio"
}

func shouldUseCPAImageRoute(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "cpa")
}

func isPaidImageAccountType(accountType string) bool {
	switch strings.TrimSpace(accountType) {
	case "Plus", "Pro", "Team":
		return true
	default:
		return false
	}
}

func shouldUseOfficialResponses(preferredAccount bool, responsesEligible bool, configuredRoute string) bool {
	if preferredAccount {
		return false
	}
	if !responsesEligible {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(configuredRoute), "responses")
}

func (s *Server) configuredImageRoute(accountType string) string {
	switch strings.TrimSpace(accountType) {
	case "Plus", "Pro", "Team":
		return normalizeConfiguredImageRoute(s.cfg.ChatGPT.PaidImageRoute, "responses")
	default:
		return normalizeConfiguredImageRoute(s.cfg.ChatGPT.FreeImageRoute, "legacy")
	}
}

func (s *Server) imageRequestConfig() handler.ImageRequestConfig {
	return handler.ImageRequestConfig{
		RequestTimeout: time.Duration(max(1, s.cfg.ChatGPT.RequestTimeout)) * time.Second,
		SSETimeout:     time.Duration(max(1, s.cfg.ChatGPT.SSETimeout)) * time.Second,
		PollInterval:   time.Duration(max(1, s.cfg.ChatGPT.PollInterval)) * time.Second,
		PollMaxWait:    time.Duration(max(1, s.cfg.ChatGPT.PollMaxWait)) * time.Second,
	}
}

func (s *Server) resolveImageUpstreamModel(requestedModel, accountType string) string {
	return handler.ResolveImageUpstreamModelWithDefaults(
		requestedModel,
		accountType,
		s.cfg.ChatGPT.FreeImageModel,
		s.cfg.ChatGPT.PaidImageModel,
	)
}

func normalizeConfiguredImageRoute(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return strings.ToLower(strings.TrimSpace(fallback))
	case "legacy", "conversation":
		return "legacy"
	case "responses":
		return "responses"
	default:
		return strings.ToLower(strings.TrimSpace(fallback))
	}
}

func resolveLoggedImageToolModel(requestedModel string) string {
	switch strings.ToLower(strings.TrimSpace(requestedModel)) {
	case "gpt-image-1":
		return "gpt-image-1"
	case "gpt-image-2":
		return "gpt-image-2"
	default:
		return ""
	}
}

func (s *Server) logImageRequest(entry imageRequestLogEntry) {
	if s == nil || s.reqLogs == nil {
		return
	}
	s.reqLogs.add(entry)
}

func isStaticAssetRequest(path string) bool {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return false
	}
	if strings.HasPrefix(cleaned, "_next/") {
		return true
	}
	return strings.Contains(filepath.Base(cleaned), ".")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if cleaned := strings.TrimSpace(value); cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}
