package users

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"chatgpt2api/internal/config"

	_ "modernc.org/sqlite"
)

func newStorageBackend(cfg *config.Config) storageBackend {
	return &sqliteBackend{path: cfg.ResolvePath(cfg.Storage.SQLitePath)}
}

type sqliteBackend struct {
	path string
	db   *sql.DB
}

func (b *sqliteBackend) Init() error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", b.path)
	if err != nil {
		return err
	}
	b.db = db
	_, err = b.db.Exec(`CREATE TABLE IF NOT EXISTS portal_users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL DEFAULT '',
		avatar_url TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL,
		disabled INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		last_login_at TEXT NOT NULL DEFAULT ''
	);`)
	if err != nil {
		return err
	}
	if err := b.ensureColumn("portal_users", "display_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return b.ensureColumn("portal_users", "avatar_url", "TEXT NOT NULL DEFAULT ''")
}

func (b *sqliteBackend) Close() error {
	if b.db == nil {
		return nil
	}
	return b.db.Close()
}

func (b *sqliteBackend) Load() ([]Record, error) {
	rows, err := b.db.Query(`SELECT id, email, display_name, avatar_url, password_hash, role, disabled, created_at, last_login_at FROM portal_users ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []Record{}
	for rows.Next() {
		var item Record
		var disabled int
		if err := rows.Scan(&item.ID, &item.Email, &item.DisplayName, &item.AvatarURL, &item.PasswordHash, &item.Role, &disabled, &item.CreatedAt, &item.LastLoginAt); err != nil {
			return nil, err
		}
		item.Disabled = disabled != 0
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (b *sqliteBackend) Save(items []Record) error {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM portal_users`); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO portal_users (id, email, display_name, avatar_url, password_hash, role, disabled, created_at, last_login_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range cloneSortedRecords(items) {
		disabled := 0
		if item.Disabled {
			disabled = 1
		}
		if _, err := stmt.Exec(item.ID, item.Email, item.DisplayName, item.AvatarURL, item.PasswordHash, item.Role, disabled, item.CreatedAt, item.LastLoginAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (b *sqliteBackend) ensureColumn(tableName, columnName, definition string) error {
	exists, err := b.columnExists(tableName, columnName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = b.db.Exec(`ALTER TABLE ` + tableName + ` ADD COLUMN ` + columnName + ` ` + definition)
	return err
}

func (b *sqliteBackend) columnExists(tableName, columnName string) (bool, error) {
	rows, err := b.db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			dataType   string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(columnName)) {
			return true, nil
		}
	}
	return false, rows.Err()
}
