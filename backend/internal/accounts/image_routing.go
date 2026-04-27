package accounts

import (
	"math"
	"sort"
	"strings"
	"time"
)

type ImageAccountRoutingPolicy struct {
	Enabled             bool   `json:"enabled"`
	SortMode            string `json:"sortMode"`
	GroupSize           int    `json:"groupSize"`
	EnabledGroupIndexes []int  `json:"enabledGroupIndexes"`
	ReserveMode         string `json:"reserveMode"`
	ReservePercent      int    `json:"reservePercent"`
}

type ImageAccountRoutingDecision struct {
	PolicyApplied  bool
	GroupIndex     int
	SortMode       string
	ReservePercent int
}

func defaultImageAccountRoutingPolicy() ImageAccountRoutingPolicy {
	return ImageAccountRoutingPolicy{
		Enabled:             false,
		SortMode:            "imported_at",
		GroupSize:           10,
		EnabledGroupIndexes: []int{0, 1},
		ReserveMode:         "daily_first_seen_percent",
		ReservePercent:      20,
	}
}

type imageRoutingCandidate struct {
	auth    LocalAuth
	account PublicAccount
	ready   bool
}

func (p ImageAccountRoutingPolicy) Normalize() ImageAccountRoutingPolicy {
	next := p
	switch strings.ToLower(strings.TrimSpace(next.SortMode)) {
	case "name":
		next.SortMode = "name"
	case "quota":
		next.SortMode = "quota"
	default:
		next.SortMode = "imported_at"
	}
	if next.GroupSize <= 0 {
		next.GroupSize = 10
	}
	if next.ReservePercent < 0 {
		next.ReservePercent = 0
	}
	if next.ReservePercent > 100 {
		next.ReservePercent = 100
	}
	if strings.TrimSpace(next.ReserveMode) == "" {
		next.ReserveMode = "daily_first_seen_percent"
	}

	groupSet := make(map[int]struct{}, len(next.EnabledGroupIndexes))
	groupIndexes := make([]int, 0, len(next.EnabledGroupIndexes))
	for _, groupIndex := range next.EnabledGroupIndexes {
		if groupIndex < 0 {
			continue
		}
		if _, exists := groupSet[groupIndex]; exists {
			continue
		}
		groupSet[groupIndex] = struct{}{}
		groupIndexes = append(groupIndexes, groupIndex)
	}
	sort.Ints(groupIndexes)
	next.EnabledGroupIndexes = groupIndexes
	return next
}

func (s *Store) AcquireImageAuthLeaseWithPolicyFilteredWithDisabledOption(
	excluded map[string]struct{},
	allow func(PublicAccount) bool,
	allowDisabled bool,
	policy *ImageAccountRoutingPolicy,
) (*LocalAuth, PublicAccount, ImageAccountRoutingDecision, func(), error) {
	return s.acquireImageAuthWithPolicyLease(excluded, allow, allowDisabled, policy)
}

func (s *Store) ImageAccountAllowedForPolicy(accessToken string, account PublicAccount, policy *ImageAccountRoutingPolicy) bool {
	if s == nil || policy == nil || !policy.Enabled {
		return true
	}
	auth, err := s.findAuthByToken(accessToken)
	if err != nil {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accountAboveReserveLocked(auth.Name, account, policy.Normalize())
}

func (s *Store) CountAvailableImageAuthLeaseCandidatesWithPolicyFilteredWithDisabledOption(
	allow func(PublicAccount) bool,
	allowDisabled bool,
	policy *ImageAccountRoutingPolicy,
) (int, error) {
	if policy == nil || !policy.Enabled {
		return s.countAvailableImageAuthLeaseCandidates(allow, allowDisabled)
	}

	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, err
	}
	syncStates := s.loadAllSyncStates()
	now := time.Now()
	normalizedPolicy := policy.Normalize()

	s.mu.Lock()
	defer s.mu.Unlock()

	allAccounts := make([]imageRoutingCandidate, 0, len(localAuths))
	for _, auth := range localAuths {
		if auth.AccessToken == "" || !s.matchesProvider(auth.Provider) {
			continue
		}
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		allAccounts = append(allAccounts, imageRoutingCandidate{
			auth:    auth,
			account: account,
			ready:   isUsableImageAccount(account, allowDisabled),
		})
	}
	if len(allAccounts) == 0 {
		return 0, nil
	}

	sortRoutingCandidates(allAccounts, normalizedPolicy.SortMode)

	groupCount := 0
	if normalizedPolicy.GroupSize > 0 {
		groupCount = int(math.Ceil(float64(len(allAccounts)) / float64(normalizedPolicy.GroupSize)))
	}

	selectedGroupIndexes := make([]int, 0, len(normalizedPolicy.EnabledGroupIndexes))
	for _, groupIndex := range normalizedPolicy.EnabledGroupIndexes {
		if groupIndex < 0 || groupIndex >= groupCount {
			continue
		}
		selectedGroupIndexes = append(selectedGroupIndexes, groupIndex)
	}
	if len(selectedGroupIndexes) == 0 && len(normalizedPolicy.EnabledGroupIndexes) > 0 && groupCount > 0 {
		selectedGroupIndexes = make([]int, groupCount)
		for index := range selectedGroupIndexes {
			selectedGroupIndexes[index] = index
		}
	}

	count := s.countImageRoutingCandidatesFromGroups(
		allAccounts,
		selectedGroupIndexes,
		normalizedPolicy,
		allow,
		allowDisabled,
		now,
	)
	if count > 0 {
		return count, nil
	}

	if allow != nil && len(selectedGroupIndexes) > 0 {
		allGroupIndexes := make([]int, groupCount)
		for index := range allGroupIndexes {
			allGroupIndexes[index] = index
		}
		return s.countImageRoutingCandidatesFromGroups(
			allAccounts,
			allGroupIndexes,
			normalizedPolicy,
			allow,
			allowDisabled,
			now,
		), nil
	}

	return 0, nil
}

