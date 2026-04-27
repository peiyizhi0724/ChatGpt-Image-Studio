package accounts

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"chatgpt2api/handler"
	"chatgpt2api/internal/cliproxy"
	"chatgpt2api/internal/config"
)

const (
	proFallbackImageGenQuota  = 999
	AccountSourceKindAuthFile = "auth_file"
	AccountSourceKindToken    = "token"
)

type LocalAuth struct {
	Name        string
	Path        string
	Provider    string
	SourceKind  string
	AccessToken string
	Email       string
	UserID      string
	Disabled    bool
	Note        string
	Priority    int
	ImportedAt  time.Time
	Data        map[string]any
}

type RuntimeState struct {
	Type                       string           `json:"type,omitempty"`
	Status                     string           `json:"status,omitempty"`
	Quota                      int              `json:"quota"`
	QuotaKnown                 bool             `json:"quota_known"`
	Email                      string           `json:"email,omitempty"`
	UserID                     string           `json:"user_id,omitempty"`
	LimitsProgress             []map[string]any `json:"limits_progress,omitempty"`
	DefaultModelSlug           string           `json:"default_model_slug,omitempty"`
	RestoreAt                  string           `json:"restore_at,omitempty"`
	Success                    int              `json:"success"`
	Fail                       int              `json:"fail"`
	LastUsedAt                 string           `json:"last_used_at,omitempty"`
	LastRefreshedAt            string           `json:"last_refreshed_at,omitempty"`
	ImageQuotaDailyBase        int              `json:"image_quota_daily_base,omitempty"`
	ImageQuotaDailyBaseResetAt string           `json:"image_quota_daily_base_reset_at,omitempty"`
}

type SyncState struct {
	Name         string `json:"name"`
	Origin       string `json:"origin"`
	LastSyncedAt string `json:"last_synced_at,omitempty"`
}

type SyncAccountStatus struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Location       string `json:"location"`
	LocalDisabled  *bool  `json:"localDisabled,omitempty"`
	RemoteDisabled *bool  `json:"remoteDisabled,omitempty"`
}

type SyncSummary struct {
	Configured       bool                `json:"configured"`
	Local            int                 `json:"local"`
	Remote           int                 `json:"remote"`
	Summary          map[string]int      `json:"summary"`
	Accounts         []SyncAccountStatus `json:"accounts"`
	LastRun          *SyncRunResult      `json:"lastRun,omitempty"`
	DisabledMismatch int                 `json:"disabledMismatch"`
}

type SyncRunResult struct {
	OK                  bool   `json:"ok"`
	Running             bool   `json:"running"`
	Error               string `json:"error,omitempty"`
	Direction           string `json:"direction,omitempty"`
	Uploaded            int    `json:"uploaded"`
	UploadFailed        int    `json:"upload_failed"`
	Downloaded          int    `json:"downloaded"`
	DownloadFailed      int    `json:"download_failed"`
	RemoteDeleted       int    `json:"remote_deleted"`
	DisabledAligned     int    `json:"disabled_aligned"`
	DisabledAlignFailed int    `json:"disabled_align_failed"`
	Total               int    `json:"total"`
	Processed           int    `json:"processed"`
	Phase               string `json:"phase,omitempty"`
	Current             string `json:"current,omitempty"`
	StartedAt           string `json:"started_at"`
	FinishedAt          string `json:"finished_at"`
	UpdatedAt           string `json:"updated_at,omitempty"`
}

type PublicAccount struct {
	ID               string           `json:"id"`
	FileName         string           `json:"fileName"`
	AccessToken      string           `json:"access_token"`
	SourceKind       string           `json:"sourceKind"`
	Type             string           `json:"type"`
	Status           string           `json:"status"`
	Quota            int              `json:"quota"`
	Email            string           `json:"email,omitempty"`
	UserID           string           `json:"user_id,omitempty"`
	LimitsProgress   []map[string]any `json:"limits_progress"`
	DefaultModelSlug string           `json:"default_model_slug,omitempty"`
	RestoreAt        string           `json:"restoreAt,omitempty"`
	Success          int              `json:"success"`
	Fail             int              `json:"fail"`
	LastUsedAt       string           `json:"lastUsedAt,omitempty"`
	LastRefreshedAt  string           `json:"lastRefreshedAt,omitempty"`
	Provider         string           `json:"provider"`
	Disabled         bool             `json:"disabled"`
	Note             string           `json:"note,omitempty"`
	Priority         int              `json:"priority"`
	SyncStatus       string           `json:"syncStatus,omitempty"`
	SyncOrigin       string           `json:"syncOrigin,omitempty"`
	LastSyncedAt     string           `json:"lastSyncedAt,omitempty"`
	RemoteDisabled   *bool            `json:"remoteDisabled,omitempty"`
	ImportedAt       string           `json:"importedAt,omitempty"`
}

type AccountUpdate struct {
	Type   *string
	Status *string
	Quota  *int
	Note   *string
}

type RefreshError struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
}

type RefreshProgress struct {
	Total       int    `json:"total"`
	Processed   int    `json:"processed"`
	Refreshed   int    `json:"refreshed"`
	Failed      int    `json:"failed"`
	Current     string `json:"current"`
	AccessToken string `json:"access_token"`
	Error       string `json:"error,omitempty"`
}

type RefreshOptions struct {
	MaxWorkers int
	Progress   func(RefreshProgress)
}

type ImportedAuthFile struct {
	Name string
	Data []byte
}

type ImportSkip struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type ImportFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type importedAuthCandidate struct {
	OriginalName string
	AuthData     map[string]any
	AccessToken  string
	IdentityKey  string
	FreshAt      time.Time
}

type Store struct {
	cfg            *config.Config
	authDir        string
	stateFile      string
	syncStateDir   string
	backend        accountStorageBackend
	defaultQuota   int
	refreshWorkers int
	providerType   string
	mu             sync.Mutex
	states         map[string]RuntimeState
	imageLeases    map[string]int
	lastSyncRun    *SyncRunResult
}

type Snapshot struct {
	AuthFiles  []ImportedAuthFile
	States     map[string]RuntimeState
	SyncStates map[string]SyncState
}

var ErrSourceAccountNotFound = errors.New("source account not found")
var ErrNoAvailableImageAuth = errors.New("no available image auth")
var ErrImageAuthInUse = errors.New("image auth is in use")
var ErrSelectedImageGroupsExhausted = errors.New("selected image groups exhausted")

type stateEnvelope struct {
	Accounts map[string]RuntimeState `json:"accounts"`
}

