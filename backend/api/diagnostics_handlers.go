package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chatgpt2api/internal/buildinfo"
	"chatgpt2api/internal/outboundproxy"
)

const (
	checkStatusPass = "pass"
	checkStatusWarn = "warn"
	checkStatusFail = "fail"
)

type startupCheckItem struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	Hint       string `json:"hint,omitempty"`
	DurationMS int64  `json:"durationMs"`
}

type startupCheckResponse struct {
	StartedAt   string             `json:"startedAt"`
	FinishedAt  string             `json:"finishedAt"`
	Mode        string             `json:"mode"`
	Overall     string             `json:"overall"`
	PassCount   int                `json:"passCount"`
	WarnCount   int                `json:"warnCount"`
	FailCount   int                `json:"failCount"`
	Checks      []startupCheckItem `json:"checks"`
	SummaryText string             `json:"summaryText"`
}

type runtimeStatusResponse struct {
	Timestamp string `json:"timestamp"`
	Mode      string `json:"mode"`
	Admission struct {
		MaxConcurrency int   `json:"maxConcurrency"`
		QueueLimit     int   `json:"queueLimit"`
		QueueTimeoutMS int64 `json:"queueTimeoutMs"`
		Inflight       int   `json:"inflight"`
		Queued         int   `json:"queued"`
	} `json:"admission"`
	Accounts struct {
		Total         int `json:"total"`
		Available     int `json:"available"`
		AvailablePaid int `json:"availablePaid"`
	} `json:"accounts"`
	Recent struct {
		WindowSeconds    int    `json:"windowSeconds"`
		FailureCount     int    `json:"failureCount"`
		LastError        string `json:"lastError,omitempty"`
		LastErrorCode    string `json:"lastErrorCode,omitempty"`
		LastErrorAt      string `json:"lastErrorAt,omitempty"`
		LastErrorAccount string `json:"lastErrorAccount,omitempty"`
	} `json:"recent"`
}

type diagnosticsExportPayload struct {
	GeneratedAt  string                 `json:"generatedAt"`
	Version      map[string]string      `json:"version"`
	StartupCheck startupCheckResponse   `json:"startupCheck"`
	Runtime      runtimeStatusResponse  `json:"runtime"`
	Config       configPayload          `json:"config"`
	RequestLogs  []imageRequestLogEntry `json:"requestLogs"`
}

func (s *Server) handleStartupCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.runStartupCheck(r.Context()))
}

func (s *Server) handleRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.collectRuntimeStatus())
}

func (s *Server) handleExportDiagnostics(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	filename := fmt.Sprintf("chatgpt-image-studio-diagnostics-%s.json", now.Format("20060102-150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	writeJSON(w, http.StatusOK, diagnosticsExportPayload{
		GeneratedAt: now.Format(time.RFC3339Nano),
		Version: map[string]string{
			"version":   buildinfo.ResolveVersion(s.cfg.App.Version),
			"commit":    buildinfo.Commit,
			"buildTime": buildinfo.BuildTime,
		},
		StartupCheck: s.runStartupCheck(r.Context()),
		Runtime:      s.collectRuntimeStatus(),
		Config:       s.maskSensitiveConfig(s.buildConfigPayload()),
		RequestLogs:  s.reqLogs.list(100),
	})
}

func (s *Server) collectRuntimeStatus() runtimeStatusResponse {
	now := time.Now()
	out := runtimeStatusResponse{
		Timestamp: now.Format(time.RFC3339Nano),
		Mode:      s.configuredImageMode(),
	}

	maxConcurrent, queueLimit, queueTimeout := s.cfg.ImageQueueConfig()
	if s.imageAdmission != nil {
		snapshot := s.imageAdmission.snapshot(maxConcurrent, queueLimit, queueTimeout)
		out.Admission.MaxConcurrency = snapshot.MaxConcurrency
		out.Admission.QueueLimit = snapshot.QueueLimit
		out.Admission.QueueTimeoutMS = snapshot.QueueTimeoutMS
		out.Admission.Inflight = snapshot.Inflight
		out.Admission.Queued = snapshot.Queued
	}

	accountsList, err := s.getStore().ListAccounts()
	if err == nil {
		allowDisabled := s.allowDisabledStudioImageAccounts()
		out.Accounts.Total = len(accountsList)
		for _, account := range accountsList {
			if isImageAccountUsable(account, allowDisabled) {
				out.Accounts.Available++
				if account.Type == "Plus" || account.Type == "Pro" || account.Type == "Team" {
					out.Accounts.AvailablePaid++
				}
			}
		}
	}

	out.Recent.WindowSeconds = 600
	windowStart := now.Add(-10 * time.Minute)
	for _, item := range s.reqLogs.list(200) {
		if item.Success {
			continue
		}
		if out.Recent.LastError == "" {
			out.Recent.LastError = item.Error
			out.Recent.LastErrorCode = item.ErrorCode
			out.Recent.LastErrorAt = item.FinishedAt
			out.Recent.LastErrorAccount = firstNonEmpty(item.AccountEmail, item.AccountFile)
		}
		parsedAt, parseErr := time.Parse(time.RFC3339Nano, item.StartedAt)
		if parseErr == nil && parsedAt.After(windowStart) {
			out.Recent.FailureCount++
		}
	}

	return out
}

