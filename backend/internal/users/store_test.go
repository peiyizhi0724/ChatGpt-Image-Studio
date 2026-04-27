package users

import (
	"os"
	"path/filepath"
	"testing"

	"chatgpt2api/internal/config"
)

func newUsersTestConfig(t *testing.T) *config.Config {
	t.Helper()
	root := t.TempDir()
	cfg := config.New(root)
	if err := cfg.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Storage.SQLitePath = "data/chatgpt-image-studio.db"
	return cfg
}

func TestSQLiteUsersStorePersistsAcrossReload(t *testing.T) {
	cfg := newUsersTestConfig(t)

	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	user, err := store.Register("Admin@Example.com", "secret123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Role != RoleAdmin {
		t.Fatalf("first user role = %q, want %q", user.Role, RoleAdmin)
	}

	if _, err := store.Authenticate("admin@example.com", "secret123"); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	role := RoleUser
	disabled := true
	if _, err := store.Update(user.ID, Update{Role: &role, Disabled: &disabled}); err == nil {
		t.Fatal("expected Update to reject removing last enabled admin")
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reloaded, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("Reloaded NewStore: %v", err)
	}
	defer reloaded.Close()

	items, err := reloaded.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("List count = %d, want 1", len(items))
	}
	if items[0].Email != "admin@example.com" {
		t.Fatalf("reloaded email = %q", items[0].Email)
	}
	if items[0].LastLoginAt == "" {
		t.Fatal("expected LastLoginAt to persist after reload")
	}

	if _, err := os.Stat(cfg.ResolvePath("data/users.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy users file should not be created, err=%v", err)
	}
}

func TestSQLiteUsersStoreIgnoresLegacyUsersFile(t *testing.T) {
	cfg := newUsersTestConfig(t)

	legacyPath := cfg.ResolvePath("data/users.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	legacyRaw := []byte("{\"users\":[{\"id\":\"legacy-admin\",\"email\":\"legacy@example.com\",\"password_hash\":\"hashed-password\",\"role\":\"admin\",\"disabled\":false,\"created_at\":\"2026-04-27T00:00:00Z\"}]}\n")
	if err := os.WriteFile(legacyPath, legacyRaw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	items, err := store.List()
	if err != nil {
		t.Fatalf("List before register: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("legacy users file should be ignored, got %d users", len(items))
	}

	if _, err := store.Register("fresh@example.com", "secret123"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	persisted, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("ReadFile legacy: %v", err)
	}
	if string(persisted) != string(legacyRaw) {
		t.Fatal("legacy users file should remain untouched")
	}
}

func TestSQLiteUsersStoreProfileAndPasswordFlows(t *testing.T) {
	cfg := newUsersTestConfig(t)

	store, err := NewStore(cfg)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	user, err := store.Register("artist@example.com", "secret123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	displayName := "画师A"
	avatarURL := "https://example.com/avatar.png"
	updated, err := store.UpdateProfile(user.ID, ProfileUpdate{
		DisplayName: &displayName,
		AvatarURL:   &avatarURL,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.DisplayName != displayName {
		t.Fatalf("display name = %q, want %q", updated.DisplayName, displayName)
	}
	if updated.AvatarURL != avatarURL {
		t.Fatalf("avatar url = %q, want %q", updated.AvatarURL, avatarURL)
	}

	if err := store.ChangePassword(user.ID, "secret123", "secret456"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := store.Authenticate("artist@example.com", "secret123"); err == nil {
		t.Fatal("expected old password to stop working after ChangePassword")
	}
	if _, err := store.Authenticate("artist@example.com", "secret456"); err != nil {
		t.Fatalf("Authenticate with changed password: %v", err)
	}

	if err := store.ResetPassword("artist@example.com", "secret789"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if _, err := store.Authenticate("artist@example.com", "secret456"); err == nil {
		t.Fatal("expected previous password to stop working after ResetPassword")
	}
	if _, err := store.Authenticate("artist@example.com", "secret789"); err != nil {
		t.Fatalf("Authenticate with reset password: %v", err)
	}
}