func NewStore(cfg *config.Config) (*Store, error) {
	store := &Store{
		cfg:            cfg,
		authDir:        cfg.ResolvePath(cfg.Storage.AuthDir),
		stateFile:      cfg.ResolvePath(cfg.Storage.StateFile),
		syncStateDir:   cfg.ResolvePath(cfg.Storage.SyncStateDir),
		defaultQuota:   max(1, cfg.Accounts.DefaultQuota),
		refreshWorkers: max(1, cfg.Accounts.RefreshWorkers),
		providerType:   strings.TrimSpace(cfg.Sync.ProviderType),
		states:         map[string]RuntimeState{},
		imageLeases:    map[string]int{},
	}

	backend, err := newAccountStorageBackend(cfg, store.authDir, store.stateFile, store.syncStateDir, store.providerType)
	if err != nil {
		return nil, err
	}
	store.backend = backend
	if err := store.storage().Init(); err != nil {
		return nil, err
	}
	if err := migrateIntoEmptyBackendIfNeeded(
		store.storage(),
		cfg.Storage.Backend,
		store.authDir,
		store.stateFile,
		store.syncStateDir,
		cfg.ResolvePath(cfg.Storage.SQLitePath),
		cfg.Storage.RedisAddr,
		cfg.Storage.RedisPassword,
		cfg.Storage.RedisPrefix,
		cfg.Storage.RedisDB,
	); err != nil {
		return nil, err
	}
	if err := store.loadState(); err != nil {
		return nil, err
	}
	if err := store.ensureSyncStateInitialized(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) defaultQuotaValue() int {
	if s != nil && s.cfg != nil {
		return max(1, s.cfg.Accounts.DefaultQuota)
	}
	return max(1, s.defaultQuota)
}

func (s *Store) refreshWorkersValue() int {
	if s != nil && s.cfg != nil {
		return max(1, s.cfg.Accounts.RefreshWorkers)
	}
	return max(1, s.refreshWorkers)
}

func (s *Store) autoRefreshConfig() (bool, time.Duration) {
	if s == nil || s.cfg == nil {
		return false, 30 * time.Minute
	}
	intervalMinutes := max(1, s.cfg.Accounts.AutoRefreshInterval)
	return s.cfg.Accounts.AutoRefreshEnabled, time.Duration(intervalMinutes) * time.Minute
}

func (s *Store) RunAutoRefreshLoop(ctx context.Context) {
	if s == nil {
		return
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	var lastRun time.Time
	for {
		enabled, interval := s.autoRefreshConfig()
		if !enabled {
			lastRun = time.Time{}
		} else if lastRun.IsZero() || time.Since(lastRun) >= interval {
			requestTimeoutSeconds := 120
			if s.cfg != nil {
				requestTimeoutSeconds = max(30, s.cfg.ChatGPT.RequestTimeout)
			}

			runCtx, cancel := context.WithTimeout(ctx, time.Duration(requestTimeoutSeconds)*time.Second)
			refreshed, refreshErrors, err := s.RefreshAccounts(runCtx, nil)
			cancel()
			lastRun = time.Now()

			switch {
			case err != nil:
				slog.Warn("auto refresh accounts failed", "error", err)
			case len(refreshErrors) > 0:
				slog.Warn(
					"auto refresh accounts completed with errors",
					"refreshed",
					refreshed,
					"errors",
					len(refreshErrors),
					"first_error",
					refreshErrors[0].Error,
				)
			default:
				slog.Info("auto refresh accounts completed", "refreshed", refreshed)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) storage() accountStorageBackend {
	if s.backend != nil {
		return s.backend
	}
	s.backend = newFileAccountStorage(s.authDir, s.stateFile, s.syncStateDir)
	return s.backend
}

func (s *Store) Close() error {
	if s.backend == nil {
		return nil
	}
	return s.backend.Close()
}

func (s *Store) GetImageRoutingPolicy() (ImageAccountRoutingPolicy, error) {
	policy, err := s.storage().LoadImageRoutingPolicy()
	if err != nil {
		return ImageAccountRoutingPolicy{}, err
	}
	if policy == nil {
		return defaultImageAccountRoutingPolicy(), nil
	}
	normalized := policy.Normalize()
	if normalized.GroupSize <= 0 {
		defaults := defaultImageAccountRoutingPolicy()
		normalized.GroupSize = defaults.GroupSize
	}
	if normalized.ReserveMode == "" {
		normalized.ReserveMode = defaultImageAccountRoutingPolicy().ReserveMode
	}
	return normalized, nil
}

func (s *Store) SaveImageRoutingPolicy(policy ImageAccountRoutingPolicy) error {
	return s.storage().SaveImageRoutingPolicy(policy.Normalize())
}

func (s *Store) ListAccounts() ([]PublicAccount, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, err
	}
	syncStates := s.loadAllSyncStates()

	s.mu.Lock()
	defer s.mu.Unlock()

	accounts := make([]PublicAccount, 0, len(localAuths))
	for _, auth := range localAuths {
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		if account.AccessToken == "" {
			continue
		}
		accounts = append(accounts, account)
	}

	sort.Slice(accounts, func(i, j int) bool {
		if accountRank(accounts[i]) != accountRank(accounts[j]) {
			return accountRank(accounts[i]) < accountRank(accounts[j])
		}
		if accounts[i].Priority != accounts[j].Priority {
			return accounts[i].Priority > accounts[j].Priority
		}
		if accounts[i].Quota != accounts[j].Quota {
			return accounts[i].Quota > accounts[j].Quota
		}
		left := strings.ToLower(firstNonEmpty(accounts[i].Email, accounts[i].FileName))
		right := strings.ToLower(firstNonEmpty(accounts[j].Email, accounts[j].FileName))
		return left < right
	})

	return accounts, nil
}

func (s *Store) ListLocalAuths() ([]LocalAuth, error) {
	return s.loadAuths()
}

func (s *Store) Snapshot() (*Snapshot, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, err
	}
	files := make([]ImportedAuthFile, 0, len(localAuths))
	for _, auth := range localAuths {
		raw, err := s.readAuthRaw(auth.Name)
		if err != nil {
			return nil, err
		}
		files = append(files, ImportedAuthFile{Name: auth.Name, Data: raw})
	}

	s.mu.Lock()
	states := cloneRuntimeStatesMap(s.states)
	lastRun := s.lastSyncRun
	s.mu.Unlock()
	_ = lastRun

	syncStates := s.loadAllSyncStates()
	clonedSyncStates := make(map[string]SyncState, len(syncStates))
	for key, value := range syncStates {
		clonedSyncStates[key] = value
	}

	return &Snapshot{
		AuthFiles:  files,
		States:     states,
		SyncStates: clonedSyncStates,
	}, nil
}

func (s *Store) ReplaceAllData(snapshot *Snapshot) error {
	if snapshot == nil {
		return nil
	}

	existingAuths, err := s.loadAuths()
	if err != nil {
		return err
	}
	expectedAuths := make(map[string]struct{}, len(snapshot.AuthFiles))
	for _, file := range snapshot.AuthFiles {
		expectedAuths[file.Name] = struct{}{}
		if err := s.saveAuthBytes(file.Name, file.Data); err != nil {
			return err
		}
	}
	for _, auth := range existingAuths {
		if _, ok := expectedAuths[auth.Name]; ok {
			continue
		}
		if err := s.deleteAuth(auth.Name); err != nil {
			return err
		}
	}

	s.mu.Lock()
	s.states = cloneRuntimeStatesMap(snapshot.States)
	if err := s.saveStateLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()

	existingSyncStates := s.loadAllSyncStates()
	expectedSyncStates := make(map[string]struct{}, len(snapshot.SyncStates))
	for name, state := range snapshot.SyncStates {
		expectedSyncStates[name] = struct{}{}
		if err := s.storage().SaveSyncState(state); err != nil {
			return err
		}
	}
	for name := range existingSyncStates {
		if _, ok := expectedSyncStates[name]; ok {
			continue
		}
		if err := s.storage().DeleteSyncState(name); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) AddAccounts(tokens []string) (int, int, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, 0, err
	}

	existing := make(map[string]LocalAuth, len(localAuths))
	existingNames := make(map[string]struct{}, len(localAuths))
	for _, auth := range localAuths {
		existing[auth.AccessToken] = auth
		existingNames[auth.Name] = struct{}{}
	}

	added := 0
	skipped := 0
	for _, token := range dedupeTokens(tokens) {
		if _, ok := existing[token]; ok {
			skipped++
			continue
		}

		authData := s.newAuthFileData(token)
		name := s.newAuthFileName(authData, existingNames)
		if err := s.saveAuthData(name, authData); err != nil {
			return added, skipped, err
		}
		s.markLocalNew(name)
		s.upsertState(name, func(state RuntimeState) RuntimeState {
			state.Type = normalizePlanType(firstNonEmpty(stringValue(authData["chatgpt_plan_type"]), guessPlanFromPayload(authData)))
			state.Email = firstNonEmpty(stringValue(authData["email"]), guessEmail(authData))
			state.UserID = firstNonEmpty(stringValue(authData["user_id"]), guessUserID(authData))
			if !state.QuotaKnown {
				state.Quota = s.defaultQuota
				state.Status = "正常"
			}
			return state
		})
		existingNames[name] = struct{}{}
		added++
	}

	return added, skipped, nil
}

func (s *Store) ImportAuthFiles(files []ImportedAuthFile) (int, []string, []ImportSkip, []ImportFailure, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, nil, nil, nil, err
	}

	existingByToken := make(map[string]LocalAuth, len(localAuths))
	existingByIdentity := make(map[string]LocalAuth, len(localAuths))
	existingNames := make(map[string]struct{}, len(localAuths))
	for _, auth := range localAuths {
		if auth.AccessToken != "" {
			existingByToken[auth.AccessToken] = auth
		}
		if key := authIdentityKey(auth.Data, auth.Provider); key != "" {
			if current, ok := existingByIdentity[key]; !ok || authFreshnessTime(auth.Data).After(authFreshnessTime(current.Data)) {
				existingByIdentity[key] = auth
			}
		}
		existingNames[auth.Name] = struct{}{}
	}

	importedTokens := make([]string, 0, len(files))
	skipped := make([]ImportSkip, 0)
	failures := make([]ImportFailure, 0)
	imported := 0
	batchCandidates := make(map[string]importedAuthCandidate, len(files))

	for _, file := range files {
		name := filepath.Base(strings.TrimSpace(file.Name))
		if name == "" {
			name = fmt.Sprintf("import-%d.json", time.Now().UnixNano())
		}
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			failures = append(failures, ImportFailure{Name: name, Error: "file must be .json"})
			continue
		}

		var authData map[string]any
		if err := json.Unmarshal(file.Data, &authData); err != nil {
			failures = append(failures, ImportFailure{Name: name, Error: "invalid auth json"})
			continue
		}
		if authData == nil {
			authData = map[string]any{}
		}

		accessToken := strings.TrimSpace(stringValue(authData["access_token"]))
		if accessToken == "" {
			failures = append(failures, ImportFailure{Name: name, Error: "access_token is required"})
			continue
		}
		if existing, ok := existingByToken[accessToken]; ok {
			skipped = append(skipped, ImportSkip{Name: name, Reason: fmt.Sprintf("duplicate token with %s", existing.Name)})
			continue
		}

		if strings.TrimSpace(stringValue(authData["type"])) == "" {
			authData["type"] = firstNonEmpty(s.providerType, "codex")
		}
		authData["access_token"] = accessToken
		if email := firstNonEmpty(stringValue(authData["email"]), guessEmail(authData)); email != "" {
			authData["email"] = email
		}
		if userID := firstNonEmpty(stringValue(authData["user_id"]), guessUserID(authData)); userID != "" {
			authData["user_id"] = userID
		}
		if plan := guessPlanFromPayload(authData); plan != "" && strings.TrimSpace(stringValue(authData["chatgpt_plan_type"])) == "" {
			authData["chatgpt_plan_type"] = strings.ToLower(plan)
		}
		if strings.TrimSpace(stringValue(authData["created_at"])) == "" {
			authData["created_at"] = time.Now().UTC().Format(time.RFC3339)
		}
		authData["source_kind"] = resolveAccountSourceKind(authData)

		identityKey := authIdentityKey(authData, s.providerType)
		candidate := importedAuthCandidate{
			OriginalName: name,
			AuthData:     authData,
			AccessToken:  accessToken,
			IdentityKey:  identityKey,
			FreshAt:      authFreshnessTime(authData),
		}
		if identityKey == "" {
			identityKey = "token::" + shortID(accessToken)
			candidate.IdentityKey = identityKey
		}

		if current, ok := batchCandidates[identityKey]; ok {
			if candidate.FreshAt.After(current.FreshAt) {
				skipped = append(skipped, ImportSkip{Name: current.OriginalName, Reason: fmt.Sprintf("older than %s in current import", candidate.OriginalName)})
				batchCandidates[identityKey] = candidate
			} else {
				skipped = append(skipped, ImportSkip{Name: candidate.OriginalName, Reason: fmt.Sprintf("older than %s in current import", current.OriginalName)})
			}
			continue
		}
		batchCandidates[identityKey] = candidate
	}

	keys := make([]string, 0, len(batchCandidates))
	for key := range batchCandidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		candidate := batchCandidates[key]
		targetName := candidate.OriginalName
		targetData := candidate.AuthData
		if existing, ok := existingByIdentity[candidate.IdentityKey]; ok {
			existingFreshAt := authFreshnessTime(existing.Data)
			if !candidate.FreshAt.After(existingFreshAt) {
				skipped = append(skipped, ImportSkip{Name: candidate.OriginalName, Reason: fmt.Sprintf("older than existing %s", existing.Name)})
				continue
			}
			targetName = existing.Name
		} else if _, exists := existingNames[targetName]; exists {
			targetName = s.newAuthFileName(targetData, existingNames)
		}

		if err := s.saveAuthData(targetName, targetData); err != nil {
			failures = append(failures, ImportFailure{Name: candidate.OriginalName, Error: err.Error()})
			continue
		}

		s.markLocalNew(targetName)
		s.upsertState(targetName, func(state RuntimeState) RuntimeState {
			state.Type = firstNonEmpty(normalizePlanType(guessPlanFromPayload(targetData)), state.Type, "Free")
			state.Email = firstNonEmpty(stringValue(targetData["email"]), guessEmail(targetData), state.Email)
			state.UserID = firstNonEmpty(stringValue(targetData["user_id"]), guessUserID(targetData), state.UserID)
			if !state.QuotaKnown {
				state.Quota = s.defaultQuota
				if boolValue(targetData["disabled"]) {
					state.Status = "禁用"
				} else {
					state.Status = "正常"
				}
			}
			return state
		})

		existingNames[targetName] = struct{}{}
		existingByToken[candidate.AccessToken] = LocalAuth{Name: targetName, AccessToken: candidate.AccessToken}
		existingByIdentity[candidate.IdentityKey] = LocalAuth{
			Name:        targetName,
			AccessToken: candidate.AccessToken,
			Provider:    firstNonEmpty(stringValue(targetData["type"]), s.providerType),
			Data:        targetData,
		}
		importedTokens = append(importedTokens, candidate.AccessToken)
		imported++
	}

	return imported, importedTokens, skipped, failures, nil
}

