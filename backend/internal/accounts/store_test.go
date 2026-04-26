package accounts

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNeedsImageQuotaRefreshWithoutQuotaMessage(t *testing.T) {
	account := PublicAccount{
		Status:         "正常",
		Quota:          5,
		LimitsProgress: nil,
	}

	if !NeedsImageQuotaRefresh(account, time.Now()) {
		t.Fatal("expected refresh to be required when image_gen quota message is missing")
	}
}

func TestNeedsImageQuotaRefreshByResetTime(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)

	freshAccount := PublicAccount{
		Status: "正常",
		Quota:  3,
		LimitsProgress: []map[string]any{
			{
				"feature_name": "image_gen",
				"remaining":    3,
				"reset_after":  now.Add(2 * time.Hour).Format(time.RFC3339),
			},
		},
	}
	if NeedsImageQuotaRefresh(freshAccount, now) {
		t.Fatal("expected refresh to be skipped before reset_after")
	}

	expiredAccount := PublicAccount{
		Status: "限流",
		Quota:  0,
		LimitsProgress: []map[string]any{
			{
				"feature_name": "image_gen",
				"remaining":    0,
				"reset_after":  now.Add(-1 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	if !NeedsImageQuotaRefresh(expiredAccount, now) {
		t.Fatal("expected refresh to be required after reset_after")
	}
}

func TestImportAuthFilesSkipsDuplicateToken(t *testing.T) {
	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	store := &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states:       map[string]RuntimeState{},
	}

	existingToken := "token-1"
	if err := writeJSONFile(filepath.Join(authDir, "existing.json"), map[string]any{
		"type":         "codex",
		"access_token": existingToken,
		"email":        "existing@example.com",
	}); err != nil {
		t.Fatalf("seed existing auth file: %v", err)
	}

	imported, importedTokens, skipped, failures, err := store.ImportAuthFiles([]ImportedAuthFile{
		{
			Name: "duplicate.json",
			Data: []byte(`{"type":"codex","access_token":"token-1","email":"dup@example.com"}`),
		},
		{
			Name: "fresh.json",
			Data: []byte(`{"type":"codex","access_token":"token-2","email":"fresh@example.com"}`),
		},
	})
	if err != nil {
		t.Fatalf("ImportAuthFiles returned error: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected imported=1, got %d", imported)
	}
	if len(importedTokens) != 1 || importedTokens[0] != "token-2" {
		t.Fatalf("expected imported token token-2, got %#v", importedTokens)
	}
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped file, got %d", len(skipped))
	}
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %#v", failures)
	}

	auths, err := store.loadAuths()
	if err != nil {
		t.Fatalf("loadAuths returned error: %v", err)
	}
	if len(auths) != 2 {
		t.Fatalf("expected 2 auth files after import, got %d", len(auths))
	}
}

func TestImportAuthFilesKeepsNewerByIdentityAndType(t *testing.T) {
	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	store := &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states:       map[string]RuntimeState{},
	}

	existingName := "existing.json"
	existingData := map[string]any{
		"type":         "codex",
		"access_token": "token-old",
		"account_id":   "acct-1",
		"email":        "user@example.com",
		"last_refresh": "2026-04-20T01:00:00+08:00",
	}
	if err := writeJSONFile(filepath.Join(authDir, existingName), existingData); err != nil {
		t.Fatalf("seed existing auth file: %v", err)
	}

	imported, importedTokens, skipped, failures, err := store.ImportAuthFiles([]ImportedAuthFile{
		{
			Name: "newer.json",
			Data: []byte(`{"type":"codex","access_token":"token-new","account_id":"acct-1","email":"user@example.com","last_refresh":"2026-04-21T01:00:00+08:00"}`),
		},
	})
	if err != nil {
		t.Fatalf("ImportAuthFiles returned error: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected imported=1, got %d", imported)
	}
	if len(importedTokens) != 1 || importedTokens[0] != "token-new" {
		t.Fatalf("expected imported token token-new, got %#v", importedTokens)
	}
	if len(skipped) != 0 {
		t.Fatalf("expected no skipped files, got %#v", skipped)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %#v", failures)
	}

	var stored map[string]any
	if err := readJSONFile(filepath.Join(authDir, existingName), &stored); err != nil {
		t.Fatalf("read overwritten auth file: %v", err)
	}
	if got := stringValue(stored["access_token"]); got != "token-new" {
		t.Fatalf("expected overwritten access_token token-new, got %s", got)
	}
}

