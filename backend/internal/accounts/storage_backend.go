package accounts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatgpt2api/internal/config"
)

type accountStorageBackend interface {
	Init() error
	Close() error
	LoadAuths() ([]LocalAuth, error)
	ReadAuthRaw(name string) ([]byte, error)
	SaveAuthRaw(name string, raw []byte) error
	DeleteAuth(name string) error
	LoadRuntimeStates() (map[string]RuntimeState, error)
	SaveRuntimeStates(states map[string]RuntimeState) error
	LoadSyncStates() (map[string]SyncState, error)
	SaveSyncState(state SyncState) error
	DeleteSyncState(name string) error
	EnsureSyncStateInitialized(auths []LocalAuth) error
	LoadImageRoutingPolicy() (*ImageAccountRoutingPolicy, error)
	SaveImageRoutingPolicy(policy ImageAccountRoutingPolicy) error
}

func newAccountStorageBackend(cfg *config.Config, authDir, stateFile, syncStateDir, providerType string) (accountStorageBackend, error) {
	switch cfg.Storage.Backend {
	case "sqlite":
		return newSQLiteAccountStorage(cfg.ResolvePath(cfg.Storage.SQLitePath))
	case "redis":
		return newRedisAccountStorage(cfg.Storage.RedisAddr, cfg.Storage.RedisPassword, cfg.Storage.RedisDB, cfg.Storage.RedisPrefix)
	default:
		return newFileAccountStorage(authDir, stateFile, syncStateDir), nil
	}
}

func migrateFileStorageIfNeeded(target accountStorageBackend, backendName, authDir, stateFile, syncStateDir string) error {
	if strings.EqualFold(strings.TrimSpace(backendName), "current") || strings.EqualFold(strings.TrimSpace(backendName), "local") || target == nil {
		return nil
	}

	targetAuths, err := target.LoadAuths()
	if err != nil {
		return err
	}
	targetStates, err := target.LoadRuntimeStates()
	if err != nil {
		return err
	}
	targetSyncStates, err := target.LoadSyncStates()
	if err != nil {
		return err
	}
	if len(targetAuths) > 0 || len(targetStates) > 0 || len(targetSyncStates) > 0 {
		return nil
	}

	source := &fileAccountStorage{
		authDir:      authDir,
		stateFile:    stateFile,
		syncStateDir: syncStateDir,
	}
	if err := source.Init(); err != nil {
		return err
	}

	sourceAuths, err := source.LoadAuths()
	if err != nil {
		return err
	}
	for _, auth := range sourceAuths {
		raw, err := source.ReadAuthRaw(auth.Name)
		if err != nil {
			return err
		}
		if err := target.SaveAuthRaw(auth.Name, raw); err != nil {
			return err
		}
	}

	sourceStates, err := source.LoadRuntimeStates()
	if err != nil {
		return err
	}
	if len(sourceStates) > 0 {
		if err := target.SaveRuntimeStates(sourceStates); err != nil {
			return err
		}
	}

	sourceSyncStates, err := source.LoadSyncStates()
	if err != nil {
		return err
	}
	for _, state := range sourceSyncStates {
		if err := target.SaveSyncState(state); err != nil {
			return err
		}
	}

	return nil
}

