package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"chatgpt2api/internal/outboundproxy"

	"github.com/BurntSushi/toml"
)

const (
	exampleConfigFile = "config.example.toml"
	userConfigFile    = "config.toml"
	dataDirName       = "data"
)

var (
	osGetwd      = os.Getwd
	osExecutable = os.Executable
)

type Paths struct {
	Root     string
	Defaults string
	Override string
}

type AppConfig struct {
	Name            string `toml:"name"`
	Version         string `toml:"version"`
	APIKey          string `toml:"api_key"`
	AuthKey         string `toml:"auth_key"`
	ImageFormat     string `toml:"image_format"`
	MaxUploadSizeMB int    `toml:"max_upload_size_mb"`
}

type ServerConfig struct {
	Host                     string `toml:"host"`
	Port                     int    `toml:"port"`
	StaticDir                string `toml:"static_dir"`
	MaxImageConcurrency      int    `toml:"max_image_concurrency"`
	ImageQueueLimit          int    `toml:"image_queue_limit"`
	ImageQueueTimeoutSeconds int    `toml:"image_queue_timeout_seconds"`
	ImageTaskQueueTTLSeconds int    `toml:"image_task_queue_ttl_seconds"`
}

type ChatGPTConfig struct {
	Model                            string `toml:"model"`
	SSETimeout                       int    `toml:"sse_timeout"`
	PollInterval                     int    `toml:"poll_interval"`
	PollMaxWait                      int    `toml:"poll_max_wait"`
	RequestTimeout                   int    `toml:"request_timeout"`
	ImageMode                        string `toml:"image_mode"`
	FreeImageRoute                   string `toml:"free_image_route"`
	FreeImageModel                   string `toml:"free_image_model"`
	PaidImageRoute                   string `toml:"paid_image_route"`
	PaidImageModel                   string `toml:"paid_image_model"`
	StudioAllowDisabledImageAccounts bool   `toml:"studio_allow_disabled_image_accounts"`
}

type AccountsConfig struct {
	DefaultQuota                int  `toml:"default_quota"`
	PreferRemoteRefresh         bool `toml:"prefer_remote_refresh"`
	RefreshWorkers              int  `toml:"refresh_workers"`
	AutoRefreshEnabled          bool `toml:"auto_refresh_enabled"`
	AutoRefreshInterval         int  `toml:"auto_refresh_interval_minutes"`
	ImageQuotaRefreshTTLSeconds int  `toml:"image_quota_refresh_ttl_seconds"`
}

type StorageConfig struct {
	Backend                  string `toml:"backend"`
	ConfigBackend            string `toml:"config_backend"`
	AuthDir                  string `toml:"auth_dir"`
	StateFile                string `toml:"state_file"`
	SyncStateDir             string `toml:"sync_state_dir"`
	ImageDir                 string `toml:"image_dir"`
	ImageStorage             string `toml:"image_storage"`
	ImageConversationStorage string `toml:"image_conversation_storage"`
	ImageDataStorage         string `toml:"image_data_storage"`
	SQLitePath               string `toml:"sqlite_path"`
	RedisAddr                string `toml:"redis_addr"`
	RedisPassword            string `toml:"redis_password"`
	RedisDB                  int    `toml:"redis_db"`
	RedisPrefix              string `toml:"redis_prefix"`
}

type SyncConfig struct {
	Enabled        bool   `toml:"enabled"`
	BaseURL        string `toml:"base_url"`
	ManagementKey  string `toml:"management_key"`
	RequestTimeout int    `toml:"request_timeout"`
	Concurrency    int    `toml:"concurrency"`
	ProviderType   string `toml:"provider_type"`
}

type LogConfig struct {
	LogAllRequests bool `toml:"log_all_requests"`
}

type ProxyConfig struct {
	Enabled          bool   `toml:"enabled"`
	URL              string `toml:"url"`
	Mode             string `toml:"mode"`
	SyncEnabled      bool   `toml:"sync_enabled"`
	AutoRetryEnabled bool   `toml:"auto_retry_enabled"`
	ControllerURL    string `toml:"controller_url"`
	ControllerSecret string `toml:"controller_secret"`
	ControllerGroup  string `toml:"controller_group"`
}