func (s *Store) DeleteAccounts(accessTokens []string) (int, error) {
	targets := dedupeTokens(accessTokens)
	if len(targets) == 0 {
		return 0, nil
	}

	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, err
	}

	targetSet := make(map[string]struct{}, len(targets))
	for _, token := range targets {
		targetSet[token] = struct{}{}
	}

	removed := 0
	for _, auth := range localAuths {
		if _, ok := targetSet[auth.AccessToken]; !ok {
			continue
		}
		if err := s.deleteAuth(auth.Name); err != nil {
			return removed, err
		}
		s.deleteSyncState(auth.Name)
		s.removeState(auth.Name)
		removed++
	}
	return removed, nil
}

func (s *Store) RefreshAccounts(ctx context.Context, accessTokens []string) (int, []RefreshError, error) {
	return s.RefreshAccountsWithOptions(ctx, accessTokens, RefreshOptions{})
}

func (s *Store) RefreshAccountsWithOptions(ctx context.Context, accessTokens []string, options RefreshOptions) (int, []RefreshError, error) {
	targets := dedupeTokens(accessTokens)
	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, nil, err
	}

	authByToken := make(map[string]LocalAuth, len(localAuths))
	for _, auth := range localAuths {
		if auth.AccessToken != "" {
			authByToken[auth.AccessToken] = auth
		}
	}

	if len(targets) == 0 {
		targets = make([]string, 0, len(authByToken))
		for token := range authByToken {
			targets = append(targets, token)
		}
		sort.Strings(targets)
	}

	type refreshResult struct {
		token string
		auth  LocalAuth
		info  *handler.RemoteAccountInfo
		err   error
	}

	jobs := make(chan string)
	results := make(chan refreshResult, len(targets))
	workers := minInt(s.refreshWorkersValue(), max(1, len(targets)))
	if options.MaxWorkers > 0 {
		workers = minInt(workers, max(1, options.MaxWorkers))
	}
	for i := 0; i < workers; i++ {
		go func() {
			for token := range jobs {
				auth, ok := authByToken[token]
				if !ok {
					results <- refreshResult{token: token, err: fmt.Errorf("account not found")}
					continue
				}
				timeout := time.Duration(max(10, s.cfg.ChatGPT.RequestTimeout)) * time.Second
				info, err := handler.FetchAccountInfoWithProxy(ctx, token, auth.Data, timeout, s.cfg.ChatGPTProxyURL())
				results <- refreshResult{token: token, auth: auth, info: info, err: err}
			}
		}()
	}

	for _, token := range targets {
		jobs <- token
	}
	close(jobs)

	refreshed := 0
	errors := make([]RefreshError, 0)
	processed := 0
	for range targets {
		result := <-results
		auth := authByToken[result.token]
		if result.auth.Name != "" {
			auth = result.auth
		}
		if result.err != nil {
			message := result.err.Error()
			if strings.Contains(message, "/backend-api/me failed: HTTP 401") {
				s.upsertState(auth.Name, func(state RuntimeState) RuntimeState {
					state.Status = "异常"
					state.Quota = 0
					state.QuotaKnown = true
					state.LastRefreshedAt = time.Now().UTC().Format(time.RFC3339)
					return state
				})
				message = "检测到封号"
			}
			errors = append(errors, RefreshError{AccessToken: result.token, Error: message})
			processed++
			if options.Progress != nil {
				options.Progress(RefreshProgress{
					Total:       len(targets),
					Processed:   processed,
					Refreshed:   refreshed,
					Failed:      len(errors),
					Current:     refreshProgressLabel(auth, result.token),
					AccessToken: result.token,
					Error:       message,
				})
			}
			continue
		}

		s.upsertState(auth.Name, func(state RuntimeState) RuntimeState {
			state.Type = firstNonEmpty(result.info.AccountType, state.Type, normalizePlanType(guessPlanFromPayload(auth.Data)), "Free")
			state.Status = firstNonEmpty(result.info.Status, state.Status)
			state.Quota = result.info.Quota
			state.QuotaKnown = true
			state.Email = firstNonEmpty(result.info.Email, state.Email, auth.Email)
			state.UserID = firstNonEmpty(result.info.UserID, state.UserID, auth.UserID)
			state.LimitsProgress = cloneSlice(result.info.LimitsProgress)
			state.DefaultModelSlug = firstNonEmpty(result.info.DefaultModelSlug, state.DefaultModelSlug)
			state.RestoreAt = firstNonEmpty(result.info.RestoreAt, state.RestoreAt)
			state.LastRefreshedAt = time.Now().UTC().Format(time.RFC3339)
			return syncImageQuotaDailyBaseState(state, result.info.Quota, result.info.LimitsProgress, result.info.RestoreAt)
		})
		refreshed++
		processed++
		if options.Progress != nil {
			options.Progress(RefreshProgress{
				Total:       len(targets),
				Processed:   processed,
				Refreshed:   refreshed,
				Failed:      len(errors),
				Current:     refreshProgressLabel(auth, result.token),
				AccessToken: result.token,
			})
		}
	}

	return refreshed, errors, nil
}