func TestBuildPublicAccountFallsBackProImageQuotaWhenLimitMissing(t *testing.T) {
	store := &Store{
		defaultQuota: 5,
		providerType: "codex",
		states: map[string]RuntimeState{
			"pro.json": {
				Type:           "Pro",
				Status:         "限流",
				Quota:          0,
				QuotaKnown:     true,
				LimitsProgress: nil,
			},
		},
	}

	got := store.buildPublicAccount(
		LocalAuth{
			Name:        "pro.json",
			AccessToken: "token-1",
			Provider:    "codex",
			Data:        map[string]any{},
		},
		SyncState{},
		nil,
	)

	if got.Type != "Pro" {
		t.Fatalf("expected type Pro, got %q", got.Type)
	}
	if got.Quota != proFallbackImageGenQuota {
		t.Fatalf("expected quota %d, got %d", proFallbackImageGenQuota, got.Quota)
	}
	if got.Status != "正常" {
		t.Fatalf("expected status 正常, got %q", got.Status)
	}

	hasImageGen := false
	for _, item := range got.LimitsProgress {
		if strings.TrimSpace(strings.ToLower(stringValue(item["feature_name"]))) != "image_gen" {
			continue
		}
		hasImageGen = true
		if gotRemaining := intValue(item["remaining"]); gotRemaining != proFallbackImageGenQuota {
			t.Fatalf("expected image_gen remaining %d, got %d", proFallbackImageGenQuota, gotRemaining)
		}
	}
	if !hasImageGen {
		t.Fatal("expected limits_progress to include image_gen")
	}
}

func TestAcquireImageAuthFilteredReturnsSentinelWhenNoEligibleAccount(t *testing.T) {
	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	store := &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states: map[string]RuntimeState{
			"free.json": {
				Type:       "Free",
				Status:     "正常",
				Quota:      1,
				QuotaKnown: true,
			},
		},
	}

	if err := writeJSONFile(filepath.Join(authDir, "free.json"), map[string]any{
		"type":         "codex",
		"access_token": "token-free",
		"email":        "free@example.com",
	}); err != nil {
		t.Fatalf("seed free auth file: %v", err)
	}

	_, _, err := store.AcquireImageAuthFiltered(nil, func(account PublicAccount) bool {
		return account.Type != "Free"
	})
	if !errors.Is(err, ErrNoAvailableImageAuth) {
		t.Fatalf("AcquireImageAuthFiltered() error = %v, want ErrNoAvailableImageAuth", err)
	}
}

func TestAcquireImageAuthFilteredAcceptsPaidAccountInferredFromAccessToken(t *testing.T) {
	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	store := &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states:       map[string]RuntimeState{},
	}

	token := mustTestJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type": "plus",
		},
	})

	if err := writeJSONFile(filepath.Join(authDir, "paid.json"), map[string]any{
		"type":         "codex",
		"access_token": token,
		"email":        "paid@example.com",
	}); err != nil {
		t.Fatalf("seed paid auth file: %v", err)
	}

	_, account, err := store.AcquireImageAuthFiltered(nil, func(account PublicAccount) bool {
		return account.Type == "Plus" || account.Type == "Pro" || account.Type == "Team"
	})
	if err != nil {
		t.Fatalf("AcquireImageAuthFiltered() returned error: %v", err)
	}
	if account.Type != "Plus" {
		t.Fatalf("AcquireImageAuthFiltered() type = %q, want %q", account.Type, "Plus")
	}
}

func TestAcquireImageAuthFilteredWithDisabledOptionAllowsDisabledAccount(t *testing.T) {
	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	store := &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states: map[string]RuntimeState{
			"disabled.json": {
				Type:       "Plus",
				Status:     "禁用",
				Quota:      3,
				QuotaKnown: true,
			},
		},
	}

	if err := writeJSONFile(filepath.Join(authDir, "disabled.json"), map[string]any{
		"type":         "codex",
		"access_token": "token-disabled",
		"email":        "disabled@example.com",
		"disabled":     true,
	}); err != nil {
		t.Fatalf("seed disabled auth file: %v", err)
	}

	if _, _, err := store.AcquireImageAuthFiltered(nil, nil); !errors.Is(err, ErrNoAvailableImageAuth) {
		t.Fatalf("AcquireImageAuthFiltered() error = %v, want ErrNoAvailableImageAuth", err)
	}

	_, account, err := store.AcquireImageAuthFilteredWithDisabledOption(nil, nil, true)
	if err != nil {
		t.Fatalf("AcquireImageAuthFilteredWithDisabledOption() returned error: %v", err)
	}
	if account.Email != "disabled@example.com" {
		t.Fatalf("AcquireImageAuthFilteredWithDisabledOption() email = %q, want %q", account.Email, "disabled@example.com")
	}
}