type CPAConfig struct {
	BaseURL        string `toml:"base_url"`
	APIKey         string `toml:"api_key"`
	RequestTimeout int    `toml:"request_timeout"`
	RouteStrategy  string `toml:"route_strategy"`
}

type MailConfig struct {
	Enabled        bool   `toml:"enabled"`
	SMTPHost       string `toml:"smtp_host"`
	SMTPPort       int    `toml:"smtp_port"`
	Username       string `toml:"username"`
	Password       string `toml:"password"`
	FromAddress    string `toml:"from_address"`
	FromName       string `toml:"from_name"`
	UseImplicitTLS bool   `toml:"use_implicit_tls"`
	CodeTTLMinutes int    `toml:"code_ttl_minutes"`
	ResendInterval int    `toml:"resend_interval_seconds"`
}

type NewAPIConfig struct {
	BaseURL        string `toml:"base_url"`
	Username       string `toml:"username"`
	Password       string `toml:"password"`
	AccessToken    string `toml:"access_token"`
	UserID         int    `toml:"user_id"`
	SessionCookie  string `toml:"session_cookie"`
	RequestTimeout int    `toml:"request_timeout"`
}

type Sub2APIConfig struct {
	BaseURL        string `toml:"base_url"`
	Email          string `toml:"email"`
	Password       string `toml:"password"`
	APIKey         string `toml:"api_key"`
	GroupID        string `toml:"group_id"`
	RequestTimeout int    `toml:"request_timeout"`
}

type Config struct {
	mu     sync.RWMutex `toml:"-"`
	loadMu sync.Mutex   `toml:"-"`
	loaded bool         `toml:"-"`
	paths  Paths        `toml:"-"`

	App      AppConfig      `toml:"app"`
	Server   ServerConfig   `toml:"server"`
	ChatGPT  ChatGPTConfig  `toml:"chatgpt"`
	Accounts AccountsConfig `toml:"accounts"`
	Storage  StorageConfig  `toml:"storage"`
	Sync     SyncConfig     `toml:"sync"`
	Log      LogConfig      `toml:"log"`
	Proxy    ProxyConfig    `toml:"proxy"`
	CPA      CPAConfig      `toml:"cpa"`
	Mail     MailConfig     `toml:"mail"`
	NewAPI   NewAPIConfig   `toml:"newapi"`
	Sub2API  Sub2APIConfig  `toml:"sub2api"`
}

func New(rootDir string) *Config {
	return &Config{paths: resolvePaths(rootDir)}
}

func (c *Config) Load() error {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()

	next := &Config{paths: c.paths}

	if err := decodeDefaultTemplate(next); err != nil {
		return fmt.Errorf("decode embedded defaults: %w", err)
	}
	if fileExists(c.paths.Override) {
		_, _ = migrateLegacyOverrideFile(c.paths.Override)
		if err := decodeOverrideFile(c.paths.Override, next); err != nil {
			return fmt.Errorf("decode override: %w", err)
		}
	}
	if err := next.validate(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.copyFrom(next)
	c.loaded = true
	return nil
}

func (c *Config) EnsureLoaded() error {
	c.mu.RLock()
	loaded := c.loaded
	c.mu.RUnlock()
	if loaded {
		return nil
	}
	return c.Load()
}

func (c *Config) GetString(key string, fallback ...string) string {
	value, ok := c.lookup(key)
	if !ok {
		return stringFallback(fallback)
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return stringFallback(fallback)
	}
}

func (c *Config) GetInt(key string, fallback ...int) int {
	value, ok := c.lookup(key)
	if !ok {
		return intFallback(fallback)
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	default:
		return intFallback(fallback)
	}
}

func (c *Config) GetBool(key string, fallback ...bool) bool {
	value, ok := c.lookup(key)
	if !ok {
		return boolFallback(fallback)
	}
	typed, ok := value.(bool)
	if !ok {
		return boolFallback(fallback)
	}
	return typed
}

func (c *Config) Paths() Paths {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.paths
}

func (c *Config) RootDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.paths.Root
}