func (s *Store) UpdateAccount(accessToken string, update AccountUpdate) (*PublicAccount, error) {
	auth, err := s.findAuthByToken(accessToken)
	if err != nil {
		return nil, err
	}

	state := s.getState(auth.Name)
	if update.Type != nil {
		state.Type = normalizePlanType(*update.Type)
	}
	if update.Status != nil {
		state.Status = strings.TrimSpace(*update.Status)
		switch state.Status {
		case "禁用":
			auth.Disabled = true
		case "正常", "限流", "异常":
			auth.Disabled = false
		}
	}
	if update.Quota != nil {
		state.Quota = max(0, *update.Quota)
		state.QuotaKnown = true
	}
	if update.Note != nil {
		auth.Note = strings.TrimSpace(*update.Note)
	}

	auth.Data["disabled"] = auth.Disabled
	if auth.Note != "" {
		auth.Data["note"] = auth.Note
	} else {
		delete(auth.Data, "note")
	}
	if err := s.saveAuthData(auth.Name, auth.Data); err != nil {
		return nil, err
	}
	s.setState(auth.Name, state)

	syncStates := s.loadAllSyncStates()
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
	return &account, nil
}

func (s *Store) GetAccountByToken(accessToken string) (*PublicAccount, error) {
	auth, err := s.findAuthByToken(accessToken)
	if err != nil {
		return nil, err
	}

	syncStates := s.loadAllSyncStates()
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
	return &account, nil
}

func (s *Store) FindImageAuthByID(accountID string) (*LocalAuth, PublicAccount, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, PublicAccount{}, err
	}
	syncStates := s.loadAllSyncStates()

	s.mu.Lock()
	defer s.mu.Unlock()

	target := strings.TrimSpace(accountID)
	for _, auth := range localAuths {
		if auth.AccessToken == "" || !s.matchesProvider(auth.Provider) {
			continue
		}
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		if account.ID == target {
			return &auth, account, nil
		}
	}

	return nil, PublicAccount{}, ErrSourceAccountNotFound
}

func (s *Store) FindImageAuthByIDWithLease(accountID string) (*LocalAuth, PublicAccount, func(), error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, PublicAccount{}, nil, err
	}
	syncStates := s.loadAllSyncStates()

	s.mu.Lock()
	defer s.mu.Unlock()

	target := strings.TrimSpace(accountID)
	for _, auth := range localAuths {
		if auth.AccessToken == "" || !s.matchesProvider(auth.Provider) {
			continue
		}
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		if account.ID != target {
			continue
		}
		release, leaseErr := s.acquireImageLeaseLocked(auth.AccessToken)
		if leaseErr != nil {
			return nil, PublicAccount{}, nil, leaseErr
		}
		return &auth, account, release, nil
	}

	return nil, PublicAccount{}, nil, ErrSourceAccountNotFound
}

func (s *Store) AcquireImageAuth(excluded map[string]struct{}) (*LocalAuth, PublicAccount, error) {
	return s.acquireImageAuth(excluded, nil, false)
}

func (s *Store) AcquireImageAuthFiltered(excluded map[string]struct{}, allow func(PublicAccount) bool) (*LocalAuth, PublicAccount, error) {
	return s.acquireImageAuth(excluded, allow, false)
}

func (s *Store) AcquireImageAuthFilteredWithDisabledOption(excluded map[string]struct{}, allow func(PublicAccount) bool, allowDisabled bool) (*LocalAuth, PublicAccount, error) {
	return s.acquireImageAuth(excluded, allow, allowDisabled)
}

func (s *Store) AcquireImageAuthLeaseFilteredWithDisabledOption(excluded map[string]struct{}, allow func(PublicAccount) bool, allowDisabled bool) (*LocalAuth, PublicAccount, func(), error) {
	return s.acquireImageAuthWithLease(excluded, allow, allowDisabled)
}

func (s *Store) acquireImageAuth(excluded map[string]struct{}, allow func(PublicAccount) bool, allowDisabled bool) (*LocalAuth, PublicAccount, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, PublicAccount{}, err
	}
	syncStates := s.loadAllSyncStates()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	accounts := make([]struct {
		auth    LocalAuth
		account PublicAccount
		ready   bool
	}, 0, len(localAuths))
	for _, auth := range localAuths {
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		if _, blocked := excluded[auth.AccessToken]; blocked {
			continue
		}
		if allow != nil && !allow(account) {
			continue
		}
		ready := isUsableImageAccount(account, allowDisabled)
		refreshNeeded := NeedsImageQuotaRefresh(account, now)
		if auth.AccessToken == "" ||
			(auth.Disabled && !allowDisabled) ||
			(account.Status == "禁用" && !allowDisabled) ||
			account.Status == "异常" ||
			(!ready && !refreshNeeded) {
			continue
		}
		accounts = append(accounts, struct {
			auth    LocalAuth
			account PublicAccount
			ready   bool
		}{auth: auth, account: account, ready: ready})
	}

	if len(accounts) == 0 {
		return nil, PublicAccount{}, fmt.Errorf("%w in %s", ErrNoAvailableImageAuth, s.authDir)
	}

	sort.Slice(accounts, func(i, j int) bool {
		leftReady := accounts[i].ready
		rightReady := accounts[j].ready
		if leftReady != rightReady {
			return leftReady
		}
		if accounts[i].account.Priority != accounts[j].account.Priority {
			return accounts[i].account.Priority > accounts[j].account.Priority
		}
		if accounts[i].account.Fail != accounts[j].account.Fail {
			return accounts[i].account.Fail < accounts[j].account.Fail
		}
		return accounts[i].account.LastUsedAt < accounts[j].account.LastUsedAt
	})

	selected := accounts[0]
	return &selected.auth, selected.account, nil
}

