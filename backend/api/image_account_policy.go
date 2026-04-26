package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"chatgpt2api/internal/accounts"
)

const imageAccountPolicyHeader = "X-Studio-Account-Policy"

func parseRequestImageAccountRoutingPolicy(r *http.Request) (*accounts.ImageAccountRoutingPolicy, error) {
	if r == nil {
		return nil, nil
	}
	raw := strings.TrimSpace(r.Header.Get(imageAccountPolicyHeader))
	if raw == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, newRequestError("invalid_account_policy", "invalid image account routing policy header")
	}

	var policy accounts.ImageAccountRoutingPolicy
	if err := json.Unmarshal(decoded, &policy); err != nil {
		return nil, newRequestError("invalid_account_policy", "invalid image account routing policy payload")
	}

	normalized := policy.Normalize()
	if !normalized.Enabled {
		return nil, nil
	}
	return &normalized, nil
}

func applyImageRoutingLogFields(decision accounts.ImageAccountRoutingDecision, entry *imageRequestLogEntry) {
	if entry == nil || !decision.PolicyApplied {
		return
	}
	entry.RoutingPolicyApplied = true
	entry.RoutingGroupIndex = decision.GroupIndex
	entry.RoutingSortMode = decision.SortMode
	entry.RoutingReservePercent = decision.ReservePercent
}