func (s *Store) CountPotentialImageAuthCandidatesWithPolicyFilteredWithDisabledOption(
	allow func(PublicAccount) bool,
	allowDisabled bool,
	policy *ImageAccountRoutingPolicy,
) (int, error) {
	if policy == nil || !policy.Enabled {
		return s.countPotentialImageAuthCandidates(allow, allowDisabled)
	}

	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, err
	}
	syncStates := s.loadAllSyncStates()
	now := time.Now()
	normalizedPolicy := policy.Normalize()

	s.mu.Lock()
	defer s.mu.Unlock()

	allAccounts := make([]imageRoutingCandidate, 0, len(localAuths))
	for _, auth := range localAuths {
		if auth.AccessToken == "" || !s.matchesProvider(auth.Provider) {
			continue
		}
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		allAccounts = append(allAccounts, imageRoutingCandidate{
			auth:    auth,
			account: account,
			ready:   isUsableImageAccount(account, allowDisabled),
		})
	}
	if len(allAccounts) == 0 {
		return 0, nil
	}

	sortRoutingCandidates(allAccounts, normalizedPolicy.SortMode)

	groupCount := 0
	if normalizedPolicy.GroupSize > 0 {
		groupCount = int(math.Ceil(float64(len(allAccounts)) / float64(normalizedPolicy.GroupSize)))
	}

	selectedGroupIndexes := make([]int, 0, len(normalizedPolicy.EnabledGroupIndexes))
	for _, groupIndex := range normalizedPolicy.EnabledGroupIndexes {
		if groupIndex < 0 || groupIndex >= groupCount {
			continue
		}
		selectedGroupIndexes = append(selectedGroupIndexes, groupIndex)
	}
	if len(selectedGroupIndexes) == 0 && len(normalizedPolicy.EnabledGroupIndexes) > 0 && groupCount > 0 {
		selectedGroupIndexes = make([]int, groupCount)
		for index := range selectedGroupIndexes {
			selectedGroupIndexes[index] = index
		}
	}
	if len(selectedGroupIndexes) == 0 && len(normalizedPolicy.EnabledGroupIndexes) == 0 && groupCount > 0 {
		selectedGroupIndexes = make([]int, groupCount)
		for index := range selectedGroupIndexes {
			selectedGroupIndexes[index] = index
		}
	}

	count := s.countPotentialImageRoutingCandidatesFromGroups(
		allAccounts,
		selectedGroupIndexes,
		normalizedPolicy,
		allow,
		allowDisabled,
		now,
	)
	if count > 0 {
		return count, nil
	}

	if allow != nil && len(selectedGroupIndexes) > 0 {
		allGroupIndexes := make([]int, groupCount)
		for index := range allGroupIndexes {
			allGroupIndexes[index] = index
		}
		return s.countPotentialImageRoutingCandidatesFromGroups(
			allAccounts,
			allGroupIndexes,
			normalizedPolicy,
			allow,
			allowDisabled,
			now,
		), nil
	}

	return 0, nil
}