func (s *Store) acquireImageAuthWithLease(excluded map[string]struct{}, allow func(PublicAccount) bool, allowDisabled bool) (*LocalAuth, PublicAccount, func(), error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, PublicAccount{}, nil, err
	}
	syncStates := s.loadAllSyncStates()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	candidates := make([]struct {
		auth    LocalAuth
		account PublicAccount
		ready   bool
	}, 0, len(localAuths))
	for _, auth := range localAuths {
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		if _, blocked := excluded[auth.AccessToken]; blocked {
			continue
		}
		if s.isImageLeasedLocked(auth.AccessToken) {
			continue
		}
		if allow != nil && !allow(account) {
			continue
		}
		ready := isUsableImageAccount(account, allowDisabled)
		refreshNeeded := NeedsImageQuotaRefresh(account, now)
		if auth.AccessToken == "" ||
			(auth.Disabled && !allowDisabled) ||
			(account.Status == "禁用" && !allowDisabled) ||
			account.Status == "异常" ||
			(!ready && !refreshNeeded) {
			continue
		}
		candidates = append(candidates, struct {
			auth    LocalAuth
			account PublicAccount
			ready   bool
		}{auth: auth, account: account, ready: ready})
	}

	if len(candidates) == 0 {
		return nil, PublicAccount{}, nil, fmt.Errorf("%w in %s", ErrNoAvailableImageAuth, s.authDir)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ready != candidates[j].ready {
			return candidates[i].ready
		}
		if candidates[i].account.Priority != candidates[j].account.Priority {
			return candidates[i].account.Priority > candidates[j].account.Priority
		}
		if candidates[i].account.Fail != candidates[j].account.Fail {
			return candidates[i].account.Fail < candidates[j].account.Fail
		}
		return candidates[i].account.LastUsedAt < candidates[j].account.LastUsedAt
	})

	selected := candidates[0]
	release, leaseErr := s.acquireImageLeaseLocked(selected.auth.AccessToken)
	if leaseErr != nil {
		return nil, PublicAccount{}, nil, leaseErr
	}
	return &selected.auth, selected.account, release, nil
}

func (s *Store) acquireImageLeaseLocked(accessToken string) (func(), error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return nil, fmt.Errorf("access token is required")
	}
	if s.imageLeases == nil {
		s.imageLeases = map[string]int{}
	}
	if s.imageLeases[token] > 0 {
		return nil, ErrImageAuthInUse
	}
	s.imageLeases[token]++

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			count := s.imageLeases[token]
			if count <= 1 {
				delete(s.imageLeases, token)
				return
			}
			s.imageLeases[token] = count - 1
		})
	}, nil
}

func (s *Store) isImageLeasedLocked(accessToken string) bool {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return false
	}
	return s.imageLeases[token] > 0
}

func (s *Store) RecordImageResult(accessToken string, success bool) {
	auth, err := s.findAuthByToken(accessToken)
	if err != nil {
		return
	}

	s.upsertState(auth.Name, func(state RuntimeState) RuntimeState {
		state.LastUsedAt = time.Now().Format("2006-01-02 15:04:05")
		if success {
			state.Success++
			if state.QuotaKnown && state.Quota > 0 {
				state.Quota--
				if state.Quota == 0 && state.Status != "禁用" && state.Status != "异常" {
					state.Status = "限流"
				}
			}
			state.LimitsProgress = decrementLimitRemaining(state.LimitsProgress, "image_gen")
			if state.Status == "" {
				state.Status = "正常"
			}
		} else {
			state.Fail++
		}
		return state
	})
}

func (s *Store) MarkImageTokenAbnormal(accessToken string) {
	auth, err := s.findAuthByToken(accessToken)
	if err != nil {
		return
	}
	s.upsertState(auth.Name, func(state RuntimeState) RuntimeState {
		state.Status = "异常"
		state.Quota = 0
		state.QuotaKnown = true
		return state
	})
}

func (s *Store) MarkImageAccountLimited(accessToken string) {
	auth, err := s.findAuthByToken(accessToken)
	if err != nil {
		return
	}
	s.upsertState(auth.Name, func(state RuntimeState) RuntimeState {
		state.Status = "限流"
		state.Quota = 0
		state.QuotaKnown = true
		state.LimitsProgress = setLimitRemaining(state.LimitsProgress, "image_gen", 0)
		return state
	})
}

func NeedsImageQuotaRefresh(account PublicAccount, now time.Time) bool {
	hasQuotaMessage, resetAt, hasResetAt := imageGenQuotaWindow(account)
	if !hasQuotaMessage {
		return true
	}
	if !hasResetAt {
		return true
	}
	return !now.Before(resetAt)
}

func NeedsImageQuotaRefreshWithTTL(account PublicAccount, now time.Time, ttl time.Duration) bool {
	hasQuotaMessage, resetAt, hasResetAt := imageGenQuotaWindow(account)
	if !hasQuotaMessage {
		return true
	}
	if hasResetAt && !now.Before(resetAt) {
		return true
	}

	if ttl <= 0 {
		ttl = 120 * time.Second
	}
	lastRefreshedAt, ok := parseFlexibleTime(account.LastRefreshedAt)
	if !ok {
		return false
	}
	return now.Sub(lastRefreshedAt) >= ttl
}

func isUsableImageAccount(account PublicAccount, allowDisabled bool) bool {
	return (allowDisabled || account.Status != "禁用") &&
		account.Status != "异常" &&
		account.Status != "限流" &&
		account.Quota > 0
}

func imageGenQuotaWindow(account PublicAccount) (bool, time.Time, bool) {
	for _, item := range account.LimitsProgress {
		if strings.TrimSpace(strings.ToLower(stringValue(item["feature_name"]))) != "image_gen" {
			continue
		}
		resetAt := firstNonEmpty(stringValue(item["reset_after"]), account.RestoreAt)
		parsedTime, ok := parseQuotaResetTime(resetAt)
		return true, parsedTime, ok
	}
	return false, time.Time{}, false
}

func parseQuotaResetTime(value string) (time.Time, bool) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, cleaned)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func decrementLimitRemaining(limits []map[string]any, featureName string) []map[string]any {
	if len(limits) == 0 {
		return limits
	}

	next := cloneSlice(limits)
	target := strings.TrimSpace(strings.ToLower(featureName))
	for index, item := range next {
		if strings.TrimSpace(strings.ToLower(stringValue(item["feature_name"]))) != target {
			continue
		}

		remaining := intValue(item["remaining"])
		if remaining <= 0 {
			return next
		}
		item["remaining"] = remaining - 1
		next[index] = item
		return next
	}

	return next
}

func setLimitRemaining(limits []map[string]any, featureName string, remaining int) []map[string]any {
	if len(limits) == 0 {
		return []map[string]any{
			{
				"feature_name": strings.TrimSpace(featureName),
				"remaining":    remaining,
			},
		}
	}

	next := cloneSlice(limits)
	target := strings.TrimSpace(strings.ToLower(featureName))
	for index, item := range next {
		if strings.TrimSpace(strings.ToLower(stringValue(item["feature_name"]))) != target {
			continue
		}
		item["remaining"] = remaining
		next[index] = item
		return next
	}

	next = append(next, map[string]any{
		"feature_name": strings.TrimSpace(featureName),
		"remaining":    remaining,
	})
	return next
}

func (s *Store) SyncStatus(ctx context.Context, client *cliproxy.Client) (*SyncSummary, error) {
	summary, err := s.buildSyncSummary(ctx, client)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	summary.LastRun = cloneSyncRunResult(s.lastSyncRun)
	s.mu.Unlock()
	return summary, nil
}

