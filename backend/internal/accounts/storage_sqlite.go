package accounts

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type sqliteAccountStorage struct {
	path string
	db   *sql.DB
}

func newSQLiteAccountStorage(path string) (accountStorageBackend, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &sqliteAccountStorage{path: path, db: db}, nil
}

func (s *sqliteAccountStorage) Init() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS auth_files (name TEXT PRIMARY KEY, raw_json BLOB NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS runtime_state (id INTEGER PRIMARY KEY CHECK (id = 1), raw_json BLOB NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS sync_states (name TEXT PRIMARY KEY, raw_json BLOB NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteAccountStorage) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *sqliteAccountStorage) LoadAuths() ([]LocalAuth, error) {
	rows, err := s.db.Query(`SELECT name, raw_json FROM auth_files ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]LocalAuth, 0)
	for rows.Next() {
		var name string
		var raw []byte
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, err
		}
		auth, err := parseLocalAuth(name, name, raw, "")
		if err != nil {
			continue
		}
		result = append(result, auth)
	}
	return result, rows.Err()
}

func (s *sqliteAccountStorage) ReadAuthRaw(name string) ([]byte, error) {
	var raw []byte
	err := s.db.QueryRow(`SELECT raw_json FROM auth_files WHERE name = ?`, filepath.Base(strings.TrimSpace(name))).Scan(&raw)
	return raw, err
}

func (s *sqliteAccountStorage) SaveAuthRaw(name string, raw []byte) error {
	_, err := s.db.Exec(`INSERT INTO auth_files(name, raw_json) VALUES(?, ?) ON CONFLICT(name) DO UPDATE SET raw_json = excluded.raw_json`, filepath.Base(strings.TrimSpace(name)), raw)
	return err
}

func (s *sqliteAccountStorage) DeleteAuth(name string) error {
	_, err := s.db.Exec(`DELETE FROM auth_files WHERE name = ?`, filepath.Base(strings.TrimSpace(name)))
	return err
}

func (s *sqliteAccountStorage) LoadRuntimeStates() (map[string]RuntimeState, error) {
	var raw []byte
	err := s.db.QueryRow(`SELECT raw_json FROM runtime_state WHERE id = 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return map[string]RuntimeState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result map[string]RuntimeState
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]RuntimeState{}, nil
	}
	return result, nil
}

func (s *sqliteAccountStorage) SaveRuntimeStates(states map[string]RuntimeState) error {
	raw, err := json.Marshal(states)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO runtime_state(id, raw_json) VALUES(1, ?) ON CONFLICT(id) DO UPDATE SET raw_json = excluded.raw_json`, raw)
	return err
}

func (s *sqliteAccountStorage) LoadSyncStates() (map[string]SyncState, error) {
	rows, err := s.db.Query(`SELECT name, raw_json FROM sync_states`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]SyncState{}
	for rows.Next() {
		var name string
		var raw []byte
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, err
		}
		var state SyncState
		if err := json.Unmarshal(raw, &state); err != nil {
			continue
		}
		if state.Name == "" {
			state.Name = name
		}
		result[state.Name] = state
	}
	return result, rows.Err()
}

func (s *sqliteAccountStorage) SaveSyncState(state SyncState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO sync_states(name, raw_json) VALUES(?, ?) ON CONFLICT(name) DO UPDATE SET raw_json = excluded.raw_json`, state.Name, raw)
	return err
}

func (s *sqliteAccountStorage) DeleteSyncState(name string) error {
	_, err := s.db.Exec(`DELETE FROM sync_states WHERE name = ?`, name)
	return err
}

func (s *sqliteAccountStorage) EnsureSyncStateInitialized(auths []LocalAuth) error {
	var initialized string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = 'sync_bootstrap'`).Scan(&initialized)
	if err == nil && initialized == "1" {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, auth := range auths {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(1) FROM sync_states WHERE name = ?`, auth.Name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		raw, err := json.Marshal(SyncState{Name: auth.Name, Origin: "local", LastSyncedAt: now})
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(`INSERT INTO sync_states(name, raw_json) VALUES(?, ?)`, auth.Name, raw); err != nil {
			return err
		}
	}

	_, err = s.db.Exec(`INSERT INTO metadata(key, value) VALUES('sync_bootstrap', '1') ON CONFLICT(key) DO UPDATE SET value = '1'`)
	return err
}

func (s *sqliteAccountStorage) LoadImageRoutingPolicy() (*ImageAccountRoutingPolicy, error) {
	var raw []byte
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = 'image_account_policy'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var policy ImageAccountRoutingPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func (s *sqliteAccountStorage) SaveImageRoutingPolicy(policy ImageAccountRoutingPolicy) error {
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO metadata(key, value) VALUES('image_account_policy', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, raw)
	return err
}
