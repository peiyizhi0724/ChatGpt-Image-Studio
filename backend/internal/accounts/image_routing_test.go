package accounts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	testStatusNormal = "\u6b63\u5e38"
)

func TestSyncImageQuotaDailyBaseStateKeepsBaseWithinResetWindow(t *testing.T) {
	state := RuntimeState{
		ImageQuotaDailyBase:        10,
		ImageQuotaDailyBaseResetAt: "2026-04-26T10:00:00Z",
	}

	next := syncImageQuotaDailyBaseState(state, 8, []map[string]any{
		{
			"feature_name": "image_gen",
			"remaining":    6,
			"reset_after":  "2026-04-26T10:00:00Z",
		},
	}, "2026-04-26T10:00:00Z")

	if next.ImageQuotaDailyBase != 10 {
		t.Fatalf("ImageQuotaDailyBase = %d, want 10", next.ImageQuotaDailyBase)
	}
	if next.ImageQuotaDailyBaseResetAt != "2026-04-26T10:00:00Z" {
		t.Fatalf("ImageQuotaDailyBaseResetAt = %q, want same window", next.ImageQuotaDailyBaseResetAt)
	}
}

func TestSyncImageQuotaDailyBaseStateResetsOnNewWindow(t *testing.T) {
	state := RuntimeState{
		ImageQuotaDailyBase:        10,
		ImageQuotaDailyBaseResetAt: "2026-04-26T10:00:00Z",
	}

	next := syncImageQuotaDailyBaseState(state, 8, []map[string]any{
		{
			"feature_name": "image_gen",
			"remaining":    7,
			"reset_after":  "2026-04-27T10:00:00Z",
		},
	}, "2026-04-27T10:00:00Z")

	if next.ImageQuotaDailyBase != 7 {
		t.Fatalf("ImageQuotaDailyBase = %d, want 7", next.ImageQuotaDailyBase)
	}
	if next.ImageQuotaDailyBaseResetAt != "2026-04-27T10:00:00Z" {
		t.Fatalf("ImageQuotaDailyBaseResetAt = %q, want new window", next.ImageQuotaDailyBaseResetAt)
	}
}

func TestAcquireImageAuthLeaseWithPolicyUsesImportedAtGrouping(t *testing.T) {
	store := newImageRoutingTestStore(t)
	baseTime := time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC)

	for index := 0; index < 12; index++ {
		name := fmt.Sprintf("acct-%02d.json", index+1)
		token := fmt.Sprintf("token-%02d", index+1)
		email := fmt.Sprintf("acct-%02d@example.com", index+1)
		createdAt := baseTime.Add(time.Duration(index) * time.Minute)
		priority := 1
		if index == 10 {
			priority = 20
		}

		seedImageRoutingAccount(t, store, name, token, email, createdAt, RuntimeState{
			Type:       "Free",
			Status:     testStatusNormal,
			Quota:      5,
			QuotaKnown: true,
			LimitsProgress: []map[string]any{
				{
					"feature_name": "image_gen",
					"remaining":    5,
					"reset_after":  baseTime.Add(24 * time.Hour).Format(time.RFC3339),
				},
			},
		})
		store.states[name] = RuntimeState{
			Type:                       "Free",
			Status:                     testStatusNormal,
			Quota:                      5,
			QuotaKnown:                 true,
			LimitsProgress:             []map[string]any{{"feature_name": "image_gen", "remaining": 5, "reset_after": baseTime.Add(24 * time.Hour).Format(time.RFC3339)}},
			ImageQuotaDailyBase:        5,
			ImageQuotaDailyBaseResetAt: baseTime.Add(24 * time.Hour).Format(time.RFC3339),
		}
		if priority > 1 {
			auth, err := store.findAuthByName(name)
			if err != nil {
				t.Fatalf("findAuthByName(%q): %v", name, err)
			}
			auth.Data["priority"] = priority
			if err := writeJSONFile(auth.Path, auth.Data); err != nil {
				t.Fatalf("write priority seed: %v", err)
			}
		}
	}

	auth, account, decision, release, err := store.AcquireImageAuthLeaseWithPolicyFilteredWithDisabledOption(
		nil,
		nil,
		false,
		&ImageAccountRoutingPolicy{
			Enabled:             true,
			SortMode:            "imported_at",
			GroupSize:           10,
			EnabledGroupIndexes: []int{1},
			ReserveMode:         "daily_first_seen_percent",
			ReservePercent:      20,
		},
	)
	if err != nil {
		t.Fatalf("AcquireImageAuthLeaseWithPolicyFilteredWithDisabledOption() error: %v", err)
	}
	defer release()

	if decision.GroupIndex != 1 {
		t.Fatalf("GroupIndex = %d, want 1", decision.GroupIndex)
	}
	if auth.Name != "acct-11.json" {
		t.Fatalf("selected auth = %q, want acct-11.json", auth.Name)
	}
	if account.Email != "acct-11@example.com" {
		t.Fatalf("selected email = %q, want acct-11@example.com", account.Email)
	}
}