func (s *Store) RunSync(ctx context.Context, client *cliproxy.Client, direction string) (*SyncRunResult, error) {
	startedAt := time.Now().UTC().Format(time.RFC3339)
	mode := normalizeSyncDirection(direction)
	if client == nil || !client.Configured() {
		result := &SyncRunResult{
			OK:         false,
			Running:    false,
			Error:      "cliproxy sync is not configured",
			Direction:  mode,
			StartedAt:  startedAt,
			FinishedAt: time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		s.setLastSyncRun(result)
		return result, nil
	}

	status, err := s.buildSyncSummary(ctx, client)
	if err != nil {
		result := &SyncRunResult{
			OK:         false,
			Running:    false,
			Error:      err.Error(),
			Direction:  mode,
			StartedAt:  startedAt,
			FinishedAt: time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		s.setLastSyncRun(result)
		return result, nil
	}

	toUpload := make([]string, 0)
	toDownload := make([]string, 0)
	disabledMismatch := make([]SyncAccountStatus, 0)
	remoteDeleted := 0
	for _, item := range status.Accounts {
		switch item.Status {
		case "pending_upload":
			if mode == "push" {
				toUpload = append(toUpload, item.Name)
			}
		case "remote_only":
			if mode == "pull" {
				toDownload = append(toDownload, item.Name)
			}
		case "remote_deleted":
			remoteDeleted++
			if mode == "push" {
				toUpload = append(toUpload, item.Name)
			}
		case "synced":
			if item.LocalDisabled != nil && item.RemoteDisabled != nil && *item.LocalDisabled != *item.RemoteDisabled {
				disabledMismatch = append(disabledMismatch, item)
			}
		}
	}

	result := &SyncRunResult{
		OK:            true,
		Running:       true,
		Direction:     mode,
		RemoteDeleted: remoteDeleted,
		StartedAt:     startedAt,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if mode == "pull" {
		result.Total = len(toDownload) + len(disabledMismatch)
		result.Phase = "准备从 CPA 拉取"
	} else {
		result.Total = len(toUpload) + len(disabledMismatch)
		result.Phase = "准备推送至 CPA"
	}
	s.setLastSyncRun(result)

	for _, name := range toUpload {
		result.Phase = "推送账号"
		result.Current = name
		result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		s.setLastSyncRun(result)
		data, readErr := s.readAuthRaw(name)
		if readErr != nil {
			result.OK = false
			result.UploadFailed++
			slog.Warn("sync upload read failed", "file", name, "error", readErr)
		} else if err := client.UploadAuthFile(ctx, name, data); err != nil {
			result.OK = false
			result.UploadFailed++
			slog.Warn("sync upload failed", "file", name, "error", err)
		} else {
			result.Uploaded++
			s.markSynced(name, "local")
		}
		result.Processed++
		result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		s.setLastSyncRun(result)
	}

	for _, name := range toDownload {
		result.Phase = "拉取账号"
		result.Current = name
		result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		s.setLastSyncRun(result)
		data, downloadErr := client.DownloadAuthFile(ctx, name)
		if downloadErr != nil {
			result.OK = false
			result.DownloadFailed++
			slog.Warn("sync download failed", "file", name, "error", downloadErr)
		} else if err := s.saveAuthBytes(name, data); err != nil {
			result.OK = false
			result.DownloadFailed++
			slog.Warn("sync save failed", "file", name, "error", err)
		} else {
			result.Downloaded++
			s.markSynced(name, "remote")
		}
		result.Processed++
		result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		s.setLastSyncRun(result)
	}

	for _, item := range disabledMismatch {
		result.Phase = "对齐禁用状态"
		result.Current = item.Name
		result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		s.setLastSyncRun(result)
		switch mode {
		case "pull":
			if item.RemoteDisabled == nil {
				result.Processed++
				result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				s.setLastSyncRun(result)
				continue
			}
			if err := s.alignLocalAuthDisabled(item.Name, *item.RemoteDisabled); err != nil {
				result.OK = false
				result.DisabledAlignFailed++
				slog.Warn("sync local disable align failed", "file", item.Name, "error", err)
			} else {
				s.markSynced(item.Name, "remote")
				result.DisabledAligned++
			}
		default:
			if item.LocalDisabled == nil {
				result.Processed++
				result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				s.setLastSyncRun(result)
				continue
			}
			if err := client.PatchAuthFileStatus(ctx, item.Name, *item.LocalDisabled); err != nil {
				result.OK = false
				result.DisabledAlignFailed++
				slog.Warn("sync disable align failed", "file", item.Name, "error", err)
			} else {
				s.markSynced(item.Name, "local")
				result.DisabledAligned++
			}
		}
		result.Processed++
		result.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		s.setLastSyncRun(result)
	}

	result.Running = false
	result.Current = ""
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	result.UpdatedAt = result.FinishedAt
	s.setLastSyncRun(result)
	return result, nil
}

func normalizeSyncDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "pull", "from_cpa", "from-cpa":
		return "pull"
	case "push", "to_cpa", "to-cpa":
		return "push"
	default:
		return "push"
	}
}

func (s *Store) buildSyncSummary(ctx context.Context, client *cliproxy.Client) (*SyncSummary, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, err
	}
	localAuths = filterSyncableLocalAuths(localAuths)
	syncStates := s.loadAllSyncStates()

	summary := &SyncSummary{
		Configured: client != nil && client.Configured(),
		Accounts:   []SyncAccountStatus{},
		Summary: map[string]int{
			"synced":         0,
			"pending_upload": 0,
			"remote_only":    0,
			"remote_deleted": 0,
		},
	}
	if client == nil || !client.Configured() {
		summary.Local = len(localAuths)
		return summary, nil
	}

	remoteMap, err := client.ListAuthFiles(ctx)
	if err != nil {
		return nil, err
	}

	localMap := make(map[string]LocalAuth, len(localAuths))
	for _, auth := range localAuths {
		localMap[auth.Name] = auth
	}

	allNames := make([]string, 0, len(localMap)+len(remoteMap))
	nameSet := map[string]struct{}{}
	for name := range localMap {
		nameSet[name] = struct{}{}
		allNames = append(allNames, name)
	}
	for name := range remoteMap {
		if _, ok := nameSet[name]; ok {
			continue
		}
		nameSet[name] = struct{}{}
		allNames = append(allNames, name)
	}
	sort.Strings(allNames)

	for _, name := range allNames {
		localAuth, inLocal := localMap[name]
		remoteAuth, inRemote := remoteMap[name]
		state := syncStates[name]

		item := SyncAccountStatus{Name: name}
		switch {
		case inLocal && inRemote:
			item.Status = "synced"
			item.Location = "both"
		case inLocal && !inRemote:
			if state.LastSyncedAt != "" {
				item.Status = "remote_deleted"
				item.Location = "local"
			} else {
				item.Status = "pending_upload"
				item.Location = "local"
			}
		case !inLocal && inRemote:
			item.Status = "remote_only"
			item.Location = "remote"
		}

		if inLocal {
			item.LocalDisabled = boolPtr(localAuth.Disabled)
		}
		if inRemote {
			item.RemoteDisabled = boolPtr(remoteAuth.Disabled)
		}
		if item.LocalDisabled != nil && item.RemoteDisabled != nil && *item.LocalDisabled != *item.RemoteDisabled && item.Status == "synced" {
			summary.DisabledMismatch++
		}
		summary.Summary[item.Status]++
		summary.Accounts = append(summary.Accounts, item)
	}

	sort.Slice(summary.Accounts, func(i, j int) bool {
		if syncRank(summary.Accounts[i].Status) != syncRank(summary.Accounts[j].Status) {
			return syncRank(summary.Accounts[i].Status) < syncRank(summary.Accounts[j].Status)
		}
		return summary.Accounts[i].Name < summary.Accounts[j].Name
	})

	summary.Local = len(localMap)
	summary.Remote = len(remoteMap)
	return summary, nil
}

func (s *Store) loadAuths() ([]LocalAuth, error) {
	items, err := s.storage().LoadAuths()
	if err != nil {
		return nil, err
	}

	result := make([]LocalAuth, 0, len(items))
	for _, auth := range items {
		if !s.matchesProvider(auth.Provider) {
			continue
		}
		info, infoErr := os.Stat(auth.Path)
		auth.ImportedAt = resolveAuthImportedAt(auth.Data, infoErr, info, auth.Name)
		result = append(result, auth)
	}
	return result, nil
}