func (s *Store) acquireImageAuthWithPolicyLease(
	excluded map[string]struct{},
	allow func(PublicAccount) bool,
	allowDisabled bool,
	policy *ImageAccountRoutingPolicy,
) (*LocalAuth, PublicAccount, ImageAccountRoutingDecision, func(), error) {
	if policy == nil || !policy.Enabled {
		auth, account, release, err := s.AcquireImageAuthLeaseFilteredWithDisabledOption(excluded, allow, allowDisabled)
		return auth, account, ImageAccountRoutingDecision{}, release, err
	}

	localAuths, err := s.loadAuths()
	if err != nil {
		return nil, PublicAccount{}, ImageAccountRoutingDecision{}, nil, err
	}
	syncStates := s.loadAllSyncStates()
	now := time.Now()
	normalizedPolicy := policy.Normalize()

	s.mu.Lock()
	defer s.mu.Unlock()

	allAccounts := make([]imageRoutingCandidate, 0, len(localAuths))
	for _, auth := range localAuths {
		if auth.AccessToken == "" || !s.matchesProvider(auth.Provider) {
			continue
		}
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		allAccounts = append(allAccounts, imageRoutingCandidate{
			auth:    auth,
			account: account,
			ready:   isUsableImageAccount(account, allowDisabled),
		})
	}
	if len(allAccounts) == 0 {
		return nil, PublicAccount{}, ImageAccountRoutingDecision{}, nil, ErrNoAvailableImageAuth
	}

	sortRoutingCandidates(allAccounts, normalizedPolicy.SortMode)

	groupCount := 0
	if normalizedPolicy.GroupSize > 0 {
		groupCount = int(math.Ceil(float64(len(allAccounts)) / float64(normalizedPolicy.GroupSize)))
	}

	selectedGroupIndexes := make([]int, 0, len(normalizedPolicy.EnabledGroupIndexes))
	for _, groupIndex := range normalizedPolicy.EnabledGroupIndexes {
		if groupIndex < 0 || groupIndex >= groupCount {
			continue
		}
		selectedGroupIndexes = append(selectedGroupIndexes, groupIndex)
	}
	if len(selectedGroupIndexes) == 0 && len(normalizedPolicy.EnabledGroupIndexes) > 0 && groupCount > 0 {
		selectedGroupIndexes = make([]int, groupCount)
		for index := range selectedGroupIndexes {
			selectedGroupIndexes[index] = index
		}
	}

	auth, account, decision, release, ok := s.selectImageRoutingCandidateFromGroups(
		allAccounts,
		selectedGroupIndexes,
		normalizedPolicy,
		excluded,
		allow,
		allowDisabled,
		now,
	)
	if ok {
		return auth, account, decision, release, nil
	}

	if allow != nil && len(selectedGroupIndexes) > 0 {
		allGroupIndexes := make([]int, groupCount)
		for index := range allGroupIndexes {
			allGroupIndexes[index] = index
		}
		auth, account, _, release, ok = s.selectImageRoutingCandidateFromGroups(
			allAccounts,
			allGroupIndexes,
			normalizedPolicy,
			excluded,
			allow,
			allowDisabled,
			now,
		)
		if ok {
			return auth, account, ImageAccountRoutingDecision{}, release, nil
		}
	}

	if len(selectedGroupIndexes) > 0 || len(normalizedPolicy.EnabledGroupIndexes) > 0 {
		return nil, PublicAccount{}, ImageAccountRoutingDecision{}, nil, ErrSelectedImageGroupsExhausted
	}
	return nil, PublicAccount{}, ImageAccountRoutingDecision{}, nil, ErrNoAvailableImageAuth
}