func (s *Server) runStartupCheck(ctx context.Context) startupCheckResponse {
	startedAt := time.Now()
	result := startupCheckResponse{
		StartedAt: startedAt.Format(time.RFC3339Nano),
		Mode:      s.configuredImageMode(),
		Checks:    make([]startupCheckItem, 0, 6),
	}

	addCheck := func(key, label string, run func() (string, string, string)) {
		checkStartedAt := time.Now()
		status, detail, hint := run()
		result.Checks = append(result.Checks, startupCheckItem{
			Key:        key,
			Label:      label,
			Status:     status,
			Detail:     detail,
			Hint:       hint,
			DurationMS: time.Since(checkStartedAt).Milliseconds(),
		})
	}

	addCheck("server", "后端服务", func() (string, string, string) {
		return checkStatusPass, fmt.Sprintf("服务已启动：%s:%d", strings.TrimSpace(s.cfg.Server.Host), s.cfg.Server.Port), ""
	})

	addCheck("proxy", "代理连通性", func() (string, string, string) {
		if !s.cfg.Proxy.Enabled {
			return checkStatusWarn, "未启用代理", "如需走代理访问官方链路，请先启用并填写 proxy.url"
		}
		if err := outboundproxy.Validate(s.cfg.Proxy.URL); err != nil {
			return checkStatusFail, fmt.Sprintf("代理配置无效：%v", err), "请检查代理 URL、协议与端口"
		}
		target, err := resolveProxyDialTarget(s.cfg.Proxy.URL)
		if err != nil {
			return checkStatusFail, fmt.Sprintf("代理地址解析失败：%v", err), ""
		}
		conn, dialErr := net.DialTimeout("tcp", target, 3*time.Second)
		if dialErr != nil {
			return checkStatusFail, fmt.Sprintf("代理不可达：%v", dialErr), "请确认代理程序已启动且端口正确"
		}
		_ = conn.Close()
		return checkStatusPass, fmt.Sprintf("代理可连接：%s", target), ""
	})

	addCheck("chatgpt", "官方站点连通", func() (string, string, string) {
		if result.Mode != "studio" {
			return checkStatusWarn, "当前不是 studio 模式，已跳过官方链路检测", ""
		}
		statusCode, err := probeEndpoint(ctx, "https://chatgpt.com", s.cfg.ChatGPTProxyURL(), 8*time.Second)
		if err != nil {
			return checkStatusFail, fmt.Sprintf("访问 chatgpt.com 失败：%v", err), "请检查代理、网络或防火墙设置"
		}
		return checkStatusPass, fmt.Sprintf("chatgpt.com 可达，HTTP %d", statusCode), ""
	})

	addCheck("cpa", "CPA 服务连通", func() (string, string, string) {
		baseURL := strings.TrimSpace(s.cfg.CPAImageBaseURL())
		if baseURL == "" {
			if result.Mode == "cpa" {
				return checkStatusFail, "CPA base URL 未配置", "请在配置中填写 cpa.base_url 或 sync.base_url"
			}
			return checkStatusWarn, "CPA base URL 未配置", ""
		}
		normalized := normalizeProbeURL(baseURL)
		statusCode, err := probeEndpoint(ctx, normalized, "", 5*time.Second)
		if err != nil {
			if result.Mode == "cpa" {
				return checkStatusFail, fmt.Sprintf("CPA 服务不可达：%v", err), ""
			}
			return checkStatusWarn, fmt.Sprintf("CPA 服务不可达：%v", err), ""
		}
		return checkStatusPass, fmt.Sprintf("CPA 服务可达，HTTP %d", statusCode), ""
	})

	addCheck("accounts", "账号可用性", func() (string, string, string) {
		accountsList, err := s.getStore().ListAccounts()
		if err != nil {
			return checkStatusFail, fmt.Sprintf("读取账号失败：%v", err), ""
		}
		allowDisabled := s.allowDisabledStudioImageAccounts()
		available := 0
		for _, account := range accountsList {
			if isImageAccountUsable(account, allowDisabled) {
				available++
			}
		}
		if len(accountsList) == 0 {
			return checkStatusFail, "当前账号池为空", "请先导入或创建账号"
		}
		if result.Mode == "studio" && available == 0 {
			return checkStatusFail, fmt.Sprintf("账号总数 %d，可用账号 0", len(accountsList)), "请刷新账号状态或修复代理后重试"
		}
		if available == 0 {
			return checkStatusWarn, fmt.Sprintf("账号总数 %d，可用账号 0", len(accountsList)), ""
		}
		return checkStatusPass, fmt.Sprintf("账号总数 %d，可用账号 %d", len(accountsList), available), ""
	})

	for _, item := range result.Checks {
		switch item.Status {
		case checkStatusPass:
			result.PassCount++
		case checkStatusWarn:
			result.WarnCount++
		case checkStatusFail:
			result.FailCount++
		}
	}

	switch {
	case result.FailCount > 0:
		result.Overall = checkStatusFail
		result.SummaryText = fmt.Sprintf("检测完成：%d 项通过，%d 项警告，%d 项失败", result.PassCount, result.WarnCount, result.FailCount)
	case result.WarnCount > 0:
		result.Overall = checkStatusWarn
		result.SummaryText = fmt.Sprintf("检测完成：%d 项通过，%d 项警告", result.PassCount, result.WarnCount)
	default:
		result.Overall = checkStatusPass
		result.SummaryText = fmt.Sprintf("检测完成：%d 项全部通过", result.PassCount)
	}
	result.FinishedAt = time.Now().Format(time.RFC3339Nano)
	return result
}

