package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/portalstore"
	"chatgpt2api/internal/users"
	"chatgpt2api/internal/verification"
)

const (
	portalSessionCookieName = "studio_portal_session"
	portalSessionTTL        = 7 * 24 * time.Hour
)

type portalContextKey string

const portalUserContextKey portalContextKey = "portal_user"

type portalUserUsageResponse struct {
	ImageRequests   int `json:"image_requests"`
	GeneratedImages int `json:"generated_images"`
	PublishedWorks  int `json:"published_works"`
}

type portalUserResponse struct {
	ID          string                  `json:"id"`
	Email       string                  `json:"email"`
	Role        string                  `json:"role"`
	Disabled    bool                    `json:"disabled"`
	CreatedAt   string                  `json:"created_at"`
	LastLoginAt string                  `json:"last_login_at,omitempty"`
	Usage       portalUserUsageResponse `json:"usage"`
}

type portalQuotaSummary struct {
	Accounts          int `json:"accounts"`
	AvailableAccounts int `json:"available_accounts"`
	TotalQuota        int `json:"total_quota"`
	AvailableQuota    int `json:"available_quota"`
	PaidAccounts      int `json:"paid_accounts"`
}

type portalWorkspaceAccount struct {
	ID             string           `json:"id"`
	Type           string           `json:"type"`
	Status         string           `json:"status"`
	Quota          int              `json:"quota"`
	LimitsProgress []map[string]any `json:"limits_progress"`
	RestoreAt      string           `json:"restoreAt,omitempty"`
	Disabled       bool             `json:"disabled"`
}

func (s *Server) handlePortalRegisterCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if s.mailer == nil || !s.mailer.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "mail sender is not configured"})
		return
	}
	if s.userStore.EmailExists(body.Email) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email already exists"})
		return
	}

	entry, err := s.verifier.Issue(body.Email)
	if err != nil {
		writeJSON(w, statusForVerificationError(err), map[string]any{"error": err.Error()})
		return
	}
	if err := s.mailer.SendVerificationCode(strings.TrimSpace(body.Email), entry.Code); err != nil {
		s.verifier.Consume(body.Email)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"expires_in_seconds": max(60, int(time.Until(entry.ExpiresAt).Seconds())),
		"resend_in_seconds":  max(1, int(time.Until(entry.ResendAt).Seconds())),
		"delivery":           "email",
	})
}

func (s *Server) handlePortalRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if err := s.verifier.Verify(body.Email, body.Code); err != nil {
		writeJSON(w, statusForVerificationError(err), map[string]any{"error": err.Error()})
		return
	}

	user, err := s.userStore.Register(body.Email, body.Password)
	if err != nil {
		writeJSON(w, statusForUserError(err), map[string]any{"error": err.Error()})
		return
	}

	s.verifier.Consume(body.Email)
	s.setPortalSessionCookie(w, r, user.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":  s.decoratePortalUser(r.Context(), user),
		"quota": s.buildPortalQuotaSummary(),
	})
}

func (s *Server) handlePortalLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	user, err := s.userStore.Authenticate(body.Email, body.Password)
	if err != nil {
		writeJSON(w, statusForUserError(err), map[string]any{"error": err.Error()})
		return
	}

	s.setPortalSessionCookie(w, r, user.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":  s.decoratePortalUser(r.Context(), user),
		"quota": s.buildPortalQuotaSummary(),
	})
}

func (s *Server) handlePortalLogout(w http.ResponseWriter, r *http.Request) {
	s.clearPortalSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePortalMe(w http.ResponseWriter, r *http.Request) {
	user, ok := portalUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":  s.decoratePortalUser(r.Context(), user),
		"quota": s.buildPortalQuotaSummary(),
	})
}