func TestAcquireImageAuthLeasePreventsConcurrentReuse(t *testing.T) {
	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	store := &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states: map[string]RuntimeState{
			"lease.json": {
				Type:       "Plus",
				Status:     "姝ｅ父",
				Quota:      3,
				QuotaKnown: true,
			},
		},
	}

	if err := writeJSONFile(filepath.Join(authDir, "lease.json"), map[string]any{
		"type":         "codex",
		"access_token": "token-lease",
		"email":        "lease@example.com",
	}); err != nil {
		t.Fatalf("seed lease auth file: %v", err)
	}

	_, firstAccount, release, err := store.AcquireImageAuthLeaseFilteredWithDisabledOption(nil, nil, false)
	if err != nil {
		t.Fatalf("first AcquireImageAuthLeaseFilteredWithDisabledOption() error: %v", err)
	}
	if firstAccount.Email != "lease@example.com" {
		t.Fatalf("first account email = %q, want lease@example.com", firstAccount.Email)
	}

	_, _, _, err = store.AcquireImageAuthLeaseFilteredWithDisabledOption(nil, nil, false)
	if !errors.Is(err, ErrNoAvailableImageAuth) {
		t.Fatalf("second AcquireImageAuthLeaseFilteredWithDisabledOption() error = %v, want ErrNoAvailableImageAuth", err)
	}

	release()

	_, secondAccount, releaseAgain, err := store.AcquireImageAuthLeaseFilteredWithDisabledOption(nil, nil, false)
	if err != nil {
		t.Fatalf("third AcquireImageAuthLeaseFilteredWithDisabledOption() error: %v", err)
	}
	if secondAccount.Email != "lease@example.com" {
		t.Fatalf("second account email = %q, want lease@example.com", secondAccount.Email)
	}
	releaseAgain()
}

func TestFindImageAuthByIDWithLeaseReturnsInUse(t *testing.T) {
	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	store := &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states: map[string]RuntimeState{
			"source.json": {
				Type:       "Plus",
				Status:     "姝ｅ父",
				Quota:      5,
				QuotaKnown: true,
			},
		},
	}

	if err := writeJSONFile(filepath.Join(authDir, "source.json"), map[string]any{
		"type":         "codex",
		"access_token": "token-source",
		"email":        "source@example.com",
	}); err != nil {
		t.Fatalf("seed source auth file: %v", err)
	}

	accounts, err := store.ListAccounts()
	if err != nil {
		t.Fatalf("ListAccounts() error: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("ListAccounts() count = %d, want 1", len(accounts))
	}
	accountID := accounts[0].ID

	_, _, release, err := store.FindImageAuthByIDWithLease(accountID)
	if err != nil {
		t.Fatalf("FindImageAuthByIDWithLease() first error: %v", err)
	}

	_, _, _, err = store.FindImageAuthByIDWithLease(accountID)
	if !errors.Is(err, ErrImageAuthInUse) {
		t.Fatalf("FindImageAuthByIDWithLease() second error = %v, want ErrImageAuthInUse", err)
	}

	release()
}

func TestNeedsImageQuotaRefreshWithTTL(t *testing.T) {
	now := time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC)
	ttl := 120 * time.Second

	fresh := PublicAccount{
		Quota:           3,
		LastRefreshedAt: now.Add(-30 * time.Second).Format(time.RFC3339),
		LimitsProgress: []map[string]any{
			{
				"feature_name": "image_gen",
				"remaining":    3,
				"reset_after":  now.Add(30 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	if NeedsImageQuotaRefreshWithTTL(fresh, now, ttl) {
		t.Fatal("expected refresh to be skipped within ttl window")
	}

	stale := fresh
	stale.LastRefreshedAt = now.Add(-5 * time.Minute).Format(time.RFC3339)
	if !NeedsImageQuotaRefreshWithTTL(stale, now, ttl) {
		t.Fatal("expected refresh when ttl expires")
	}

	resetExpired := fresh
	resetExpired.LastRefreshedAt = now.Add(-30 * time.Second).Format(time.RFC3339)
	resetExpired.LimitsProgress = []map[string]any{
		{
			"feature_name": "image_gen",
			"remaining":    0,
			"reset_after":  now.Add(-1 * time.Minute).Format(time.RFC3339),
		},
	}
	if !NeedsImageQuotaRefreshWithTTL(resetExpired, now, ttl) {
		t.Fatal("expected refresh when reset_after has passed")
	}

	noRefreshRecord := fresh
	noRefreshRecord.LastRefreshedAt = ""
	if !NeedsImageQuotaRefreshWithTTL(noRefreshRecord, now, ttl) {
		t.Fatal("expected refresh when no last_refreshed_at is recorded")
	}
}

func mustTestJWT(t *testing.T, payload map[string]any) string {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(raw) + ".signature"
}