func probeEndpoint(parent context.Context, targetURL, proxyURL string, timeout time.Duration) (int, error) {
	transport, err := outboundproxy.NewHTTPTransport(proxyURL)
	if err != nil {
		return 0, err
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "chatgpt-image-studio/diagnostics")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func resolveProxyDialTarget(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	if strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("proxy host is empty")
	}
	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
		case "socks5", "socks5h":
			port = "1080"
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	return net.JoinHostPort(host, port), nil
}

func normalizeProbeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return trimmed
	}
	if strings.Contains(trimmed, "://") {
		return trimmed
	}
	return "http://" + trimmed
}

func (s *Server) maskSensitiveConfig(payload configPayload) configPayload {
	payload.App.APIKey = maskSecret(payload.App.APIKey)
	payload.App.AuthKey = maskSecret(payload.App.AuthKey)
	payload.CPA.APIKey = maskSecret(payload.CPA.APIKey)
	payload.Mail.Password = maskSecret(payload.Mail.Password)
	payload.Sync.ManagementKey = maskSecret(payload.Sync.ManagementKey)
	payload.Proxy.URL = maskURLAuth(payload.Proxy.URL)
	payload.Proxy.ControllerSecret = maskSecret(payload.Proxy.ControllerSecret)
	payload.NewAPI.Password = maskSecret(payload.NewAPI.Password)
	payload.NewAPI.AccessToken = maskSecret(payload.NewAPI.AccessToken)
	payload.NewAPI.SessionCookie = maskSecret(payload.NewAPI.SessionCookie)
	payload.Sub2API.Password = maskSecret(payload.Sub2API.Password)
	payload.Sub2API.APIKey = maskSecret(payload.Sub2API.APIKey)
	return payload
}

func maskSecret(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 6 {
		return "***"
	}
	return trimmed[:3] + strings.Repeat("*", len(trimmed)-6) + trimmed[len(trimmed)-3:]
}

func maskURLAuth(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	if parsed.User == nil {
		return trimmed
	}
	username := parsed.User.Username()
	if username == "" {
		username = "***"
	}
	parsed.User = url.UserPassword(username, "***")
	return parsed.String()
}
