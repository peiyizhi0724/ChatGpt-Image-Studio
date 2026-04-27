package users

import (
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"sync"
	"time"

	"chatgpt2api/internal/config"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

var (
	ErrNotFound           = fmt.Errorf("user not found")
	ErrEmailExists        = fmt.Errorf("email already exists")
	ErrInvalidCredentials = fmt.Errorf("invalid email or password")
	ErrCurrentPassword    = fmt.Errorf("current password is invalid")
	ErrUserDisabled       = fmt.Errorf("user is disabled")
	ErrLastAdminRequired  = fmt.Errorf("at least one enabled admin is required")
)

type Record struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	AvatarURL    string `json:"avatar_url"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	Disabled     bool   `json:"disabled"`
	CreatedAt    string `json:"created_at"`
	LastLoginAt  string `json:"last_login_at,omitempty"`
}

type PublicUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Role        string `json:"role"`
	Disabled    bool   `json:"disabled"`
	CreatedAt   string `json:"created_at"`
	LastLoginAt string `json:"last_login_at,omitempty"`
}

type Update struct {
	Role     *string
	Disabled *bool
}

type ProfileUpdate struct {
	DisplayName *string
	AvatarURL   *string
}

type storageBackend interface {
	Init() error
	Close() error
	Load() ([]Record, error)
	Save([]Record) error
}

type Store struct {
	backend storageBackend

	mu    sync.RWMutex
	users map[string]Record
}

func NewStore(cfg *config.Config) (*Store, error) {
	backend := newStorageBackend(cfg)
	if err := backend.Init(); err != nil {
		_ = backend.Close()
		return nil, err
	}

	store := &Store{
		backend: backend,
		users:   map[string]Record{},
	}
	if err := store.load(); err != nil {
		_ = backend.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.backend == nil {
		return nil
	}
	return s.backend.Close()
}

func (s *Store) Register(email, password string) (PublicUser, error) {
	normalizedEmail, err := normalizeEmail(email)
	if err != nil {
		return PublicUser{}, err
	}
	if err := validatePassword(password); err != nil {
		return PublicUser{}, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return PublicUser{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.findByEmailLocked(normalizedEmail) != nil {
		return PublicUser{}, ErrEmailExists
	}

	role := RoleUser
	if s.countEnabledAdminsLocked() == 0 {
		role = RoleAdmin
	}

	record := Record{
		ID:           uuid.NewString(),
		Email:        normalizedEmail,
		DisplayName:  buildDefaultDisplayName(normalizedEmail),
		AvatarURL:    "",
		PasswordHash: string(passwordHash),
		Role:         role,
		Disabled:     false,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.users[record.ID] = record
	if err := s.saveLocked(); err != nil {
		delete(s.users, record.ID)
		return PublicUser{}, err
	}
	return toPublicUser(record), nil
}

func (s *Store) Authenticate(email, password string) (PublicUser, error) {
	normalizedEmail, err := normalizeEmail(email)
	if err != nil {
		return PublicUser{}, ErrInvalidCredentials
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record := s.findByEmailLocked(normalizedEmail)
	if record == nil {
		return PublicUser{}, ErrInvalidCredentials
	}
	if record.Disabled {
		return PublicUser{}, ErrUserDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(password)); err != nil {
		return PublicUser{}, ErrInvalidCredentials
	}

	record.LastLoginAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.users[record.ID] = *record
	if err := s.saveLocked(); err != nil {
		return PublicUser{}, err
	}
	return toPublicUser(*record), nil
}

func (s *Store) Get(id string) (PublicUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.users[strings.TrimSpace(id)]
	if !ok {
		return PublicUser{}, ErrNotFound
	}
	return toPublicUser(record), nil
}

func (s *Store) List() ([]PublicUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]PublicUser, 0, len(s.users))
	for _, record := range s.users {
		items = append(items, toPublicUser(record))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func (s *Store) Update(id string, update Update) (PublicUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.users[strings.TrimSpace(id)]
	if !ok {
		return PublicUser{}, ErrNotFound
	}

	original := record
	if update.Role != nil {
		record.Role = normalizeRole(*update.Role)
	}
	if update.Disabled != nil {
		record.Disabled = *update.Disabled
	}

	if record.Role != RoleAdmin && original.Role == RoleAdmin && s.countEnabledAdminsAfterLocked(record.ID, record) == 0 {
		return PublicUser{}, ErrLastAdminRequired
	}
	if record.Disabled && original.Role == RoleAdmin && s.countEnabledAdminsAfterLocked(record.ID, record) == 0 {
		return PublicUser{}, ErrLastAdminRequired
	}
	if record.Role != RoleAdmin && record.Disabled && s.countEnabledAdminsAfterLocked(record.ID, record) == 0 {
		return PublicUser{}, ErrLastAdminRequired
	}

	s.users[record.ID] = record
	if err := s.saveLocked(); err != nil {
		s.users[original.ID] = original
		return PublicUser{}, err
	}
	return toPublicUser(record), nil
}

func (s *Store) UpdateProfile(id string, update ProfileUpdate) (PublicUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.users[strings.TrimSpace(id)]
	if !ok {
		return PublicUser{}, ErrNotFound
	}

	original := record
	if update.DisplayName != nil {
		displayName, err := normalizeDisplayName(*update.DisplayName, record.Email)
		if err != nil {
			return PublicUser{}, err
		}
		record.DisplayName = displayName
	}
	if update.AvatarURL != nil {
		avatarURL, err := normalizeAvatarURL(*update.AvatarURL)
		if err != nil {
			return PublicUser{}, err
		}
		record.AvatarURL = avatarURL
	}

	s.users[record.ID] = record
	if err := s.saveLocked(); err != nil {
		s.users[original.ID] = original
		return PublicUser{}, err
	}
	return toPublicUser(record), nil
}

func (s *Store) ChangePassword(id, currentPassword, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.users[strings.TrimSpace(id)]
	if !ok {
		return ErrNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(record.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrCurrentPassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	original := record
	record.PasswordHash = string(passwordHash)
	s.users[record.ID] = record
	if err := s.saveLocked(); err != nil {
		s.users[original.ID] = original
		return err
	}
	return nil
}

func (s *Store) ResetPassword(email, newPassword string) error {
	normalizedEmail, err := normalizeEmail(email)
	if err != nil {
		return ErrNotFound
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record := s.findByEmailLocked(normalizedEmail)
	if record == nil {
		return ErrNotFound
	}

	original := *record
	record.PasswordHash = string(passwordHash)
	s.users[record.ID] = *record
	if err := s.saveLocked(); err != nil {
		s.users[record.ID] = original
		return err
	}
	return nil
}

func (s *Store) HasUsers() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users) > 0
}

func (s *Store) EmailExists(email string) bool {
	normalizedEmail, err := normalizeEmail(email)
	if err != nil {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.findByEmailLocked(normalizedEmail) != nil
}

func (s *Store) Snapshot() ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSortedRecords(recordsFromMap(s.users)), nil
}

func (s *Store) ReplaceAll(records []Record) error {
	nextUsers := map[string]Record{}
	for _, item := range cloneSortedRecords(records) {
		normalized, ok := normalizeRecord(item)
		if !ok {
			continue
		}
		nextUsers[normalized.ID] = normalized
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.users
	s.users = nextUsers
	if err := s.saveLocked(); err != nil {
		s.users = previous
		return err
	}
	return nil
}

func (s *Store) load() error {
	records, err := s.backend.Load()
	if err != nil {
		return err
	}

	for _, item := range records {
		normalized, ok := normalizeRecord(item)
		if !ok {
			continue
		}
		s.users[normalized.ID] = normalized
	}
	return nil
}

func (s *Store) saveLocked() error {
	return s.backend.Save(recordsFromMap(s.users))
}

func (s *Store) findByEmailLocked(email string) *Record {
	for _, item := range s.users {
		if item.Email == email {
			copy := item
			return &copy
		}
	}
	return nil
}

func (s *Store) countEnabledAdminsLocked() int {
	count := 0
	for _, item := range s.users {
		if item.Role == RoleAdmin && !item.Disabled {
			count++
		}
	}
	return count
}

func (s *Store) countEnabledAdminsAfterLocked(targetID string, replacement Record) int {
	count := 0
	for id, item := range s.users {
		current := item
		if id == targetID {
			current = replacement
		}
		if current.Role == RoleAdmin && !current.Disabled {
			count++
		}
	}
	return count
}

func toPublicUser(record Record) PublicUser {
	return PublicUser{
		ID:          record.ID,
		Email:       record.Email,
		DisplayName: firstNonEmptyTrimmed(record.DisplayName, buildDefaultDisplayName(record.Email)),
		AvatarURL:   strings.TrimSpace(record.AvatarURL),
		Role:        normalizeRole(record.Role),
		Disabled:    record.Disabled,
		CreatedAt:   record.CreatedAt,
		LastLoginAt: record.LastLoginAt,
	}
}

func normalizeRecord(item Record) (Record, bool) {
	item.ID = strings.TrimSpace(item.ID)
	item.Email = strings.ToLower(strings.TrimSpace(item.Email))
	item.DisplayName = firstNonEmptyTrimmed(item.DisplayName, buildDefaultDisplayName(item.Email))
	item.AvatarURL = strings.TrimSpace(item.AvatarURL)
	item.Role = normalizeRole(item.Role)
	item.PasswordHash = strings.TrimSpace(item.PasswordHash)
	item.CreatedAt = strings.TrimSpace(item.CreatedAt)
	item.LastLoginAt = strings.TrimSpace(item.LastLoginAt)
	if item.ID == "" || item.Email == "" {
		return Record{}, false
	}
	return item, true
}

func recordsFromMap(items map[string]Record) []Record {
	result := make([]Record, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return cloneSortedRecords(result)
}

func cloneSortedRecords(items []Record) []Record {
	result := make([]Record, 0, len(items))
	for _, item := range items {
		normalized, ok := normalizeRecord(item)
		if !ok {
			continue
		}
		result = append(result, normalized)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt < result[j].CreatedAt
	})
	return result
}

func normalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", fmt.Errorf("email is required")
	}
	if _, err := mail.ParseAddress(normalized); err != nil {
		return "", fmt.Errorf("email is invalid")
	}
	return normalized, nil
}

func normalizeRole(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), RoleAdmin) {
		return RoleAdmin
	}
	return RoleUser
}

func validatePassword(password string) error {
	if len(strings.TrimSpace(password)) < 6 {
		return fmt.Errorf("password must be at least 6 characters")
	}
	return nil
}

func normalizeDisplayName(value, fallbackEmail string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return buildDefaultDisplayName(fallbackEmail), nil
	}
	if len([]rune(trimmed)) > 32 {
		return "", fmt.Errorf("display name must be at most 32 characters")
	}
	return trimmed, nil
}

func normalizeAvatarURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > 2*1024*1024 {
		return "", fmt.Errorf("avatar is too large")
	}
	lowered := strings.ToLower(trimmed)
	if strings.HasPrefix(lowered, "data:image/") || strings.HasPrefix(lowered, "https://") || strings.HasPrefix(lowered, "http://") {
		return trimmed, nil
	}
	return "", fmt.Errorf("avatar format is invalid")
}

func buildDefaultDisplayName(email string) string {
	localPart := strings.TrimSpace(email)
	if at := strings.Index(localPart, "@"); at > 0 {
		localPart = localPart[:at]
	}
	localPart = strings.TrimSpace(localPart)
	if localPart == "" {
		return "创作者"
	}
	return truncateRunes(localPart, 32)
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