func (c *Config) ResolvePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return c.RootDir()
	}
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	return filepath.Join(c.RootDir(), trimmed)
}

func (c *Config) SaveOverride(section, key string, value any) error {
	return c.SaveOverrides(map[string]map[string]any{
		section: {
			key: value,
		},
	})
}

func (c *Config) SaveOverrides(values map[string]map[string]any) error {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()

	raw, err := mergeOverrideValues(c.paths.Override, values)
	if err != nil {
		return err
	}

	if err := writeOverrideMap(c.paths.Override, raw); err != nil {
		return err
	}

	next := &Config{paths: c.paths}
	if err := decodeDefaultTemplate(next); err != nil {
		return fmt.Errorf("reload embedded defaults: %w", err)
	}
	if fileExists(c.paths.Override) {
		if err := decodeOverrideFile(c.paths.Override, next); err != nil {
			return fmt.Errorf("reload override: %w", err)
		}
	}
	if err := next.validate(); err != nil {
		return err
	}

	c.mu.Lock()
	c.copyFrom(next)
	c.loaded = true
	c.mu.Unlock()
	return nil
}

func (c *Config) PersistOverrideFile(values map[string]map[string]any) error {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()

	raw, err := mergeOverrideValues(c.paths.Override, values)
	if err != nil {
		return err
	}
	return writeOverrideMap(c.paths.Override, raw)
}

func (c *Config) ApplyOverrides(values map[string]map[string]any) error {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()

	raw := map[string]any{}
	for section, entries := range values {
		sectionMap := map[string]any{}
		for key, value := range entries {
			sectionMap[key] = sanitizeOverrideEntry(section, key, value)
		}
		raw[section] = sectionMap
	}
	sanitizeOverrideMap(raw)

	next := &Config{paths: c.paths}
	c.mu.RLock()
	next.copyFrom(c)
	next.paths = c.paths
	c.mu.RUnlock()

	if err := applyOverrideMap(reflect.ValueOf(next).Elem(), raw); err != nil {
		return err
	}
	if err := next.validate(); err != nil {
		return err
	}

	c.mu.Lock()
	c.copyFrom(next)
	c.loaded = true
	c.mu.Unlock()
	return nil
}

func mergeOverrideValues(path string, values map[string]map[string]any) (map[string]any, error) {
	raw := map[string]any{}
	if fileExists(path) {
		loaded, err := loadOverrideMap(path)
		if err != nil {
			return nil, fmt.Errorf("read override: %w", err)
		}
		raw = loaded
	}
	sanitizeOverrideMap(raw)

	for section, entries := range values {
		sec, ok := raw[section].(map[string]any)
		if !ok {
			sec = map[string]any{}
		}
		for key, value := range entries {
			sec[key] = sanitizeOverrideEntry(section, key, value)
		}
		raw[section] = sec
	}
	return raw, nil
}

func LoadDefaults(paths Paths) (*Config, error) {
	next := &Config{paths: paths}
	if err := decodeDefaultTemplate(next); err != nil {
		return nil, fmt.Errorf("decode embedded defaults: %w", err)
	}
	if err := next.validate(); err != nil {
		return nil, err
	}
	next.loaded = true
	return next, nil
}

func (c *Config) lookup(key string) (any, bool) {
	if err := c.EnsureLoaded(); err != nil {
		return nil, false
	}

	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	current := reflect.ValueOf(c).Elem()
	for _, part := range parts {
		current = indirectValue(current)
		if !current.IsValid() || current.Kind() != reflect.Struct {
			return nil, false
		}
		next, ok := structFieldByTOMLTag(current, part)
		if !ok {
			return nil, false
		}
		current = next
	}

	current = indirectValue(current)
	if !current.IsValid() {
		return nil, false
	}
	return current.Interface(), true
}

func (c *Config) copyFrom(other *Config) {
	c.App = other.App
	c.Server = other.Server
	c.ChatGPT = other.ChatGPT
	c.Accounts = other.Accounts
	c.Storage = other.Storage
	c.Sync = other.Sync
	c.Log = other.Log
	c.Proxy = other.Proxy
	c.CPA = other.CPA
	c.Mail = other.Mail
	c.NewAPI = other.NewAPI
	c.Sub2API = other.Sub2API
	c.paths = other.paths
}