func (s *Store) buildPublicAccount(auth LocalAuth, syncState SyncState, remoteDisabled *bool) PublicAccount {
	state := s.states[auth.Name]

	accountType := firstNonEmpty(state.Type, normalizePlanType(guessPlanFromPayload(auth.Data)), "Free")
	quota := state.Quota
	if !state.QuotaKnown {
		quota = s.defaultQuota
	}
	limitsProgress := cloneSlice(state.LimitsProgress)
	status := strings.TrimSpace(state.Status)
	if auth.Disabled {
		status = "禁用"
	} else if status == "" {
		if state.QuotaKnown && quota == 0 {
			status = "限流"
		} else {
			status = "正常"
		}
	}

	if accountType == "Pro" {
		hasImageGen := false
		for _, item := range limitsProgress {
			if strings.TrimSpace(strings.ToLower(stringValue(item["feature_name"]))) == "image_gen" {
				hasImageGen = true
				break
			}
		}
		if !hasImageGen {
			if !state.QuotaKnown || quota == 0 {
				quota = proFallbackImageGenQuota
			}
			limitsProgress = append(limitsProgress, map[string]any{
				"feature_name": "image_gen",
				"remaining":    quota,
			})
			if !auth.Disabled && status == "限流" {
				status = "正常"
			}
		}
	}

	syncStatus := ""
	if IsSyncableAccountSourceKind(auth.SourceKind) {
		if syncState.LastSyncedAt != "" {
			syncStatus = "synced"
		} else if auth.Name != "" {
			syncStatus = "pending_upload"
		}
	}

	return PublicAccount{
		ID:               shortID(auth.AccessToken),
		FileName:         auth.Name,
		AccessToken:      auth.AccessToken,
		SourceKind:       normalizeAccountSourceKind(auth.SourceKind),
		Type:             accountType,
		Status:           status,
		Quota:            max(0, quota),
		Email:            firstNonEmpty(state.Email, auth.Email),
		UserID:           firstNonEmpty(state.UserID, auth.UserID),
		LimitsProgress:   limitsProgress,
		DefaultModelSlug: state.DefaultModelSlug,
		RestoreAt:        state.RestoreAt,
		Success:          state.Success,
		Fail:             state.Fail,
		LastUsedAt:       state.LastUsedAt,
		LastRefreshedAt:  state.LastRefreshedAt,
		Provider:         auth.Provider,
		Disabled:         auth.Disabled,
		Note:             auth.Note,
		Priority:         auth.Priority,
		SyncStatus:       syncStatus,
		SyncOrigin:       syncState.Origin,
		LastSyncedAt:     syncState.LastSyncedAt,
		RemoteDisabled:   remoteDisabled,
		ImportedAt:       formatAccountImportedAt(auth.ImportedAt),
	}
}

func (s *Store) loadState() error {
	payload, err := s.storage().LoadRuntimeStates()
	if err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]RuntimeState{}
	}
	s.states = payload
	return nil
}

func (s *Store) saveStateLocked() error {
	return s.storage().SaveRuntimeStates(s.states)
}

func (s *Store) getState(name string) RuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[name]
}

func (s *Store) setState(name string, state RuntimeState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[name] = state
	_ = s.saveStateLocked()
}

func (s *Store) upsertState(name string, updater func(RuntimeState) RuntimeState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[name] = updater(s.states[name])
	_ = s.saveStateLocked()
}

func (s *Store) removeState(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, name)
	_ = s.saveStateLocked()
}

func (s *Store) setLastSyncRun(result *SyncRunResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSyncRun = cloneSyncRunResult(result)
}

func cloneSyncRunResult(result *SyncRunResult) *SyncRunResult {
	if result == nil {
		return nil
	}
	copy := *result
	return &copy
}

func (s *Store) ensureSyncStateInitialized() error {
	localAuths, err := s.loadAuths()
	if err != nil {
		return err
	}
	return s.storage().EnsureSyncStateInitialized(localAuths)
}

func (s *Store) loadAllSyncStates() map[string]SyncState {
	result, err := s.storage().LoadSyncStates()
	if err != nil {
		return map[string]SyncState{}
	}
	return result
}

func (s *Store) markLocalNew(name string) {
	_ = s.storage().SaveSyncState(SyncState{Name: name, Origin: "local"})
}

func (s *Store) markSynced(name, origin string) {
	_ = s.storage().SaveSyncState(SyncState{
		Name:         name,
		Origin:       firstNonEmpty(origin, "local"),
		LastSyncedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Store) deleteSyncState(name string) {
	_ = s.storage().DeleteSyncState(name)
}

func (s *Store) syncStatePath(name string) string {
	key := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	return filepath.Join(s.syncStateDir, key+".json")
}

func (s *Store) newAuthFileData(accessToken string) map[string]any {
	payload := decodeAccessTokenPayload(accessToken)
	authData := map[string]any{
		"type":         firstNonEmpty(s.providerType, "codex"),
		"source_kind":  AccountSourceKindToken,
		"access_token": strings.TrimSpace(accessToken),
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	}
	if email := guessEmail(payload); email != "" {
		authData["email"] = email
	}
	if userID := guessUserID(payload); userID != "" {
		authData["user_id"] = userID
	}
	if plan := guessPlanFromPayload(payload); plan != "" {
		authData["chatgpt_plan_type"] = strings.ToLower(plan)
	}
	return authData
}

func (s *Store) newAuthFileName(authData map[string]any, existing map[string]struct{}) string {
	stem := sanitizeFileStem(firstNonEmpty(stringValue(authData["email"]), stringValue(authData["user_id"])))
	if stem == "" {
		stem = "auth-" + shortID(stringValue(authData["access_token"]))
	}
	name := stem + ".json"
	if _, ok := existing[name]; !ok {
		return name
	}
	hash := shortID(stringValue(authData["access_token"]))
	return fmt.Sprintf("%s-%s.json", stem, hash)
}

func (s *Store) findAuthByToken(accessToken string) (LocalAuth, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return LocalAuth{}, err
	}
	token := strings.TrimSpace(accessToken)
	for _, auth := range localAuths {
		if auth.AccessToken == token {
			return auth, nil
		}
	}
	return LocalAuth{}, fmt.Errorf("account not found")
}

func normalizeAccountSourceKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case AccountSourceKindToken:
		return AccountSourceKindToken
	default:
		return AccountSourceKindAuthFile
	}
}

func resolveAccountSourceKind(data map[string]any) string {
	if kind, ok := explicitAccountSourceKind(stringValue(data["source_kind"])); ok {
		return kind
	}
	if looksLikeTokenAccountData(data) {
		return AccountSourceKindToken
	}
	return AccountSourceKindAuthFile
}

func explicitAccountSourceKind(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case AccountSourceKindToken:
		return AccountSourceKindToken, true
	case AccountSourceKindAuthFile:
		return AccountSourceKindAuthFile, true
	default:
		return "", false
	}
}

func looksLikeTokenAccountData(data map[string]any) bool {
	if len(data) == 0 || strings.TrimSpace(stringValue(data["access_token"])) == "" {
		return false
	}

	for _, key := range []string{
		"cookies",
		"cookie",
		"refresh_token",
		"session_cookie",
		"oai_device_id",
		"oai-device-id",
		"oai_session_id",
		"oai-session-id",
		"impersonate",
		"proxy_url",
		"account_id",
		"chatgpt_account_id",
		"last_refresh",
		"last_refresh_at",
		"expires_at",
	} {
		if hasMeaningfulAccountField(data[key]) {
			return false
		}
	}

	allowedKeys := map[string]struct{}{
		"type":              {},
		"provider":          {},
		"source_kind":       {},
		"access_token":      {},
		"created_at":        {},
		"email":             {},
		"user_id":           {},
		"chatgpt_plan_type": {},
		"disabled":          {},
		"note":              {},
		"priority":          {},
	}
	for key, value := range data {
		if _, ok := allowedKeys[strings.ToLower(strings.TrimSpace(key))]; ok {
			continue
		}
		if hasMeaningfulAccountField(value) {
			return false
		}
	}
	return true
}

func hasMeaningfulAccountField(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return typed
	case float64:
		return typed != 0
	case float32:
		return typed != 0
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return strings.TrimSpace(fmt.Sprint(value)) != ""
	}
}

func IsSyncableAccountSourceKind(value string) bool {
	return normalizeAccountSourceKind(value) != AccountSourceKindToken
}

func refreshProgressLabel(auth LocalAuth, accessToken string) string {
	return firstNonEmpty(auth.Email, auth.UserID, strings.TrimSuffix(auth.Name, filepath.Ext(auth.Name)), shortID(accessToken))
}

func filterSyncableLocalAuths(auths []LocalAuth) []LocalAuth {
	if len(auths) == 0 {
		return nil
	}
	items := make([]LocalAuth, 0, len(auths))
	for _, auth := range auths {
		if !IsSyncableAccountSourceKind(auth.SourceKind) {
			continue
		}
		items = append(items, auth)
	}
	return items
}

func (s *Store) findAuthByName(name string) (LocalAuth, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return LocalAuth{}, err
	}
	target := filepath.Base(strings.TrimSpace(name))
	for _, auth := range localAuths {
		if auth.Name == target {
			return auth, nil
		}
	}
	return LocalAuth{}, fmt.Errorf("account not found")
}

