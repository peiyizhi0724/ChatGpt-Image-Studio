package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"chatgpt2api/api"
	"chatgpt2api/internal/accounts"
	"chatgpt2api/internal/buildinfo"
	"chatgpt2api/internal/cliproxy"
	"chatgpt2api/internal/config"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.New("")
	paths := cfg.Paths()
	bootstrap, err := config.EnsureRuntimeFiles(paths)
	if err != nil {
		fatalStartup(logger, paths, "初始化配置文件失败", err,
			"请检查程序所在目录是否具有写入权限",
			fmt.Sprintf("目标配置目录：%s", filepath.Dir(paths.Override)),
		)
	}
	if bootstrap.OverrideWritten {
		logger.Info("config file created", slog.String("path", paths.Override))
	}

	if err := cfg.Load(); err != nil {
		fatalStartup(logger, paths, "读取配置失败", err,
			fmt.Sprintf("请检查 %s 是否是有效的 TOML 文件", paths.Override),
			fmt.Sprintf("参考示例配置：%s", paths.Defaults),
		)
	}
	cfg.App.Version = buildinfo.ResolveVersion(cfg.App.Version)

	if err := ensureStaticAssets(cfg.ResolvePath(cfg.Server.StaticDir)); err != nil {
		fatalStartup(logger, paths, "静态资源缺失", err,
			fmt.Sprintf("当前静态目录：%s", cfg.ResolvePath(cfg.Server.StaticDir)),
			"请重新构建前端资源或重新解压发布包后再启动",
		)
	}

	store, err := accounts.NewStore(cfg)
	if err != nil {
		fatalStartup(logger, paths, "初始化账号存储失败", err)
	}

	syncTimeout := time.Duration(max(10, cfg.Sync.RequestTimeout)) * time.Second
	syncClient := cliproxy.New(cfg.Sync.Enabled, cfg.Sync.BaseURL, cfg.Sync.ManagementKey, cfg.Sync.ProviderType, syncTimeout, cfg.SyncProxyURL())
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	defer backgroundCancel()
	go store.RunAutoRefreshLoop(backgroundCtx)

	host := envString("SERVER_HOST", cfg.Server.Host)
	port := envInt("SERVER_PORT", cfg.Server.Port)
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	server := &http.Server{
		Addr:              addr,
		Handler:           api.SetupRouter(cfg, store, syncClient),
		ReadHeaderTimeout: 5 * time.Second,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddressInUseError(err) {
			fatalStartup(logger, paths, "端口占用", err,
				fmt.Sprintf("请修改 %s 中的 server.port，或关闭占用该端口的进程", paths.Override),
			)
		}
		fatalStartup(logger, paths, "启动服务失败", err)
	}
	if clearErr := clearStartupError(paths); clearErr != nil {
		logger.Warn("clear startup error log", slog.Any("error", clearErr))
	}
	logger.Info("chatgpt2api-studio listening", slog.String("addr", addr))

	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if isAddressInUseError(err) {
			fatalStartup(logger, paths, "端口占用", err,
				fmt.Sprintf("请修改 %s 中的 server.port，或关闭占用该端口的进程", paths.Override),
			)
		}
		fatalStartup(logger, paths, "启动服务失败", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	backgroundCancel()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutCtx)
	logger.Info("server stopped")
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ensureStaticAssets(staticDir string) error {
	if strings.TrimSpace(staticDir) == "" {
		return fmt.Errorf("server.static_dir 不能为空")
	}
	info, err := os.Stat(staticDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s 不是目录", staticDir)
	}
	indexPath := filepath.Join(staticDir, "index.html")
	indexInfo, err := os.Stat(indexPath)
	if err != nil {
		return err
	}
	if indexInfo.IsDir() {
		return fmt.Errorf("%s 不是文件", indexPath)
	}
	return nil
}

func isAddressInUseError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "address already in use") ||
		strings.Contains(message, "only one usage of each socket address")
}

func fatalStartup(logger *slog.Logger, paths config.Paths, summary string, err error, hints ...string) {
	if logger != nil {
		logger.Error(summary, slog.Any("error", err))
	}

	var builder strings.Builder
	builder.WriteString(summary)
	if err != nil {
		builder.WriteString("：")
		builder.WriteString(err.Error())
	}
	for _, hint := range hints {
		if strings.TrimSpace(hint) == "" {
			continue
		}
		builder.WriteString("\n- ")
		builder.WriteString(strings.TrimSpace(hint))
	}

	details := builder.String()
	fmt.Fprintln(os.Stderr, details)
	if logPath, writeErr := writeStartupError(paths, details); writeErr == nil {
		fmt.Fprintf(os.Stderr, "详细信息已写入 %s\n", logPath)
	}
	os.Exit(1)
}

func writeStartupError(paths config.Paths, details string) (string, error) {
	logPath := filepath.Join(paths.Root, "data", "last-startup-error.txt")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(logPath, []byte(details+"\n"), 0o644); err != nil {
		return "", err
	}
	return logPath, nil
}

func clearStartupError(paths config.Paths) error {
	logPath := filepath.Join(paths.Root, "data", "last-startup-error.txt")
	if _, err := os.Stat(logPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return os.Remove(logPath)
}