func migrateIntoEmptyBackendIfNeeded(target accountStorageBackend, targetBackendName, authDir, stateFile, syncStateDir, sqlitePath, redisAddr, redisPassword, redisPrefix string, redisDB int) error {
	if target == nil {
		return nil
	}

	targetAuths, err := target.LoadAuths()
	if err != nil {
		return err
	}
	targetStates, err := target.LoadRuntimeStates()
	if err != nil {
		return err
	}
	targetSyncStates, err := target.LoadSyncStates()
	if err != nil {
		return err
	}
	if len(targetAuths) > 0 || len(targetStates) > 0 || len(targetSyncStates) > 0 {
		return nil
	}

	sources := make([]accountStorageBackend, 0, 3)
	switch strings.ToLower(strings.TrimSpace(targetBackendName)) {
	case "sqlite":
		sources = append(sources,
			newFileAccountStorage(authDir, stateFile, syncStateDir),
		)
		if backend, err := newRedisAccountStorage(redisAddr, redisPassword, redisDB, redisPrefix); err == nil {
			sources = append(sources, backend)
		}
	case "redis":
		sources = append(sources,
			newFileAccountStorage(authDir, stateFile, syncStateDir),
		)
		if backend, err := newSQLiteAccountStorage(sqlitePath); err == nil {
			sources = append(sources, backend)
		}
	default:
		if backend, err := newSQLiteAccountStorage(sqlitePath); err == nil {
			sources = append(sources, backend)
		}
		if backend, err := newRedisAccountStorage(redisAddr, redisPassword, redisDB, redisPrefix); err == nil {
			sources = append(sources, backend)
		}
	}

	for _, source := range sources {
		if source == nil {
			continue
		}
		_ = source.Init()
		auths, err := source.LoadAuths()
		if err != nil {
			_ = source.Close()
			continue
		}
		states, err := source.LoadRuntimeStates()
		if err != nil {
			_ = source.Close()
			continue
		}
		syncStates, err := source.LoadSyncStates()
		if err != nil {
			_ = source.Close()
			continue
		}
		if len(auths) == 0 && len(states) == 0 && len(syncStates) == 0 {
			_ = source.Close()
			continue
		}
		if err := cloneBackendData(source, target); err != nil {
			_ = source.Close()
			return err
		}
		_ = source.Close()
		return nil
	}

	return nil
}

func cloneBackendData(source, target accountStorageBackend) error {
	auths, err := source.LoadAuths()
	if err != nil {
		return err
	}
	for _, auth := range auths {
		raw, err := source.ReadAuthRaw(auth.Name)
		if err != nil {
			return err
		}
		if err := target.SaveAuthRaw(auth.Name, raw); err != nil {
			return err
		}
	}

	states, err := source.LoadRuntimeStates()
	if err != nil {
		return err
	}
	if len(states) > 0 {
		if err := target.SaveRuntimeStates(states); err != nil {
			return err
		}
	}

	syncStates, err := source.LoadSyncStates()
	if err != nil {
		return err
	}
	for _, state := range syncStates {
		if err := target.SaveSyncState(state); err != nil {
			return err
		}
	}

	policy, err := source.LoadImageRoutingPolicy()
	if err != nil {
		return err
	}
	if policy != nil {
		if err := target.SaveImageRoutingPolicy(*policy); err != nil {
			return err
		}
	}
	return nil
}

func parseLocalAuth(name, path string, raw []byte, fallbackProvider string) (LocalAuth, error) {
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return LocalAuth{}, err
	}
	sourceKind := resolveAccountSourceKind(data)
	data["source_kind"] = sourceKind
	auth := LocalAuth{
		Name:        name,
		Path:        path,
		Provider:    firstNonEmpty(stringValue(data["type"]), stringValue(data["provider"]), fallbackProvider),
		SourceKind:  sourceKind,
		AccessToken: strings.TrimSpace(stringValue(data["access_token"])),
		Email:       firstNonEmpty(stringValue(data["email"]), guessEmail(data)),
		UserID:      firstNonEmpty(stringValue(data["user_id"]), guessUserID(data)),
		Disabled:    boolValue(data["disabled"]),
		Note:        strings.TrimSpace(stringValue(data["note"])),
		Priority:    intValue(data["priority"]),
		Data:        data,
	}
	return auth, nil
}

type fileAccountStorage struct {
	authDir      string
	stateFile    string
	syncStateDir string
}

func newFileAccountStorage(authDir, stateFile, syncStateDir string) accountStorageBackend {
	return &fileAccountStorage{
		authDir:      authDir,
		stateFile:    stateFile,
		syncStateDir: syncStateDir,
	}
}

func (s *fileAccountStorage) Init() error {
	if err := os.MkdirAll(s.authDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.syncStateDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Dir(s.stateFile), 0o755)
}

func (s *fileAccountStorage) Close() error {
	return nil
}