func (s *Server) handlePortalWorkspaceBootstrap(w http.ResponseWriter, r *http.Request) {
	user, ok := portalUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}

	items, err := s.store.ListAccounts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":     s.decoratePortalUser(r.Context(), user),
		"quota":    s.buildPortalQuotaSummary(),
		"accounts": s.buildPortalWorkspaceAccounts(items),
		"workspace": map[string]any{
			"allow_disabled_studio_accounts": s.cfg.ChatGPT.ImageMode == "studio" && s.cfg.ChatGPT.StudioAllowDisabledImageAccounts,
			"image_mode":                     s.cfg.ChatGPT.ImageMode,
		},
	})
}

func (s *Server) handlePortalAccountQuota(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handlePortalAdminUsers(w http.ResponseWriter, r *http.Request) {
	items, err := s.userStore.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.decoratePortalUsers(r.Context(), items),
		"quota": s.buildPortalQuotaSummary(),
	})
}

func (s *Server) handlePortalAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "user id is required"})
		return
	}

	var body struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	user, err := s.userStore.Update(userID, users.Update{
		Role:     body.Role,
		Disabled: body.Disabled,
	})
	if err != nil {
		writeJSON(w, statusForUserError(err), map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"item":  s.decoratePortalUser(r.Context(), user),
		"quota": s.buildPortalQuotaSummary(),
	})
}

