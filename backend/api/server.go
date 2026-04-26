package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"chatgpt2api/handler"
	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/buildinfo"
	"chatgpt2api/internal/cliproxy"
	"chatgpt2api/internal/config"
	"chatgpt2api/internal/middleware"
)

type Server struct {
	cfg        *config.Config
	store      *accounts.Store
	syncClient *cliproxy.Client
	staticDir  string
	reqLogs    *imageRequestLogStore
}

type requestError struct {
	code    string
	message string
}

const cpaFixedImageModel = "gpt-image-2"

func (e *requestError) Error() string {
	return firstNonEmpty(e.message, e.code)
}

func NewServer(cfg *config.Config, store *accounts.Store, syncClient *cliproxy.Client) *Server {
	return &Server{
		cfg:        cfg,
		store:      store,
		syncClient: syncClient,
		staticDir:  cfg.ResolvePath(cfg.Server.StaticDir),
		reqLogs:    newImageRequestLogStore(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /auth/login", http.HandlerFunc(s.handleLogin))
	mux.Handle("GET /version", http.HandlerFunc(s.handleVersion))
	mux.Handle("GET /health", http.HandlerFunc(handleHealth))

	mux.Handle("GET /api/accounts", s.requireUIAuth(http.HandlerFunc(s.handleListAccounts)))
	mux.Handle("GET /api/accounts/{id}/quota", s.requireUIAuth(http.HandlerFunc(s.handleAccountQuota)))
	mux.Handle("POST /api/accounts", s.requireUIAuth(http.HandlerFunc(s.handleCreateAccounts)))
	mux.Handle("POST /api/accounts/import", s.requireUIAuth(http.HandlerFunc(s.handleImportAccounts)))
	mux.Handle("DELETE /api/accounts", s.requireUIAuth(http.HandlerFunc(s.handleDeleteAccounts)))
	mux.Handle("POST /api/accounts/refresh", s.requireUIAuth(http.HandlerFunc(s.handleRefreshAccounts)))
	mux.Handle("POST /api/accounts/update", s.requireUIAuth(http.HandlerFunc(s.handleUpdateAccount)))
	mux.Handle("GET /api/config", s.requireUIAuth(http.HandlerFunc(s.handleGetConfig)))
	mux.Handle("GET /api/config/defaults", s.requireUIAuth(http.HandlerFunc(s.handleGetDefaultConfig)))
	mux.Handle("PUT /api/config", s.requireUIAuth(http.HandlerFunc(s.handleUpdateConfig)))
	mux.Handle("GET /api/requests", s.requireUIAuth(http.HandlerFunc(s.handleListRequestLogs)))
	mux.Handle("GET /api/sync/status", s.requireUIAuth(http.HandlerFunc(s.handleSyncStatus)))
	mux.Handle("POST /api/sync/run", s.requireUIAuth(http.HandlerFunc(s.handleRunSync)))

	mux.Handle("POST /v1/images/generations", s.requireImageAuth(http.HandlerFunc(s.handleImageGenerations)))
	mux.Handle("POST /v1/images/edits", s.requireImageAuth(http.HandlerFunc(s.handleImageEdits)))
	mux.Handle("POST /v1/images/upscale", s.requireImageAuth(http.HandlerFunc(s.handleImageUpscale)))
	mux.Handle("POST /v1/chat/completions", s.requireImageAuth(http.HandlerFunc(s.handleImageChatCompletions)))
	mux.Handle("POST /v1/responses", s.requireImageAuth(http.HandlerFunc(s.handleImageResponses)))
	mux.Handle("GET /v1/models", s.requireImageAuth(http.HandlerFunc(s.handleModels)))
	mux.Handle("GET /v1/files/image/", handleImageFile())

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
	items, err := s.store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleAccountQuota(w http.ResponseWriter, r *http.Request) {
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
		_, refreshErrors, refreshErr := s.store.RefreshAccounts(r.Context(), []string{account.AccessToken})
		if refreshErr != nil {
			refreshError = refreshErr.Error()
		}
		if len(refreshErrors) > 0 {
			refreshError = firstNonEmpty(refreshErrors[0].Error, refreshError)
		}
		if refreshError == "" {
			if updated, updatedErr := s.store.GetAccountByToken(account.AccessToken); updatedErr == nil && updated != nil {
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
	added, skipped, err := s.store.AddAccounts(body.Tokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	refreshed, refreshErrors, err := s.store.RefreshAccounts(r.Context(), body.Tokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	items, err := s.store.ListAccounts()
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

	imported, importedTokens, skipped, importFailures, err := s.store.ImportAuthFiles(files)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	refreshed := 0
	refreshErrors := []accounts.RefreshError{}
	if len(importedTokens) > 0 {
		refreshed, refreshErrors, err = s.store.RefreshAccounts(r.Context(), importedTokens)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	items, err := s.store.ListAccounts()
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
	var body struct {
		Tokens []string `json:"tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	removed, err := s.store.DeleteAccounts(body.Tokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	items, err := s.store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "removed": removed})
}

func (s *Server) handleRefreshAccounts(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccessTokens []string `json:"access_tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	refreshed, refreshErrors, err := s.store.RefreshAccounts(r.Context(), body.AccessTokens)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	items, err := s.store.ListAccounts()
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

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
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

	item, err := s.store.UpdateAccount(body.AccessToken, update)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	items, err := s.store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"item": item, "items": items})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.store.SyncStatus(r.Context(), s.syncClient)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRunSync(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Direction string `json:"direction"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	result, err := s.store.RunSync(r.Context(), s.syncClient, body.Direction)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	status, statusErr := s.store.SyncStatus(r.Context(), s.syncClient)
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
		data, err = s.withImageResults(r.Context(), "selection-edit", responseFormat, inpaintRequest.sourceAccountID, requestedModel, false, func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
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
	writeJSON(w, http.StatusOK, map[string]any{"created": time.Now().Unix(), "data": data})
}

func (s *Server) handleImageUpscale(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(int64(max(1, s.cfg.App.MaxUploadSizeMB)) << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form"})
		return
	}

	images, err := readImagesFromMultipart(r.MultipartForm)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if len(images) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "at least one image is required"})
		return
	}

	scale := firstNonEmpty(r.FormValue("scale"), "2x")
	extraPrompt := strings.TrimSpace(r.FormValue("prompt"))
	prompt := fmt.Sprintf("Upscale and enhance this image to %s. Preserve composition, identity, colors, and style while improving sharpness, clarity, and fine detail.", scale)
	if extraPrompt != "" {
		prompt += " " + extraPrompt
	}
	requestedModel := normalizeRequestedImageModel(r.FormValue("model"), s.cfg.ChatGPT.Model)
	responseFormat := firstNonEmpty(r.FormValue("response_format"), s.cfg.App.ImageFormat, "url")

	data, err := s.withImageResults(r.Context(), "upscale", responseFormat, "", requestedModel, handler.SupportsResponsesInlineEdit([][]byte{images[0]}, nil), func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error) {
		return client.EditImageByUpload(r.Context(), prompt, upstreamModel, [][]byte{images[0]}, nil)
	}, r)
	if err != nil {
		writeImageRequestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": time.Now().Unix(), "data": data})
}

func (s *Server) withImageResults(ctx context.Context, operation, responseFormat, preferredAccountID, requestedModel string, responsesEligible bool, run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error), r *http.Request) ([]map[string]any, error) {
	mode := s.configuredImageMode()
	if mode == "cpa" {
		return s.runPureCPAImageRequest(ctx, operation, responseFormat, requestedModel, strings.TrimSpace(preferredAccountID) != "", run, r)
	}
	if strings.TrimSpace(preferredAccountID) != "" {
		authFile, account, err := s.store.FindImageAuthByID(preferredAccountID)
		if err != nil {
			if errors.Is(err, accounts.ErrSourceAccountNotFound) {
				return nil, newRequestError("source_account_not_found", "原始图片所属账号不存在，请使用普通编辑重试")
			}
			return nil, err
		}
		data, _, err := s.runImageRequest(ctx, authFile, account, operation, responseFormat, true, requestedModel, responsesEligible, run, r)
		return data, err
	}

	attempted := map[string]struct{}{}
	var lastRetryableErr error
	for {
		authFile, account, err := s.store.AcquireImageAuthFiltered(attempted, nil)
		if err != nil {
			return nil, resolveImageAcquireError(mode, err, lastRetryableErr)
		}
		attempted[authFile.AccessToken] = struct{}{}

		data, retryable, err := s.runImageRequest(ctx, authFile, account, operation, responseFormat, false, requestedModel, responsesEligible, run, r)
		if retryable && len(attempted) < 64 {
			lastRetryableErr = err
			continue
		}
		return data, err
	}
}

func resolveImageAcquireError(mode string, err, lastRetryableErr error) error {
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
	run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error),
	r *http.Request,
) ([]map[string]any, error) {
	startedAt := time.Now()
	if !s.cfg.CPAImageConfigured() {
		err := newRequestError("cpa_image_not_configured", "CPA 图片接口还未配置，请先在配置管理中设置 CPA base_url 与 api_key")
		s.logImageRequest(imageRequestLogEntry{
			StartedAt:      startedAt.Format(time.RFC3339Nano),
			FinishedAt:     time.Now().Format(time.RFC3339Nano),
			Endpoint:       r.URL.Path,
			Operation:      operation,
			ImageMode:      "cpa",
			Direction:      "cpa",
			Route:          "cpa",
			RequestedModel: requestedModel,
			Preferred:      preferredAccount,
			Success:        false,
			Error:          err.Error(),
		})
		return nil, err
	}

	client := newCPAImageClient(
		s.cfg.CPAImageBaseURL(),
		s.cfg.CPAImageAPIKey(),
		time.Duration(max(10, s.cfg.CPAImageRequestTimeout()))*time.Second,
	)
	upstreamModel := cpaFixedImageModel
	results, err := run(client, upstreamModel)
	if err != nil {
		s.logImageRequest(imageRequestLogEntry{
			StartedAt:      startedAt.Format(time.RFC3339Nano),
			FinishedAt:     time.Now().Format(time.RFC3339Nano),
			Endpoint:       r.URL.Path,
			Operation:      operation,
			ImageMode:      "cpa",
			Direction:      "cpa",
			Route:          "cpa",
			RequestedModel: requestedModel,
			UpstreamModel:  upstreamModel,
			Preferred:      preferredAccount,
			Success:        false,
			Error:          err.Error(),
		})
		return nil, err
	}

	s.logImageRequest(imageRequestLogEntry{
		StartedAt:      startedAt.Format(time.RFC3339Nano),
		FinishedAt:     time.Now().Format(time.RFC3339Nano),
		Endpoint:       r.URL.Path,
		Operation:      operation,
		ImageMode:      "cpa",
		Direction:      "cpa",
		Route:          "cpa",
		RequestedModel: requestedModel,
		UpstreamModel:  upstreamModel,
		Preferred:      preferredAccount,
		Success:        true,
	})
	return buildImageResponse(r, client, results, responseFormat, ""), nil
}

func (s *Server) runImageRequest(ctx context.Context, authFile *accounts.LocalAuth, account accounts.PublicAccount, operation, responseFormat string, preferredAccount bool, requestedModel string, responsesEligible bool, run func(client imageWorkflowClient, upstreamModel string) ([]handler.ImageResult, error), r *http.Request) ([]map[string]any, bool, error) {
	startedAt := time.Now()
	refreshRequired := accounts.NeedsImageQuotaRefresh(account, time.Now())
	if refreshRequired {
		_, refreshErrors, refreshErr := s.store.RefreshAccounts(ctx, []string{authFile.AccessToken})
		if refreshErr == nil {
			if refreshed, accountErr := s.store.GetAccountByToken(authFile.AccessToken); accountErr == nil && refreshed != nil {
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
			s.store.MarkImageTokenAbnormal(authFile.AccessToken)
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
		if !isImageAccountUsable(account) {
			if preferredAccount {
				return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
			}
			return nil, true, fmt.Errorf("image account is unavailable")
		}
	} else if !isImageAccountUsable(account) {
		if preferredAccount {
			return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
		}
		return nil, true, fmt.Errorf("image account is unavailable")
	}
	if preferredAccount && !isImageAccountUsable(account) {
		return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
	}

	mode := s.configuredImageMode()
	var (
		client        imageWorkflowClient
		upstreamModel string
		route         string
		direction     string
	)
	if shouldUseCPAImageRoute(mode, account.Type) {
		if !s.cfg.CPAImageConfigured() {
			err := newRequestError("cpa_image_not_configured", "CPA 图片接口还未配置，请先在配置管理中设置 CPA base_url 与 api_key")
			s.logImageRequest(imageRequestLogEntry{
				StartedAt:      startedAt.Format(time.RFC3339Nano),
				FinishedAt:     time.Now().Format(time.RFC3339Nano),
				Endpoint:       r.URL.Path,
				Operation:      operation,
				ImageMode:      mode,
				Direction:      "cpa",
				Route:          "cpa",
				AccountType:    account.Type,
				AccountEmail:   account.Email,
				AccountFile:    authFile.Name,
				RequestedModel: requestedModel,
				Preferred:      preferredAccount,
				Success:        false,
				Error:          err.Error(),
			})
			if mode == "mix" && !preferredAccount {
				return nil, true, err
			}
			return nil, false, err
		}
		client = newCPAImageClient(
			s.cfg.CPAImageBaseURL(),
			s.cfg.CPAImageAPIKey(),
			time.Duration(max(10, s.cfg.CPAImageRequestTimeout()))*time.Second,
		)
		upstreamModel = cpaFixedImageModel
		route = "cpa"
		direction = "cpa"
	} else {
		route = s.configuredImageRoute(account.Type)
		upstreamModel = s.resolveImageUpstreamModel(requestedModel, account.Type)
		direction = "official"
		if shouldUseOfficialResponses(preferredAccount, responsesEligible, route) {
			client = handler.NewResponsesClientWithProxyAndConfig(
				authFile.AccessToken,
				s.cfg.ChatGPTProxyURL(),
				authFile.Data,
				s.imageRequestConfig(),
			)
		} else {
			client = handler.NewChatGPTClientWithProxyAndConfig(
				authFile.AccessToken,
				firstNonEmpty(stringValue(authFile.Data["cookies"]), stringValue(authFile.Data["cookie"])),
				s.cfg.ChatGPTProxyURL(),
				s.imageRequestConfig(),
			)
		}
	}
	results, err := run(client, upstreamModel)
	if err != nil {
		s.store.RecordImageResult(authFile.AccessToken, false)
		s.logImageRequest(imageRequestLogEntry{
			StartedAt:      startedAt.Format(time.RFC3339Nano),
			FinishedAt:     time.Now().Format(time.RFC3339Nano),
			Endpoint:       r.URL.Path,
			Operation:      operation,
			ImageMode:      mode,
			Direction:      direction,
			Route:          route,
			AccountType:    account.Type,
			AccountEmail:   account.Email,
			AccountFile:    authFile.Name,
			RequestedModel: requestedModel,
			UpstreamModel:  upstreamModel,
			Preferred:      preferredAccount,
			Success:        false,
			Error:          err.Error(),
		})
		if isInvalidImageTokenError(err) {
			s.store.MarkImageTokenAbnormal(authFile.AccessToken)
			if preferredAccount {
				return nil, false, newRequestError("source_account_unavailable", "原始图片所属账号当前不可用，请使用普通编辑重试")
			}
			return nil, true, err
		}
		if !preferredAccount && isRetryableImageRequestError(direction, err) {
			return nil, true, err
		}
		if preferredAccount && isConversationContextError(err) {
			return nil, false, newRequestError("source_context_missing", "原始图片对应会话已失效，请使用普通编辑重试")
		}
		return nil, false, err
	}

	s.store.RecordImageResult(authFile.AccessToken, true)
	s.logImageRequest(imageRequestLogEntry{
		StartedAt:      startedAt.Format(time.RFC3339Nano),
		FinishedAt:     time.Now().Format(time.RFC3339Nano),
		Endpoint:       r.URL.Path,
		Operation:      operation,
		ImageMode:      mode,
		Direction:      direction,
		Route:          route,
		AccountType:    account.Type,
		AccountEmail:   account.Email,
		AccountFile:    authFile.Name,
		RequestedModel: requestedModel,
		UpstreamModel:  upstreamModel,
		Preferred:      preferredAccount,
		Success:        true,
	})
	return buildImageResponse(r, client, results, responseFormat, account.ID), false, nil
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
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"id": "gpt-image-1", "object": "model", "created": 1700000000, "owned_by": "openai"},
			{"id": "gpt-image-2", "object": "model", "created": 1700000001, "owned_by": "openai"},
		},
	})
}

func (s *Server) handleWebApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	requestPath := strings.TrimPrefix(r.URL.Path, "/")
	asset := resolveStaticAsset(s.staticDir, requestPath)
	if asset == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, asset)
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
	items, err := s.store.ListAccounts()
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

func isRetryableImageRequestError(direction string, err error) bool {
	if err == nil {
		return false
	}
	if strings.TrimSpace(direction) != "official" {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	if strings.Contains(message, "sse read error") {
		return false
	}
	return strings.Contains(message, "conversation request:") ||
		strings.Contains(message, "responses request:") ||
		strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, ": eof") ||
		strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "tls handshake timeout") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "dial tcp")
}

func isImageAccountUsable(account accounts.PublicAccount) bool {
	return account.Status != "禁用" &&
		account.Status != "异常" &&
		account.Status != "限流" &&
		account.Quota > 0
}

func (s *Server) configuredImageMode() string {
	if normalized, ok := config.NormalizeImageModeForAPI(s.cfg.ChatGPT.ImageMode); ok {
		return normalized
	}
	return "studio"
}

func shouldUseCPAImageRoute(mode, accountType string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "cpa":
		return true
	case "mix":
		return isPaidImageAccountType(accountType)
	default:
		return false
	}
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