func (c *Config) ChatGPTProxyURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.proxyURLLocked(false)
}

func (c *Config) SyncProxyURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.proxyURLLocked(true)
}

func (c *Config) CPAImageBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if trimmed := strings.TrimSpace(c.CPA.BaseURL); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(c.Sync.BaseURL)
}

func (c *Config) CPAImageAPIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return strings.TrimSpace(c.CPA.APIKey)
}

func (c *Config) CPAImageConfigured() bool {
	return c.CPAImageBaseURL() != "" && c.CPAImageAPIKey() != ""
}

func (c *Config) CPAImageRequestTimeout() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.CPA.RequestTimeout > 0 {
		return c.CPA.RequestTimeout
	}
	return 60
}

func (c *Config) CPAImageRouteStrategy() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return normalizeCPAImageRouteStrategy(c.CPA.RouteStrategy)
}

func (c *Config) ImageQueueConfig() (int, int, time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	maxImageConcurrency := c.Server.MaxImageConcurrency
	if maxImageConcurrency <= 0 {
		maxImageConcurrency = 8
	}
	imageQueueLimit := c.Server.ImageQueueLimit
	if imageQueueLimit <= 0 {
		imageQueueLimit = 32
	}
	timeoutSeconds := c.Server.ImageQueueTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	return maxImageConcurrency, imageQueueLimit, time.Duration(timeoutSeconds) * time.Second
}

func (c *Config) ImageTaskQueueTTL() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ttlSeconds := c.Server.ImageTaskQueueTTLSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = 600
	}
	return time.Duration(ttlSeconds) * time.Second
}

func (c *Config) ImageQuotaRefreshTTL() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ttlSeconds := c.Accounts.ImageQuotaRefreshTTLSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	return time.Duration(ttlSeconds) * time.Second
}

func (c *Config) proxyURLLocked(forSync bool) string {
	if !c.Proxy.Enabled {
		return ""
	}
	if forSync && !c.Proxy.SyncEnabled {
		return ""
	}
	if normalizeProxyMode(c.Proxy.Mode) != "fixed" {
		return ""
	}
	return strings.TrimSpace(c.Proxy.URL)
}

func (c *Config) ProxyAutoRetryEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Proxy.Enabled && c.Proxy.AutoRetryEnabled && normalizeProxyMode(c.Proxy.Mode) == "fixed"
}

func (c *Config) ProxyControllerURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return proxyControllerURLLocked(c.Proxy)
}

func (c *Config) ProxyControllerSecret() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return strings.TrimSpace(c.Proxy.ControllerSecret)
}

func (c *Config) ProxyControllerGroup() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return proxyControllerGroupLocked(c.Proxy)
}