func (s *Server) handlePortalGalleryWorks(w http.ResponseWriter, r *http.Request) {
	user, ok := portalUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	store, err := s.portalGalleryStore()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	items, err := store.ListWorks(r.Context(), portalstore.GalleryListOptions{
		ViewerUserID: user.ID,
		Search:       strings.TrimSpace(r.URL.Query().Get("query")),
		Sort:         strings.TrimSpace(r.URL.Query().Get("sort")),
	})
	if err != nil {
		writeJSON(w, statusForPortalGalleryError(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePortalGalleryWork(w http.ResponseWriter, r *http.Request) {
	user, ok := portalUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	store, err := s.portalGalleryStore()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	item, comments, err := store.GetWork(r.Context(), r.PathValue("id"), user.ID)
	if err != nil {
		writeJSON(w, statusForPortalGalleryError(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"item":     item,
		"comments": comments,
	})
}

func (s *Server) handlePortalPublishGalleryWork(w http.ResponseWriter, r *http.Request) {
	user, ok := portalUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	store, err := s.portalGalleryStore()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var body struct {
		Title        string `json:"title"`
		Prompt       string `json:"prompt"`
		ImageDataURL string `json:"image_data_url"`
		ImageURL     string `json:"image_url"`
		Model        string `json:"model"`
		Size         string `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	item, err := store.PublishWork(r.Context(), portalstore.PublishInput{
		UserID:       user.ID,
		UserEmail:    user.Email,
		Title:        body.Title,
		Prompt:       body.Prompt,
		ImageDataURL: body.ImageDataURL,
		ImageURL:     body.ImageURL,
		Model:        body.Model,
		Size:         body.Size,
	})
	if err != nil {
		writeJSON(w, statusForPortalGalleryError(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": item})
}

func (s *Server) handlePortalToggleGalleryLike(w http.ResponseWriter, r *http.Request) {
	user, ok := portalUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	store, err := s.portalGalleryStore()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	result, err := store.ToggleLike(r.Context(), r.PathValue("id"), user.ID)
	if err != nil {
		writeJSON(w, statusForPortalGalleryError(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"liked":      result.Liked,
		"like_count": result.LikeCount,
	})
}

func (s *Server) handlePortalCreateGalleryComment(w http.ResponseWriter, r *http.Request) {
	user, ok := portalUserFromContext(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	store, err := s.portalGalleryStore()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	item, commentCount, err := store.AddComment(r.Context(), r.PathValue("id"), user, body.Content)
	if err != nil {
		writeJSON(w, statusForPortalGalleryError(err), map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"item":          item,
		"comment_count": commentCount,
	})
}

func (s *Server) requirePortalUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.portalUserFromRequest(r)
		if !ok {
			s.clearPortalSessionCookie(w, r)
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
			return
		}
		next.ServeHTTP(w, withPortalUser(r, user))
	})
}

func (s *Server) requirePortalAdmin(next http.Handler) http.Handler {
	return s.requirePortalUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := portalUserFromContext(r)
		if !ok || user.Role != users.RoleAdmin {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin access required"})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func withPortalUser(r *http.Request, user users.PublicUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), portalUserContextKey, user))
}

func portalUserFromContext(r *http.Request) (users.PublicUser, bool) {
	if r == nil {
		return users.PublicUser{}, false
	}
	value := r.Context().Value(portalUserContextKey)
	user, ok := value.(users.PublicUser)
	return user, ok
}

func (s *Server) portalUserFromRequest(r *http.Request) (users.PublicUser, bool) {
	cookie, err := r.Cookie(portalSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return users.PublicUser{}, false
	}

	userID, expiresAt, ok := s.parsePortalSession(cookie.Value)
	if !ok || time.Now().After(expiresAt) {
		return users.PublicUser{}, false
	}

	user, err := s.userStore.Get(userID)
	if err != nil || user.Disabled {
		return users.PublicUser{}, false
	}
	return user, true
}

func (s *Server) setPortalSessionCookie(w http.ResponseWriter, r *http.Request, userID string) {
	expiresAt := time.Now().Add(portalSessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     portalSessionCookieName,
		Value:    s.signPortalSession(userID, expiresAt),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		Expires:  expiresAt,
		MaxAge:   int(portalSessionTTL / time.Second),
	})
}

func (s *Server) clearPortalSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     portalSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func (s *Server) signPortalSession(userID string, expiresAt time.Time) string {
	payload := fmt.Sprintf("%s|%d", strings.TrimSpace(userID), expiresAt.Unix())
	mac := hmac.New(sha256.New, []byte(s.portalSessionSecret()))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	token := strings.Join([]string{strings.TrimSpace(userID), fmt.Sprintf("%d", expiresAt.Unix()), signature}, ".")
	return base64.RawURLEncoding.EncodeToString([]byte(token))
}

func (s *Server) parsePortalSession(value string) (string, time.Time, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", time.Time{}, false
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 3 {
		return "", time.Time{}, false
	}

	userID := strings.TrimSpace(parts[0])
	if userID == "" {
		return "", time.Time{}, false
	}
	var expiresUnix int64
	if _, err := fmt.Sscanf(parts[1], "%d", &expiresUnix); err != nil {
		return "", time.Time{}, false
	}
	expiresAt := time.Unix(expiresUnix, 0)
	payload := fmt.Sprintf("%s|%d", userID, expiresUnix)
	mac := hmac.New(sha256.New, []byte(s.portalSessionSecret()))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return "", time.Time{}, false
	}
	return userID, expiresAt, true
}

func (s *Server) portalSessionSecret() string {
	secret := strings.TrimSpace(s.cfg.App.AuthKey)
	if secret == "" {
		return "chatgpt-image-studio-portal-session"
	}
	return "portal-session|" + secret + "|" + s.cfg.RootDir()
}

func (s *Server) buildPortalWorkspaceAccounts(items []accounts.PublicAccount) []portalWorkspaceAccount {
	result := make([]portalWorkspaceAccount, 0, len(items))
	for _, item := range items {
		result = append(result, portalWorkspaceAccount{
			ID:             item.ID,
			Type:           item.Type,
			Status:         item.Status,
			Quota:          item.Quota,
			LimitsProgress: item.LimitsProgress,
			RestoreAt:      item.RestoreAt,
			Disabled:       item.Disabled,
		})
	}
	return result
}

func (s *Server) decoratePortalUser(ctx context.Context, user users.PublicUser) portalUserResponse {
	usage := s.portalUsageMap(ctx)[user.ID]
	return portalUserResponse{
		ID:          user.ID,
		Email:       user.Email,
		Role:        user.Role,
		Disabled:    user.Disabled,
		CreatedAt:   user.CreatedAt,
		LastLoginAt: user.LastLoginAt,
		Usage: portalUserUsageResponse{
			ImageRequests:   usage.ImageRequests,
			GeneratedImages: usage.GeneratedImages,
			PublishedWorks:  usage.PublishedWorks,
		},
	}
}

func (s *Server) decoratePortalUsers(ctx context.Context, items []users.PublicUser) []portalUserResponse {
	usageMap := s.portalUsageMap(ctx)
	result := make([]portalUserResponse, 0, len(items))
	for _, item := range items {
		usage := usageMap[item.ID]
		result = append(result, portalUserResponse{
			ID:          item.ID,
			Email:       item.Email,
			Role:        item.Role,
			Disabled:    item.Disabled,
			CreatedAt:   item.CreatedAt,
			LastLoginAt: item.LastLoginAt,
			Usage: portalUserUsageResponse{
				ImageRequests:   usage.ImageRequests,
				GeneratedImages: usage.GeneratedImages,
				PublishedWorks:  usage.PublishedWorks,
			},
		})
	}
	return result
}

func (s *Server) portalUsageMap(ctx context.Context) map[string]portalstore.UserUsage {
	store, err := s.portalGalleryStore()
	if err != nil {
		slog.Warn("portal usage unavailable", slog.Any("error", err))
		return map[string]portalstore.UserUsage{}
	}
	items, err := store.ListUsage(ctx)
	if err != nil {
		slog.Warn("portal usage load failed", slog.Any("error", err))
		return map[string]portalstore.UserUsage{}
	}
	return items
}

func (s *Server) buildPortalQuotaSummary() portalQuotaSummary {
	items, err := s.store.ListAccounts()
	if err != nil {
		return portalQuotaSummary{}
	}

	summary := portalQuotaSummary{}
	for _, item := range items {
		summary.Accounts++
		remaining := getImageRemaining(item)
		summary.TotalQuota += remaining
		if isPortalImageAccountUsable(item) {
			summary.AvailableAccounts++
			summary.AvailableQuota += remaining
		}
		if isPaidImageAccountType(item.Type) {
			summary.PaidAccounts++
		}
	}
	return summary
}

func getImageRemaining(account accounts.PublicAccount) int {
	if remaining, _ := extractAccountQuota(account.LimitsProgress, "image_gen"); remaining != nil {
		if *remaining > 0 {
			return *remaining
		}
	}
	if account.Quota > 0 {
		return account.Quota
	}
	return 0
}

func isPortalImageAccountUsable(account accounts.PublicAccount) bool {
	return !account.Disabled &&
		account.Status != "禁用" &&
		account.Status != "异常" &&
		account.Status != "限流" &&
		getImageRemaining(account) > 0
}

func statusForPortalGalleryError(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, portalstore.ErrNotFound):
		return http.StatusNotFound
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, token := range []string{"required", "invalid", "too long", "empty", "unavailable"} {
		if strings.Contains(message, token) {
			return http.StatusBadRequest
		}
	}
	return http.StatusInternalServerError
}

func statusForUserError(err error) int {
	switch err {
	case nil:
		return http.StatusOK
	case users.ErrInvalidCredentials, users.ErrUserDisabled:
		return http.StatusUnauthorized
	case users.ErrEmailExists, users.ErrLastAdminRequired:
		return http.StatusBadRequest
	case users.ErrNotFound:
		return http.StatusNotFound
	default:
		return http.StatusBadRequest
	}
}

func statusForVerificationError(err error) int {
	switch err {
	case nil:
		return http.StatusOK
	case verification.ErrCodeTooFrequent:
		return http.StatusTooManyRequests
	case verification.ErrCodeNotFound:
		return http.StatusGone
	case verification.ErrCodeMismatch:
		return http.StatusBadRequest
	default:
		return http.StatusBadRequest
	}
}

func requestIsSecure(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
