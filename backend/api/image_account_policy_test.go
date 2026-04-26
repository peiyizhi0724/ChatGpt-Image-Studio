package api

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestParseRequestImageAccountRoutingPolicy(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/images/generations", nil)
	req.Header.Set(imageAccountPolicyHeader, base64.RawURLEncoding.EncodeToString([]byte(`{
		"enabled": true,
		"sortMode": "quota",
		"groupSize": 8,
		"enabledGroupIndexes": [1, 0, 1],
		"reserveMode": "daily_first_seen_percent",
		"reservePercent": 25
	}`)))

	policy, err := parseRequestImageAccountRoutingPolicy(req)
	if err != nil {
		t.Fatalf("parseRequestImageAccountRoutingPolicy() error: %v", err)
	}
	if policy == nil {
		t.Fatal("parseRequestImageAccountRoutingPolicy() returned nil policy")
	}
	if policy.SortMode != "quota" {
		t.Fatalf("SortMode = %q, want quota", policy.SortMode)
	}
	if policy.GroupSize != 8 {
		t.Fatalf("GroupSize = %d, want 8", policy.GroupSize)
	}
	if len(policy.EnabledGroupIndexes) != 2 || policy.EnabledGroupIndexes[0] != 0 || policy.EnabledGroupIndexes[1] != 1 {
		t.Fatalf("EnabledGroupIndexes = %#v, want [0 1]", policy.EnabledGroupIndexes)
	}
}

func TestParseRequestImageAccountRoutingPolicyRejectsInvalidPayload(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/images/generations", nil)
	req.Header.Set(imageAccountPolicyHeader, "not-base64")

	_, err := parseRequestImageAccountRoutingPolicy(req)
	if err == nil {
		t.Fatal("parseRequestImageAccountRoutingPolicy() error = nil, want invalid_account_policy")
	}
	if requestErrorCode(err) != "invalid_account_policy" {
		t.Fatalf("requestErrorCode = %q, want invalid_account_policy", requestErrorCode(err))
	}
}