func (s *fileAccountStorage) LoadAuths() ([]LocalAuth, error) {
	entries, err := os.ReadDir(s.authDir)
	if err != nil {
		return nil, err
	}

	result := make([]LocalAuth, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(s.authDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		auth, err := parseLocalAuth(entry.Name(), path, raw, "")
		if err != nil {
			continue
		}
		result = append(result, auth)
	}
	return result, nil
}

func (s *fileAccountStorage) ReadAuthRaw(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.authDir, filepath.Base(strings.TrimSpace(name))))
}

func (s *fileAccountStorage) SaveAuthRaw(name string, raw []byte) error {
	return writeBytesFile(filepath.Join(s.authDir, filepath.Base(strings.TrimSpace(name))), raw)
}

func (s *fileAccountStorage) DeleteAuth(name string) error {
	err := os.Remove(filepath.Join(s.authDir, filepath.Base(strings.TrimSpace(name))))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *fileAccountStorage) LoadRuntimeStates() (map[string]RuntimeState, error) {
	if _, err := os.Stat(s.stateFile); os.IsNotExist(err) {
		return map[string]RuntimeState{}, nil
	}
	var payload stateEnvelope
	if err := readJSONFile(s.stateFile, &payload); err != nil {
		return nil, err
	}
	if payload.Accounts == nil {
		return map[string]RuntimeState{}, nil
	}
	return payload.Accounts, nil
}

func (s *fileAccountStorage) SaveRuntimeStates(states map[string]RuntimeState) error {
	return writeJSONFile(s.stateFile, stateEnvelope{Accounts: states})
}

func (s *fileAccountStorage) LoadSyncStates() (map[string]SyncState, error) {
	result := map[string]SyncState{}
	entries, err := os.ReadDir(s.syncStateDir)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		var state SyncState
		if err := readJSONFile(filepath.Join(s.syncStateDir, entry.Name()), &state); err != nil {
			continue
		}
		if state.Name == "" {
			state.Name = entry.Name()
		}
		result[state.Name] = state
	}
	return result, nil
}

func (s *fileAccountStorage) SaveSyncState(state SyncState) error {
	return writeJSONFile(filepath.Join(s.syncStateDir, strings.TrimSuffix(filepath.Base(state.Name), filepath.Ext(state.Name))+".json"), state)
}

func (s *fileAccountStorage) DeleteSyncState(name string) error {
	err := os.Remove(filepath.Join(s.syncStateDir, strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))+".json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *fileAccountStorage) EnsureSyncStateInitialized(auths []LocalAuth) error {
	flag := filepath.Join(s.syncStateDir, ".migrated")
	if _, err := os.Stat(flag); err == nil {
		return nil
	}
	for _, auth := range auths {
		if _, err := os.Stat(filepath.Join(s.syncStateDir, strings.TrimSuffix(filepath.Base(auth.Name), filepath.Ext(auth.Name))+".json")); os.IsNotExist(err) {
			if err := s.SaveSyncState(SyncState{
				Name:         auth.Name,
				Origin:       "local",
				LastSyncedAt: time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(flag, []byte(time.Now().Format(time.RFC3339)), 0o644)
}

func (s *fileAccountStorage) LoadImageRoutingPolicy() (*ImageAccountRoutingPolicy, error) {
	path := s.imageRoutingPolicyFile()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	var policy ImageAccountRoutingPolicy
	if err := readJSONFile(path, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *fileAccountStorage) SaveImageRoutingPolicy(policy ImageAccountRoutingPolicy) error {
	return writeJSONFile(s.imageRoutingPolicyFile(), policy)
}

func (s *fileAccountStorage) imageRoutingPolicyFile() string {
	return filepath.Join(filepath.Dir(s.stateFile), "image_account_policy.json")
}

func (s *Store) saveAuthData(name string, data map[string]any) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return s.storage().SaveAuthRaw(name, raw)
}

func (s *Store) saveAuthBytes(name string, raw []byte) error {
	return s.storage().SaveAuthRaw(name, raw)
}

func (s *Store) readAuthRaw(name string) ([]byte, error) {
	return s.storage().ReadAuthRaw(name)
}

func (s *Store) deleteAuth(name string) error {
	return s.storage().DeleteAuth(name)
}

func unsupportedStorageError(name string) error {
	return fmt.Errorf("%s storage backend is not available", name)
}