func (s *Store) selectImageRoutingCandidateFromGroups(
	allAccounts []imageRoutingCandidate,
	groupIndexes []int,
	normalizedPolicy ImageAccountRoutingPolicy,
	excluded map[string]struct{},
	allow func(PublicAccount) bool,
	allowDisabled bool,
	now time.Time,
) (*LocalAuth, PublicAccount, ImageAccountRoutingDecision, func(), bool) {
	sawSelectedGroup := false
	for _, groupIndex := range groupIndexes {
		groupStart := groupIndex * normalizedPolicy.GroupSize
		if groupStart >= len(allAccounts) {
			continue
		}
		sawSelectedGroup = true

		groupEnd := minInt(len(allAccounts), groupStart+normalizedPolicy.GroupSize)
		groupCandidates := make([]imageRoutingCandidate, 0, groupEnd-groupStart)
		for _, candidate := range allAccounts[groupStart:groupEnd] {
			if allow != nil && !allow(candidate.account) {
				continue
			}
			if _, blocked := excluded[candidate.auth.AccessToken]; blocked {
				continue
			}
			if s.isImageLeasedLocked(candidate.auth.AccessToken) {
				continue
			}
			refreshNeeded := NeedsImageQuotaRefreshWithTTL(candidate.account, now, s.cfg.ImageQuotaRefreshTTL())
			if candidate.auth.Disabled && !allowDisabled {
				continue
			}
			if candidate.account.Status == "禁用" && !allowDisabled {
				continue
			}
			if candidate.account.Status == "异常" {
				continue
			}
			if !candidate.ready && !refreshNeeded {
				continue
			}
			if candidate.ready && !s.accountAboveReserveLocked(candidate.auth.Name, candidate.account, normalizedPolicy) {
				continue
			}
			groupCandidates = append(groupCandidates, candidate)
		}

		if len(groupCandidates) == 0 {
			continue
		}

		sort.Slice(groupCandidates, func(i, j int) bool {
			if groupCandidates[i].ready != groupCandidates[j].ready {
				return groupCandidates[i].ready
			}
			if groupCandidates[i].account.Priority != groupCandidates[j].account.Priority {
				return groupCandidates[i].account.Priority > groupCandidates[j].account.Priority
			}
			if groupCandidates[i].account.Fail != groupCandidates[j].account.Fail {
				return groupCandidates[i].account.Fail < groupCandidates[j].account.Fail
			}
			return groupCandidates[i].account.LastUsedAt < groupCandidates[j].account.LastUsedAt
		})

		selected := groupCandidates[0]
		release, leaseErr := s.acquireImageLeaseLocked(selected.auth.AccessToken)
		if leaseErr != nil {
			continue
		}
		return &selected.auth, selected.account, ImageAccountRoutingDecision{
			PolicyApplied:  true,
			GroupIndex:     groupIndex,
			SortMode:       normalizedPolicy.SortMode,
			ReservePercent: normalizedPolicy.ReservePercent,
		}, release, true
	}

	_ = sawSelectedGroup
	return nil, PublicAccount{}, ImageAccountRoutingDecision{}, nil, false
}

func (s *Store) countAvailableImageAuthLeaseCandidates(
	allow func(PublicAccount) bool,
	allowDisabled bool,
) (int, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, err
	}
	syncStates := s.loadAllSyncStates()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, auth := range localAuths {
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
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
		count++
	}
	return count, nil
}

func (s *Store) countPotentialImageAuthCandidates(
	allow func(PublicAccount) bool,
	allowDisabled bool,
) (int, error) {
	localAuths, err := s.loadAuths()
	if err != nil {
		return 0, err
	}
	syncStates := s.loadAllSyncStates()
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, auth := range localAuths {
		account := s.buildPublicAccount(auth, syncStates[auth.Name], nil)
		if allow != nil && !allow(account) {
			continue
		}
		ready := isUsableImageAccount(account, allowDisabled)
		refreshNeeded := NeedsImageQuotaRefreshWithTTL(account, now, s.cfg.ImageQuotaRefreshTTL())
		if auth.AccessToken == "" ||
			(auth.Disabled && !allowDisabled) ||
			(account.Status == "禁用" && !allowDisabled) ||
			account.Status == "异常" ||
			(!ready && !refreshNeeded) {
			continue
		}
		count++
	}
	return count, nil
}

func (s *Store) countImageRoutingCandidatesFromGroups(
	allAccounts []imageRoutingCandidate,
	groupIndexes []int,
	normalizedPolicy ImageAccountRoutingPolicy,
	allow func(PublicAccount) bool,
	allowDisabled bool,
	now time.Time,
) int {
	count := 0
	for _, groupIndex := range groupIndexes {
		groupStart := groupIndex * normalizedPolicy.GroupSize
		if groupStart >= len(allAccounts) {
			continue
		}

		groupEnd := minInt(len(allAccounts), groupStart+normalizedPolicy.GroupSize)
		for _, candidate := range allAccounts[groupStart:groupEnd] {
			if allow != nil && !allow(candidate.account) {
				continue
			}
			if s.isImageLeasedLocked(candidate.auth.AccessToken) {
				continue
			}
			refreshNeeded := NeedsImageQuotaRefreshWithTTL(candidate.account, now, s.cfg.ImageQuotaRefreshTTL())
			if candidate.auth.Disabled && !allowDisabled {
				continue
			}
			if candidate.account.Status == "禁用" && !allowDisabled {
				continue
			}
			if candidate.account.Status == "异常" {
				continue
			}
			if !candidate.ready && !refreshNeeded {
				continue
			}
			if candidate.ready && !s.accountAboveReserveLocked(candidate.auth.Name, candidate.account, normalizedPolicy) {
				continue
			}
			count++
		}
	}
	return count
}