func (s *Store) alignLocalAuthDisabled(name string, disabled bool) error {
	auth, err := s.findAuthByName(name)
	if err != nil {
		return err
	}

	auth.Disabled = disabled
	auth.Data["disabled"] = disabled
	if err := s.saveAuthData(auth.Name, auth.Data); err != nil {
		return err
	}

	s.upsertState(auth.Name, func(state RuntimeState) RuntimeState {
		if disabled {
			state.Status = "禁用"
		} else if state.Status == "禁用" || strings.TrimSpace(state.Status) == "" {
			if state.QuotaKnown && state.Quota == 0 {
				state.Status = "限流"
			} else {
				state.Status = "正常"
			}
		}
		return state
	})
	return nil
}

func (s *Store) matchesProvider(provider string) bool {
	expected := strings.ToLower(strings.TrimSpace(s.providerType))
	if expected == "" {
		return true
	}
	actual := strings.ToLower(strings.TrimSpace(provider))
	return actual == "" || actual == expected
}

func accountRank(account PublicAccount) int {
	switch account.Status {
	case "正常":
		return 0
	case "限流":
		return 1
	case "异常":
		return 2
	case "禁用":
		return 3
	default:
		return 4
	}
}

func syncRank(status string) int {
	switch status {
	case "pending_upload":
		return 0
	case "remote_only":
		return 1
	case "remote_deleted":
		return 2
	case "synced":
		return 3
	default:
		return 4
	}
}

func shortID(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:16]
}

func dedupeTokens(tokens []string) []string {
	result := make([]string, 0, len(tokens))
	seen := map[string]struct{}{}
	for _, token := range tokens {
		cleaned := strings.TrimSpace(token)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func authIdentityKey(data map[string]any, provider string) string {
	accountType := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringValue(data["type"]), provider)))
	identity := firstNonEmpty(
		stringValue(data["account_id"]),
		stringValue(data["chatgpt_account_id"]),
		guessUserID(data),
		guessEmail(data),
	)
	identity = strings.ToLower(strings.TrimSpace(identity))
	if identity == "" {
		return ""
	}
	return accountType + "::" + identity
}

func AuthIdentityKey(data map[string]any, provider string) string {
	return authIdentityKey(data, provider)
}

func authFreshnessTime(data map[string]any) time.Time {
	for _, key := range []string{"last_refresh", "last_refreshed_at", "updated_at", "modified_at", "created_at"} {
		if parsed, ok := parseFlexibleTime(stringValue(data[key])); ok {
			return parsed
		}
	}
	return time.Time{}
}

func parseFlexibleTime(value string) (time.Time, bool) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
	} {
		parsed, err := time.Parse(layout, cleaned)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func resolveAuthImportedAt(data map[string]any, infoErr error, info os.FileInfo, fallbackName string) time.Time {
	if parsed, ok := parseFlexibleTime(stringValue(data["created_at"])); ok {
		return parsed
	}
	if parsed, ok := parseFlexibleTime(stringValue(data["imported_at"])); ok {
		return parsed
	}
	if infoErr == nil && info != nil && !info.ModTime().IsZero() {
		return info.ModTime()
	}
	if strings.TrimSpace(fallbackName) == "" {
		return time.Time{}
	}
	return time.Time{}
}

func formatAccountImportedAt(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func syncImageQuotaDailyBaseState(state RuntimeState, quota int, limitsProgress []map[string]any, restoreAt string) RuntimeState {
	remaining, resetAt, ok := extractImageQuotaSnapshot(limitsProgress, restoreAt, quota)
	if !ok || remaining <= 0 {
		return state
	}

	if resetAt != "" && strings.TrimSpace(state.ImageQuotaDailyBaseResetAt) != strings.TrimSpace(resetAt) {
		state.ImageQuotaDailyBase = remaining
		state.ImageQuotaDailyBaseResetAt = strings.TrimSpace(resetAt)
		return state
	}

	if state.ImageQuotaDailyBase <= 0 {
		state.ImageQuotaDailyBase = remaining
	}
	if strings.TrimSpace(state.ImageQuotaDailyBaseResetAt) == "" && strings.TrimSpace(resetAt) != "" {
		state.ImageQuotaDailyBaseResetAt = strings.TrimSpace(resetAt)
	}
	return state
}

func extractImageQuotaSnapshot(limitsProgress []map[string]any, restoreAt string, quota int) (int, string, bool) {
	for _, item := range limitsProgress {
		if strings.TrimSpace(strings.ToLower(stringValue(item["feature_name"]))) != "image_gen" {
			continue
		}
		remaining := intValue(item["remaining"])
		resetAt := firstNonEmpty(stringValue(item["reset_after"]), restoreAt)
		return max(0, remaining), resetAt, true
	}

	if quota > 0 {
		return quota, strings.TrimSpace(restoreAt), true
	}
	return 0, strings.TrimSpace(restoreAt), false
}

func decodeAccessTokenPayload(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	payload := parts[1]
	payload += strings.Repeat("=", (4-len(payload)%4)%4)
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return map[string]any{}
	}
	result := map[string]any{}
	if err := json.Unmarshal(decoded, &result); err != nil {
		return map[string]any{}
	}
	return result
}

func guessEmail(data map[string]any) string {
	if email := strings.TrimSpace(stringValue(data["email"])); email != "" {
		return email
	}
	if authPayload, ok := data["https://api.openai.com/auth"].(map[string]any); ok {
		if email := strings.TrimSpace(stringValue(authPayload["email"])); email != "" {
			return email
		}
	}
	return ""
}

func guessUserID(data map[string]any) string {
	if userID := strings.TrimSpace(stringValue(data["user_id"])); userID != "" {
		return userID
	}
	if userID := strings.TrimSpace(stringValue(data["sub"])); userID != "" {
		return userID
	}
	if authPayload, ok := data["https://api.openai.com/auth"].(map[string]any); ok {
		if userID := strings.TrimSpace(stringValue(authPayload["chatgpt_account_id"])); userID != "" {
			return userID
		}
	}
	return ""
}

func guessPlanFromPayload(data map[string]any) string {
	if authPayload, ok := data["https://api.openai.com/auth"].(map[string]any); ok {
		if plan := normalizePlanType(stringValue(authPayload["chatgpt_plan_type"])); plan != "" {
			return plan
		}
	}
	if plan := normalizePlanType(stringValue(data["chatgpt_plan_type"])); plan != "" {
		return plan
	}
	if plan := normalizePlanType(stringValue(data["plan_type"])); plan != "" {
		return plan
	}
	if plan := normalizePlanType(stringValue(data["account_type"])); plan != "" {
		return plan
	}
	if token := strings.TrimSpace(stringValue(data["access_token"])); token != "" {
		if tokenPayload := decodeAccessTokenPayload(token); len(tokenPayload) > 0 {
			if plan := guessPlanFromPayload(tokenPayload); plan != "" {
				return plan
			}
		}
	}
	return ""
}

func normalizePlanType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "free":
		return "Free"
	case "plus", "personal":
		return "Plus"
	case "team", "business", "enterprise":
		return "Team"
	case "pro":
		return "Pro"
	default:
		return strings.TrimSpace(value)
	}
}

func NormalizePlanType(value string) string {
	return normalizePlanType(value)
}

func writeJSONFile(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesFile(path, append(raw, '\n'))
}

func writeBytesFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSONFile(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func cloneSlice(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return []map[string]any{}
	}
	raw, _ := json.Marshal(items)
	var cloned []map[string]any
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func cloneRuntimeStatesMap(items map[string]RuntimeState) map[string]RuntimeState {
	if len(items) == 0 {
		return map[string]RuntimeState{}
	}
	raw, _ := json.Marshal(items)
	var cloned map[string]RuntimeState
	_ = json.Unmarshal(raw, &cloned)
	if cloned == nil {
		return map[string]RuntimeState{}
	}
	return cloned
}

func sanitizeFileStem(value string) string {
	replacer := strings.NewReplacer("@", "-", ".", "-", " ", "-", "/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	cleaned := strings.Trim(strings.ToLower(replacer.Replace(value)), "-")
	if cleaned == "" {
		return ""
	}
	return cleaned
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
	case fmt.Stringer:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	case string:
		var n int
		fmt.Sscanf(strings.TrimSpace(typed), "%d", &n)
		return n
	default:
		return 0
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func boolPtr(value bool) *bool {
	v := value
	return &v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
