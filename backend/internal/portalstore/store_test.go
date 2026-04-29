package portalstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "portalstore.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	store := &Store{
		db:       db,
		imageDir: t.TempDir(),
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		t.Fatalf("init store: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
	})

	return store
}

func TestGetWorkLoadsCommentsWithJoinedUsers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE portal_users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			avatar_url TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'user',
			disabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			last_login_at TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create portal_users: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO portal_users (id, email, display_name, avatar_url, created_at)
		VALUES
			('user-1', 'user1@example.com', 'User One', 'https://example.com/u1.png', '2026-04-28T14:00:00Z'),
			('user-2', 'user2@example.com', 'User Two', 'https://example.com/u2.png', '2026-04-28T14:05:00Z')
	`); err != nil {
		t.Fatalf("insert portal_users: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO portal_gallery_works (
			id, user_id, user_email, title, prompt, image_filename, image_url, model, size, like_count, comment_count, created_at, updated_at
		)
		VALUES (
			'work-1', 'user-1', 'user1@example.com', 'Test Work', 'A prompt', 'work-1.png', 'https://example.com/work-1.png', 'gpt-image-2', '1024x1024', 0, 2, '2026-04-28T14:10:00Z', '2026-04-28T14:10:00Z'
		)
	`); err != nil {
		t.Fatalf("insert portal_gallery_works: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO portal_gallery_comments (
			id, work_id, user_id, user_email, content, created_at
		)
		VALUES
			('comment-2', 'work-1', 'user-2', 'user2@example.com', 'Second comment', '2026-04-28T14:12:00Z'),
			('comment-1', 'work-1', 'user-1', 'user1@example.com', 'First comment', '2026-04-28T14:11:00Z')
	`); err != nil {
		t.Fatalf("insert portal_gallery_comments: %v", err)
	}

	work, comments, err := store.GetWork(ctx, "work-1", "")
	if err != nil {
		t.Fatalf("GetWork returned error: %v", err)
	}

	if work.ID != "work-1" {
		t.Fatalf("work.ID = %q, want %q", work.ID, "work-1")
	}
	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}
	if comments[0].ID != "comment-1" || comments[1].ID != "comment-2" {
		t.Fatalf("comment order = [%q, %q], want [comment-1, comment-2]", comments[0].ID, comments[1].ID)
	}
	if comments[0].UserDisplayName != "User One" {
		t.Fatalf("comments[0].UserDisplayName = %q, want %q", comments[0].UserDisplayName, "User One")
	}
	if comments[1].UserDisplayName != "User Two" {
		t.Fatalf("comments[1].UserDisplayName = %q, want %q", comments[1].UserDisplayName, "User Two")
	}
}