func TestAcquireImageAuthLeaseWithPolicyRollsToNextSelectedGroupOnReserve(t *testing.T) {
	store := newImageRoutingTestStore(t)
	baseTime := time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC)
	resetAt := baseTime.Add(24 * time.Hour).Format(time.RFC3339)

	for index := 0; index < 12; index++ {
		name := fmt.Sprintf("acct-%02d.json", index+1)
		token := fmt.Sprintf("token-%02d", index+1)
		email := fmt.Sprintf("acct-%02d@example.com", index+1)
		createdAt := baseTime.Add(time.Duration(index) * time.Minute)

		remaining := 1
		if index >= 10 {
			remaining = 4
		}

		seedImageRoutingAccount(t, store, name, token, email, createdAt, RuntimeState{
			Type:                       "Free",
			Status:                     testStatusNormal,
			Quota:                      remaining,
			QuotaKnown:                 true,
			LimitsProgress:             []map[string]any{{"feature_name": "image_gen", "remaining": remaining, "reset_after": resetAt}},
			ImageQuotaDailyBase:        5,
			ImageQuotaDailyBaseResetAt: resetAt,
		})
	}

	auth, _, decision, release, err := store.AcquireImageAuthLeaseWithPolicyFilteredWithDisabledOption(
		nil,
		nil,
		false,
		&ImageAccountRoutingPolicy{
			Enabled:             true,
			SortMode:            "imported_at",
			GroupSize:           10,
			EnabledGroupIndexes: []int{0, 1},
			ReserveMode:         "daily_first_seen_percent",
			ReservePercent:      20,
		},
	)
	if err != nil {
		t.Fatalf("AcquireImageAuthLeaseWithPolicyFilteredWithDisabledOption() error: %v", err)
	}
	defer release()

	if decision.GroupIndex != 1 {
		t.Fatalf("GroupIndex = %d, want 1", decision.GroupIndex)
	}
	if auth.Name != "acct-11.json" {
		t.Fatalf("selected auth = %q, want acct-11.json", auth.Name)
	}
}

func TestAcquireImageAuthLeaseWithPolicyReturnsExhaustedForSelectedGroups(t *testing.T) {
	store := newImageRoutingTestStore(t)
	baseTime := time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC)
	resetAt := baseTime.Add(24 * time.Hour).Format(time.RFC3339)

	for index := 0; index < 12; index++ {
		name := fmt.Sprintf("acct-%02d.json", index+1)
		token := fmt.Sprintf("token-%02d", index+1)
		email := fmt.Sprintf("acct-%02d@example.com", index+1)
		createdAt := baseTime.Add(time.Duration(index) * time.Minute)

		remaining := 1
		if index >= 10 {
			remaining = 4
		}

		seedImageRoutingAccount(t, store, name, token, email, createdAt, RuntimeState{
			Type:                       "Free",
			Status:                     testStatusNormal,
			Quota:                      remaining,
			QuotaKnown:                 true,
			LimitsProgress:             []map[string]any{{"feature_name": "image_gen", "remaining": remaining, "reset_after": resetAt}},
			ImageQuotaDailyBase:        5,
			ImageQuotaDailyBaseResetAt: resetAt,
		})
	}

	_, _, _, _, err := store.AcquireImageAuthLeaseWithPolicyFilteredWithDisabledOption(
		nil,
		nil,
		false,
		&ImageAccountRoutingPolicy{
			Enabled:             true,
			SortMode:            "imported_at",
			GroupSize:           10,
			EnabledGroupIndexes: []int{0},
			ReserveMode:         "daily_first_seen_percent",
			ReservePercent:      20,
		},
	)
	if !errors.Is(err, ErrSelectedImageGroupsExhausted) {
		t.Fatalf("error = %v, want ErrSelectedImageGroupsExhausted", err)
	}
}

func newImageRoutingTestStore(t *testing.T) *Store {
	t.Helper()

	rootDir := t.TempDir()
	authDir := filepath.Join(rootDir, "auths")
	syncDir := filepath.Join(rootDir, "sync")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatalf("mkdir sync dir: %v", err)
	}

	return &Store{
		authDir:      authDir,
		syncStateDir: syncDir,
		stateFile:    filepath.Join(rootDir, "state.json"),
		defaultQuota: 5,
		providerType: "codex",
		states:       map[string]RuntimeState{},
		imageLeases:  map[string]int{},
	}
}

func seedImageRoutingAccount(
	t *testing.T,
	store *Store,
	name string,
	token string,
	email string,
	createdAt time.Time,
	state RuntimeState,
) {
	t.Helper()

	authData := map[string]any{
		"type":         "codex",
		"access_token": token,
		"email":        email,
		"created_at":   createdAt.Format(time.RFC3339),
	}
	if err := writeJSONFile(filepath.Join(store.authDir, name), authData); err != nil {
		t.Fatalf("write auth %q: %v", name, err)
	}
	store.states[name] = state
}
