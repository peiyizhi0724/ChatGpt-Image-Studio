package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateProxyConfig(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{
			Enabled: true,
			URL:     "socks5h://127.0.0.1:10808",
			Mode:    "fixed",
		},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("expected valid proxy config, got %v", err)
	}
}

func TestValidateRejectsUnsupportedProxyMode(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{
			Enabled: true,
			URL:     "socks5h://127.0.0.1:10808",
			Mode:    "singbox",
		},
	}

	if err := cfg.validate(); err == nil {
		t.Fatal("expected unsupported mode validation error")
	}
}

func TestProxyURLs(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{
			Enabled:     true,
			URL:         "socks5h://127.0.0.1:10808",
			Mode:        "fixed",
			SyncEnabled: false,
		},
	}

	if got := cfg.ChatGPTProxyURL(); got != "socks5h://127.0.0.1:10808" {
		t.Fatalf("unexpected chatgpt proxy url %q", got)
	}
	if got := cfg.SyncProxyURL(); got != "" {
		t.Fatalf("expected sync proxy to be disabled, got %q", got)
	}

	cfg.Proxy.SyncEnabled = true
	if got := cfg.SyncProxyURL(); got != "socks5h://127.0.0.1:10808" {
		t.Fatalf("unexpected sync proxy url %q", got)
	}
}

func TestDetectConfigRootFindsBackendFromRepoRoot(t *testing.T) {
	rootDir := t.TempDir()
	backendDir := filepath.Join(rootDir, "backend")
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	internalConfigDir := filepath.Join(backendDir, "internal", "config")
	if err := os.MkdirAll(internalConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir internal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(internalConfigDir, "config.defaults.toml"), []byte("defaults"), 0o644); err != nil {
		t.Fatalf("write internal defaults: %v", err)
	}

	got := detectConfigRoot(rootDir)
	if got != backendDir {
		t.Fatalf("expected detected root %q, got %q", backendDir, got)
	}
}

func TestNormalizeRootPrefersExecutableConfigRoot(t *testing.T) {
	releaseDir := t.TempDir()
	releaseDataDir := filepath.Join(releaseDir, "data")
	if err := os.MkdirAll(releaseDataDir, 0o755); err != nil {
		t.Fatalf("mkdir release data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(releaseDataDir, exampleConfigFile), []byte(""), 0o644); err != nil {
		t.Fatalf("write release example config: %v", err)
	}

	workingDir := t.TempDir()
	workingDataDir := filepath.Join(workingDir, "data")
	if err := os.MkdirAll(workingDataDir, 0o755); err != nil {
		t.Fatalf("mkdir working data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDataDir, exampleConfigFile), []byte(""), 0o644); err != nil {
		t.Fatalf("write working example config: %v", err)
	}

	originalGetwd := osGetwd
	originalExecutable := osExecutable
	t.Cleanup(func() {
		osGetwd = originalGetwd
		osExecutable = originalExecutable
	})

	osGetwd = func() (string, error) {
		return workingDir, nil
	}
	osExecutable = func() (string, error) {
		return filepath.Join(releaseDir, "chatgpt-image-studio.exe"), nil
	}

	if got := normalizeRoot(""); got != releaseDir {
		t.Fatalf("expected normalizeRoot to prefer executable dir %q, got %q", releaseDir, got)
	}
}

func TestValidateTreatsLegacyMixAsStudio(t *testing.T) {
	cfg := &Config{
		ChatGPT: ChatGPTConfig{
			ImageMode:      "mix",
			FreeImageRoute: "legacy",
			PaidImageRoute: "responses",
		},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() returned error: %v", err)
	}
	if cfg.ChatGPT.ImageMode != "studio" {
		t.Fatalf("image mode = %q, want studio", cfg.ChatGPT.ImageMode)
	}
}

func TestLoadMigratesLegacyMixOverrideToStudio(t *testing.T) {
	rootDir := t.TempDir()
	dataDir := filepath.Join(rootDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}

	overridePath := filepath.Join(dataDir, userConfigFile)
	override := strings.Join([]string{
		"[chatgpt]",
		`image_mode = "mix"`,
		`free_image_route = "legacy"`,
		`paid_image_route = "responses"`,
		"",
	}, "\n")
	if err := os.WriteFile(overridePath, []byte(override), 0o644); err != nil {
		t.Fatalf("write override: %v", err)
	}

	cfg := New(rootDir)
	if err := cfg.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.ChatGPT.ImageMode != "studio" {
		t.Fatalf("loaded image mode = %q, want studio", cfg.ChatGPT.ImageMode)
	}

	content, err := os.ReadFile(overridePath)
	if err != nil {
		t.Fatalf("read override: %v", err)
	}
	if !strings.Contains(string(content), `image_mode = "studio"`) {
		t.Fatalf("override file was not migrated to studio: %s", string(content))
	}
	if strings.Contains(string(content), `image_mode = "mix"`) {
		t.Fatalf("override file still contains legacy mix value: %s", string(content))
	}
}

func TestNormalizeCPAImageRouteStrategyPreservesKnownValues(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty defaults to images api", input: "", want: "images_api"},
		{name: "images api stays images api", input: "images_api", want: "images_api"},
		{name: "codex responses stays codex responses", input: "codex_responses", want: "codex_responses"},
		{name: "auto stays auto", input: "auto", want: "auto"},
		{name: "unknown falls back to images api", input: "something-else", want: "images_api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeCPAImageRouteStrategy(tt.input); got != tt.want {
				t.Fatalf("normalizeCPAImageRouteStrategy(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestImageQueueAndQuotaRefreshDefaults(t *testing.T) {
	cfg := &Config{}
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() returned error: %v", err)
	}

	maxConcurrency, queueLimit, queueTimeout := cfg.ImageQueueConfig()
	if maxConcurrency != 8 {
		t.Fatalf("maxConcurrency = %d, want 8", maxConcurrency)
	}
	if queueLimit != 32 {
		t.Fatalf("queueLimit = %d, want 32", queueLimit)
	}
	if queueTimeout != 20*time.Second {
		t.Fatalf("queueTimeout = %s, want 20s", queueTimeout)
	}
	if got := cfg.ImageQuotaRefreshTTL(); got != 120*time.Second {
		t.Fatalf("ImageQuotaRefreshTTL() = %s, want 120s", got)
	}
}

func TestImageQueueAndQuotaRefreshCustomValues(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			MaxImageConcurrency:      12,
			ImageQueueLimit:          48,
			ImageQueueTimeoutSeconds: 35,
		},
		Accounts: AccountsConfig{
			ImageQuotaRefreshTTLSeconds: 180,
		},
	}

	maxConcurrency, queueLimit, queueTimeout := cfg.ImageQueueConfig()
	if maxConcurrency != 12 {
		t.Fatalf("maxConcurrency = %d, want 12", maxConcurrency)
	}
	if queueLimit != 48 {
		t.Fatalf("queueLimit = %d, want 48", queueLimit)
	}
	if queueTimeout != 35*time.Second {
		t.Fatalf("queueTimeout = %s, want 35s", queueTimeout)
	}
	if got := cfg.ImageQuotaRefreshTTL(); got != 180*time.Second {
		t.Fatalf("ImageQuotaRefreshTTL() = %s, want 180s", got)
	}
}