func (c *Config) validate() error {
	if c.Server.MaxImageConcurrency <= 0 {
		c.Server.MaxImageConcurrency = 8
	}
	if c.Server.ImageQueueLimit <= 0 {
		c.Server.ImageQueueLimit = 32
	}
	if c.Server.ImageQueueTimeoutSeconds <= 0 {
		c.Server.ImageQueueTimeoutSeconds = 20
	}
	if c.Server.ImageTaskQueueTTLSeconds <= 0 {
		c.Server.ImageTaskQueueTTLSeconds = 600
	}
	if c.Accounts.ImageQuotaRefreshTTLSeconds <= 0 {
		c.Accounts.ImageQuotaRefreshTTLSeconds = 120
	}

	if normalized, ok := normalizeImageMode(c.ChatGPT.ImageMode); !ok {
		return fmt.Errorf("invalid chatgpt.image_mode %q: only studio or cpa are supported", strings.TrimSpace(c.ChatGPT.ImageMode))
	} else {
		c.ChatGPT.ImageMode = normalized
	}

	if normalized, ok := normalizeImageRoute(c.ChatGPT.FreeImageRoute); !ok {
		return fmt.Errorf("invalid chatgpt.free_image_route %q: only legacy or responses are supported", strings.TrimSpace(c.ChatGPT.FreeImageRoute))
	} else if normalized == "" {
		return fmt.Errorf("invalid chatgpt.free_image_route %q", strings.TrimSpace(c.ChatGPT.FreeImageRoute))
	} else {
		c.ChatGPT.FreeImageRoute = normalized
	}
	if normalized, ok := normalizeImageRoute(c.ChatGPT.PaidImageRoute); !ok {
		return fmt.Errorf("invalid chatgpt.paid_image_route %q: only legacy or responses are supported", strings.TrimSpace(c.ChatGPT.PaidImageRoute))
	} else if normalized == "" {
		return fmt.Errorf("invalid chatgpt.paid_image_route %q", strings.TrimSpace(c.ChatGPT.PaidImageRoute))
	} else {
		c.ChatGPT.PaidImageRoute = normalized
	}
	c.ChatGPT.FreeImageModel = normalizeConfiguredImageRouteModel(c.ChatGPT.FreeImageRoute, c.ChatGPT.FreeImageModel, "auto", true)
	c.ChatGPT.PaidImageModel = normalizeConfiguredImageRouteModel(c.ChatGPT.PaidImageRoute, c.ChatGPT.PaidImageModel, "gpt-5.4-mini", false)

	c.CPA.RouteStrategy = normalizeCPAImageRouteStrategy(c.CPA.RouteStrategy)
	c.Storage.Backend = normalizeStorageBackend(c.Storage.Backend)
	c.Storage.ConfigBackend = normalizeConfigBackend(c.Storage.ConfigBackend)
	legacyImageStorage := strings.ToLower(strings.TrimSpace(c.Storage.ImageStorage))
	if strings.TrimSpace(c.Storage.ImageConversationStorage) == "" && legacyImageStorage != "" {
		c.Storage.ImageConversationStorage = legacyImageStorage
	}
	if strings.TrimSpace(c.Storage.ImageDataStorage) == "" && legacyImageStorage != "" {
		c.Storage.ImageDataStorage = legacyImageStorage
	}
	c.Storage.ImageConversationStorage = normalizeImageStorage(c.Storage.ImageConversationStorage)
	c.Storage.ImageDataStorage = normalizeImageStorage(c.Storage.ImageDataStorage)
	if c.Storage.ImageConversationStorage != c.Storage.ImageDataStorage {
		c.Storage.ImageDataStorage = c.Storage.ImageConversationStorage
	}
	c.Storage.ImageStorage = c.Storage.ImageConversationStorage

	if !c.Proxy.Enabled {
		return nil
	}

	if normalizeProxyMode(c.Proxy.Mode) != "fixed" {
		return fmt.Errorf("unsupported proxy.mode %q: only fixed is supported", strings.TrimSpace(c.Proxy.Mode))
	}

	if strings.TrimSpace(c.Proxy.URL) == "" {
		return fmt.Errorf("proxy.url is required when proxy.enabled = true")
	}

	if err := outboundproxy.Validate(c.Proxy.URL); err != nil {
		return fmt.Errorf("invalid proxy.url: %w", err)
	}

	if controllerURL := strings.TrimSpace(c.Proxy.ControllerURL); controllerURL != "" {
		parsed, err := url.Parse(controllerURL)
		if err != nil {
			return fmt.Errorf("invalid proxy.controller_url: %w", err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("invalid proxy.controller_url %q: only http or https are supported", controllerURL)
		}
		if strings.TrimSpace(parsed.Host) == "" {
			return fmt.Errorf("invalid proxy.controller_url %q: host is required", controllerURL)
		}
	}

	return nil
}

func normalizeProxyMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		return "fixed"
	}
	return normalized
}

func proxyControllerURLLocked(proxyCfg ProxyConfig) string {
	if !proxyCfg.Enabled || !proxyCfg.AutoRetryEnabled || normalizeProxyMode(proxyCfg.Mode) != "fixed" {
		return ""
	}
	if explicit := strings.TrimSpace(proxyCfg.ControllerURL); explicit != "" {
		return explicit
	}
	return inferProxyControllerURL(strings.TrimSpace(proxyCfg.URL))
}