func (s *Store) countPotentialImageRoutingCandidatesFromGroups(
	allAccounts []imageRoutingCandidate,
	groupIndexes []int,
	normalizedPolicy ImageAccountRoutingPolicy,
	allow func(PublicAccount) bool,
	allowDisabled bool,
	now time.Time,
) int {
	count := 0
	for _, groupIndex := range groupIndexes {
		groupStart := groupIndex * normalizedPolicy.GroupSize
		if groupStart >= len(allAccounts) {
			continue
		}

		groupEnd := minInt(len(allAccounts), groupStart+normalizedPolicy.GroupSize)
		for _, candidate := range allAccounts[groupStart:groupEnd] {
			if allow != nil && !allow(candidate.account) {
				continue
			}
			refreshNeeded := NeedsImageQuotaRefreshWithTTL(candidate.account, now, s.cfg.ImageQuotaRefreshTTL())
			if candidate.auth.Disabled && !allowDisabled {
				continue
			}
			if candidate.account.Status == "禁用" && !allowDisabled {
				continue
			}
			if candidate.account.Status == "异常" {
				continue
			}
			if !candidate.ready && !refreshNeeded {
				continue
			}
			if candidate.ready && !s.accountAboveReserveLocked(candidate.auth.Name, candidate.account, normalizedPolicy) {
				continue
			}
			count++
		}
	}
	return count
}

func sortRoutingCandidates(candidates []imageRoutingCandidate, sortMode string) {
	sort.Slice(candidates, func(i, j int) bool {
		switch sortMode {
		case "name":
			left := strings.ToLower(firstNonEmpty(candidates[i].account.Email, candidates[i].account.FileName))
			right := strings.ToLower(firstNonEmpty(candidates[j].account.Email, candidates[j].account.FileName))
			if left != right {
				return left < right
			}
		case "quota":
			leftQuota := currentImageRemaining(candidates[i].account)
			rightQuota := currentImageRemaining(candidates[j].account)
			if leftQuota != rightQuota {
				return leftQuota > rightQuota
			}
		default:
			leftImportedAt, leftOK := parseFlexibleTime(candidates[i].account.ImportedAt)
			rightImportedAt, rightOK := parseFlexibleTime(candidates[j].account.ImportedAt)
			if leftOK && rightOK && !leftImportedAt.Equal(rightImportedAt) {
				return leftImportedAt.Before(rightImportedAt)
			}
			if leftOK != rightOK {
				return leftOK
			}
		}

		left := strings.ToLower(firstNonEmpty(candidates[i].account.Email, candidates[i].account.FileName))
		right := strings.ToLower(firstNonEmpty(candidates[j].account.Email, candidates[j].account.FileName))
		if left != right {
			return left < right
		}
		return candidates[i].auth.Name < candidates[j].auth.Name
	})
}

func (s *Store) accountAboveReserveLocked(authName string, account PublicAccount, policy ImageAccountRoutingPolicy) bool {
	if !policy.Enabled || strings.TrimSpace(policy.ReserveMode) != "daily_first_seen_percent" {
		return true
	}
	remaining := currentImageRemaining(account)
	if remaining <= 0 {
		return false
	}
	reserveCount := s.imageReserveCountLocked(authName, account, policy)
	return remaining > reserveCount
}

func (s *Store) imageReserveCountLocked(authName string, account PublicAccount, policy ImageAccountRoutingPolicy) int {
	state := s.states[authName]
	base, _, ok := extractImageQuotaSnapshot(account.LimitsProgress, account.RestoreAt, account.Quota)
	if state.ImageQuotaDailyBase > 0 {
		base = state.ImageQuotaDailyBase
	}
	if !ok || base <= 0 {
		base = max(0, account.Quota)
	}
	if base <= 0 {
		return 0
	}

	reserveCount := int(math.Ceil(float64(base*policy.ReservePercent) / 100.0))
	if reserveCount < 0 {
		reserveCount = 0
	}
	if reserveCount >= base {
		reserveCount = base - 1
	}
	if reserveCount < 0 {
		reserveCount = 0
	}
	return reserveCount
}

func currentImageRemaining(account PublicAccount) int {
	remaining, _, ok := extractImageQuotaSnapshot(account.LimitsProgress, account.RestoreAt, account.Quota)
	if !ok {
		return max(0, account.Quota)
	}
	return max(0, remaining)
}