func proxyControllerGroupLocked(proxyCfg ProxyConfig) string {
	if group := strings.TrimSpace(proxyCfg.ControllerGroup); group != "" {
		return group
	}
	return "Proxy"
}

func inferProxyControllerURL(proxyURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return ""
	}
	if strings.EqualFold(host, "localhost") || strings.EqualFold(host, "host.docker.internal") {
		return "http://" + net.JoinHostPort(host, "9090")
	}
	ip := net.ParseIP(host)
	if ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) {
		return ""
	}
	return "http://" + net.JoinHostPort(host, "9090")
}

func normalizeImageRoute(route string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(route)) {
	case "", "legacy", "conversation":
		return "legacy", true
	case "responses":
		return "responses", true
	default:
		return "", false
	}
}

func normalizeConfiguredImageRouteModel(route, value, fallback string, allowAuto bool) string {
	model := strings.ToLower(strings.TrimSpace(value))
	if model == "" {
		return fallback
	}
	if allowAuto && model == "auto" {
		return "auto"
	}
	switch model {
	case "gpt-5.4-mini", "gpt-5.4", "gpt-5.5", "gpt-5-5-thinking":
		return model
	case "gpt-image-1", "gpt-image-2":
		if normalizedRoute, _ := normalizeImageRoute(route); normalizedRoute != "responses" {
			return model
		}
	}
	return fallback
}

func normalizeImageMode(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "studio", "mix":
		return "studio", true
	case "cpa":
		return "cpa", true
	default:
		return "", false
	}
}

func normalizeStorageBackend(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "current", "local":
		return "current"
	case "sqlite":
		return "sqlite"
	case "redis":
		return "redis"
	default:
		return "current"
	}
}

func normalizeConfigBackend(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "redis":
		return "redis"
	default:
		return "file"
	}
}

func normalizeImageStorage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "server":
		return "server"
	default:
		return "browser"
	}
}

func normalizeCPAImageRouteStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "images_api"
	case "images_api":
		return "images_api"
	case "codex_responses":
		return "codex_responses"
	case "auto":
		return "auto"
	default:
		return "images_api"
	}
}

func NormalizeImageModeForAPI(mode string) (string, bool) {
	return normalizeImageMode(mode)
}

func decodeOverrideFile(path string, target *Config) error {
	raw, err := loadOverrideMap(path)
	if err != nil {
		return err
	}
	sanitizeOverrideMap(raw)
	return applyOverrideMap(reflect.ValueOf(target).Elem(), raw)
}

func loadOverrideMap(path string) (map[string]any, error) {
	raw := map[string]any{}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func migrateLegacyOverrideFile(path string) (bool, error) {
	if !fileExists(path) {
		return false, nil
	}

	raw, err := loadOverrideMap(path)
	if err != nil {
		return false, err
	}
	if !sanitizeOverrideMap(raw) {
		return false, nil
	}
	if err := writeOverrideMap(path, raw); err != nil {
		return false, err
	}
	return true, nil
}

func sanitizeOverrideMap(raw map[string]any) bool {
	if raw == nil {
		return false
	}

	changed := false
	chatgptSection, ok := raw["chatgpt"].(map[string]any)
	if !ok {
		return false
	}
	if value, ok := chatgptSection["image_mode"]; ok {
		sanitized := sanitizeOverrideEntry("chatgpt", "image_mode", value)
		if !reflect.DeepEqual(sanitized, value) {
			chatgptSection["image_mode"] = sanitized
			changed = true
		}
	}
	return changed
}

func sanitizeOverrideEntry(section, key string, value any) any {
	if section != "chatgpt" || key != "image_mode" {
		return value
	}
	text, ok := value.(string)
	if !ok {
		return value
	}
	if normalized, ok := normalizeImageMode(text); ok {
		return normalized
	}
	return value
}

func writeOverrideMap(path string, raw map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create override file: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(raw); err != nil {
		return fmt.Errorf("encode override: %w", err)
	}
	return nil
}

func decodeDefaultTemplate(target *Config) error {
	_, err := toml.Decode(defaultConfigTemplate, target)
	return err
}

func applyOverrideMap(dst reflect.Value, raw map[string]any) error {
	for key, value := range raw {
		field, ok := structFieldByTOMLTag(dst, key)
		if !ok {
			continue
		}
		if err := setOverrideValue(field, value); err != nil {
			return err
		}
	}
	return nil
}

func setOverrideValue(dst reflect.Value, raw any) error {
	if !dst.CanSet() {
		return nil
	}
	dst = indirectValue(dst)
	if !dst.IsValid() {
		return nil
	}

	switch dst.Kind() {
	case reflect.Struct:
		nested, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("expected table, got %T", raw)
		}
		return applyOverrideMap(dst, nested)
	case reflect.String:
		text, ok := raw.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", raw)
		}
		dst.SetString(text)
	case reflect.Bool:
		flag, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("expected bool, got %T", raw)
		}
		dst.SetBool(flag)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch n := raw.(type) {
		case int64:
			dst.SetInt(n)
		case int:
			dst.SetInt(int64(n))
		case float64:
			dst.SetInt(int64(n))
		default:
			return fmt.Errorf("expected int, got %T", raw)
		}
	default:
		value := reflect.ValueOf(raw)
		if value.IsValid() && value.Type().AssignableTo(dst.Type()) {
			dst.Set(value)
			return nil
		}
		return fmt.Errorf("unsupported type %s", dst.Type())
	}
	return nil
}

func structFieldByTOMLTag(value reflect.Value, part string) (reflect.Value, bool) {
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldType := valueType.Field(i)
		if !fieldType.IsExported() {
			continue
		}
		tag := strings.Split(fieldType.Tag.Get("toml"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == "" {
			tag = strings.ToLower(fieldType.Name)
		}
		if tag == part {
			return value.Field(i), true
		}
	}
	return reflect.Value{}, false
}

func indirectValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func resolvePaths(rootDir string) Paths {
	root := normalizeRoot(rootDir)
	return Paths{
		Root:     root,
		Defaults: filepath.Join(root, dataDirName, exampleConfigFile),
		Override: filepath.Join(root, dataDirName, userConfigFile),
	}
}

func normalizeRoot(rootDir string) string {
	if rootDir != "" {
		return rootDir
	}
	if exePath, err := osExecutable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if detected := detectConfigRoot(exeDir); detected != "" {
			return detected
		}
	}
	if cwd, err := osGetwd(); err == nil {
		if detected := detectConfigRoot(cwd); detected != "" {
			return detected
		}
	}
	if exePath, err := osExecutable(); err == nil {
		if exeDir := filepath.Dir(exePath); exeDir != "" {
			return exeDir
		}
	}
	if cwd, err := osGetwd(); err == nil {
		return cwd
	}
	return "."
}

func detectConfigRoot(startDir string) string {
	dir := startDir
	for {
		// Prefer a local config root when running from backend itself or from a release package.
		if hasConfigMarker(dir) {
			return dir
		}
		// Backward compatibility: older layout placed defaults in backend/config.defaults.toml.
		if fileExists(filepath.Join(dir, "config.defaults.toml")) {
			return dir
		}
		// Support running from repo root (or any subdir) by locating backend/data config files.
		backendDir := filepath.Join(dir, "backend")
		if hasConfigMarker(backendDir) {
			return backendDir
		}
		// Backward compatibility: older layout placed defaults in backend/config.defaults.toml.
		if fileExists(filepath.Join(backendDir, "config.defaults.toml")) {
			return backendDir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func hasConfigMarker(root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	dataDir := filepath.Join(root, dataDirName)
	return fileExists(filepath.Join(dataDir, userConfigFile)) ||
		fileExists(filepath.Join(dataDir, exampleConfigFile)) ||
		fileExists(filepath.Join(dataDir, "config.defaults.toml")) ||
		fileExists(filepath.Join(root, "internal", "config", "config.defaults.toml"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func stringFallback(values []string) string {
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func intFallback(values []int) int {
	if len(values) > 0 {
		return values[0]
	}
	return 0
}

func boolFallback(values []bool) bool {
	if len(values) > 0 {
		return values[0]
	}
	return false
}
