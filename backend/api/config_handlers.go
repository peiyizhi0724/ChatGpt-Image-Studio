package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"chatgpt2api/internal/config"
	"chatgpt2api/internal/configstore"
)

type configPayload struct {
	App struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		APIKey          string `json:"apiKey"`
		AuthKey         string `json:"authKey"`
		ImageFormat     string `json:"imageFormat"`
		MaxUploadSizeMB int    `json:"maxUploadSizeMB"`
	} `json:"app"`
	Server struct {
		Host                     string `json:"host"`
		Port                     int    `json:"port"`
		StaticDir                string `json:"staticDir"`
		MaxImageConcurrency      int    `json:"maxImageConcurrency"`
		ImageQueueLimit          int    `json:"imageQueueLimit"`
		ImageQueueTimeoutSeconds int    `json:"imageQueueTimeoutSeconds"`
	} `json:"server"`
	ChatGPT struct {
		Model                            string `json:"model"`
		SSETimeout                       int    `json:"sseTimeout"`
		PollInterval                     int    `json:"pollInterval"`
		PollMaxWait                      int    `json:"pollMaxWait"`
		RequestTimeout                   int    `json:"requestTimeout"`
		ImageMode                        string `json:"imageMode"`
		FreeImageRoute                   string `json:"freeImageRoute"`
		FreeImageModel                   string `json:"freeImageModel"`
		PaidImageRoute                   string `json:"paidImageRoute"`
		PaidImageModel                   string `json:"paidImageModel"`
		StudioAllowDisabledImageAccounts bool   `json:"studioAllowDisabledImageAccounts"`
	} `json:"chatgpt"`
	Accounts struct {
		DefaultQuota                int  `json:"defaultQuota"`
		PreferRemoteRefresh         bool `json:"preferRemoteRefresh"`
		RefreshWorkers              int  `json:"refreshWorkers"`
		AutoRefreshEnabled          bool `json:"autoRefreshEnabled"`
		AutoRefreshInterval         int  `json:"autoRefreshInterval"`
		ImageQuotaRefreshTTLSeconds int  `json:"imageQuotaRefreshTTLSeconds"`
	} `json:"accounts"`
	Storage struct {
		Backend                  string `json:"backend"`
		ConfigBackend            string `json:"configBackend"`
		AuthDir                  string `json:"authDir"`
		StateFile                string `json:"stateFile"`
		SyncStateDir             string `json:"syncStateDir"`
		ImageDir                 string `json:"imageDir"`
		ImageStorage             string `json:"imageStorage"`
		ImageConversationStorage string `json:"imageConversationStorage"`
		ImageDataStorage         string `json:"imageDataStorage"`
		SQLitePath               string `json:"sqlitePath"`
		RedisAddr                string `json:"redisAddr"`
		RedisPassword            string `json:"redisPassword"`
		RedisDB                  int    `json:"redisDb"`
		RedisPrefix              string `json:"redisPrefix"`
	} `json:"storage"`
	Sync struct {
		Enabled        bool   `json:"enabled"`
		BaseURL        string `json:"baseUrl"`
		ManagementKey  string `json:"managementKey"`
		RequestTimeout int    `json:"requestTimeout"`
		Concurrency    int    `json:"concurrency"`
		ProviderType   string `json:"providerType"`
	} `json:"sync"`
	Proxy struct {
		Enabled          bool   `json:"enabled"`
		URL              string `json:"url"`
		Mode             string `json:"mode"`
		SyncEnabled      bool   `json:"syncEnabled"`
		AutoRetryEnabled bool   `json:"autoRetryEnabled"`
		ControllerURL    string `json:"controllerUrl"`
		ControllerSecret string `json:"controllerSecret"`
		ControllerGroup  string `json:"controllerGroup"`
	} `json:"proxy"`
	CPA struct {
		BaseURL        string `json:"baseUrl"`
		APIKey         string `json:"apiKey"`
		RequestTimeout int    `json:"requestTimeout"`
		RouteStrategy  string `json:"routeStrategy"`
	} `json:"cpa"`
	Mail struct {
		Enabled        bool   `json:"enabled"`
		SMTPHost       string `json:"smtpHost"`
		SMTPPort       int    `json:"smtpPort"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		FromAddress    string `json:"fromAddress"`
		FromName       string `json:"fromName"`
		UseImplicitTLS bool   `json:"useImplicitTLS"`
		CodeTTLMinutes int    `json:"codeTTLMinutes"`
		ResendInterval int    `json:"resendInterval"`
	} `json:"mail"`
	NewAPI struct {
		BaseURL        string `json:"baseUrl"`
		Username       string `json:"username"`
		Password       string `json:"password"`
		AccessToken    string `json:"accessToken"`
		UserID         int    `json:"userId"`
		SessionCookie  string `json:"sessionCookie"`
		RequestTimeout int    `json:"requestTimeout"`
	} `json:"newapi"`
	Sub2API struct {
		BaseURL        string `json:"baseUrl"`
		Email          string `json:"email"`
		Password       string `json:"password"`
		APIKey         string `json:"apiKey"`
		GroupID        string `json:"groupId"`
		RequestTimeout int    `json:"requestTimeout"`
	} `json:"sub2api"`
	Log struct {
		LogAllRequests bool `json:"logAllRequests"`
	} `json:"log"`
	Paths config.Paths `json:"paths"`
}

type configSaveTarget struct {
	ConfigBackend string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisPrefix   string
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildConfigPayload())
}

func (s *Server) handleGetDefaultConfig(w http.ResponseWriter, r *http.Request) {
	defaultCfg, err := config.LoadDefaults(s.cfg.Paths())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, s.buildConfigPayloadFromConfig(defaultCfg))
}

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var payload configPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	previous := s.buildConfigPayload()

	overrides := map[string]map[string]any{
		"app": {
			"name":               payload.App.Name,
			"version":            payload.App.Version,
			"api_key":            payload.App.APIKey,
			"auth_key":           payload.App.AuthKey,
			"image_format":       payload.App.ImageFormat,
			"max_upload_size_mb": payload.App.MaxUploadSizeMB,
		},
		"server": {
			"host":                        payload.Server.Host,
			"port":                        payload.Server.Port,
			"static_dir":                  payload.Server.StaticDir,
			"max_image_concurrency":       payload.Server.MaxImageConcurrency,
			"image_queue_limit":           payload.Server.ImageQueueLimit,
			"image_queue_timeout_seconds": payload.Server.ImageQueueTimeoutSeconds,
		},
		"chatgpt": {
			"model":                                payload.ChatGPT.Model,
			"sse_timeout":                          payload.ChatGPT.SSETimeout,
			"poll_interval":                        payload.ChatGPT.PollInterval,
			"poll_max_wait":                        payload.ChatGPT.PollMaxWait,
			"request_timeout":                      payload.ChatGPT.RequestTimeout,
			"image_mode":                           payload.ChatGPT.ImageMode,
			"free_image_route":                     payload.ChatGPT.FreeImageRoute,
			"free_image_model":                     payload.ChatGPT.FreeImageModel,
			"paid_image_route":                     payload.ChatGPT.PaidImageRoute,
			"paid_image_model":                     payload.ChatGPT.PaidImageModel,
			"studio_allow_disabled_image_accounts": payload.ChatGPT.StudioAllowDisabledImageAccounts,
		},
		"accounts": {
			"default_quota":                   payload.Accounts.DefaultQuota,
			"prefer_remote_refresh":           payload.Accounts.PreferRemoteRefresh,
			"refresh_workers":                 payload.Accounts.RefreshWorkers,
			"auto_refresh_enabled":            payload.Accounts.AutoRefreshEnabled,
			"auto_refresh_interval_minutes":   payload.Accounts.AutoRefreshInterval,
			"image_quota_refresh_ttl_seconds": payload.Accounts.ImageQuotaRefreshTTLSeconds,
		},
		"storage": {
			"backend":                    payload.Storage.Backend,
			"config_backend":             payload.Storage.ConfigBackend,
			"auth_dir":                   payload.Storage.AuthDir,
			"state_file":                 payload.Storage.StateFile,
			"sync_state_dir":             payload.Storage.SyncStateDir,
			"image_dir":                  payload.Storage.ImageDir,
			"image_storage":              payload.Storage.ImageStorage,
			"image_conversation_storage": payload.Storage.ImageConversationStorage,
			"image_data_storage":         payload.Storage.ImageDataStorage,
			"sqlite_path":                payload.Storage.SQLitePath,
			"redis_addr":                 payload.Storage.RedisAddr,
			"redis_password":             payload.Storage.RedisPassword,
			"redis_db":                   payload.Storage.RedisDB,
			"redis_prefix":               payload.Storage.RedisPrefix,
		},
		"sync": {
			"enabled":         payload.Sync.Enabled,
			"base_url":        payload.Sync.BaseURL,
			"management_key":  payload.Sync.ManagementKey,
			"request_timeout": payload.Sync.RequestTimeout,
			"concurrency":     payload.Sync.Concurrency,
			"provider_type":   payload.Sync.ProviderType,
		},
		"proxy": {
			"enabled":            payload.Proxy.Enabled,
			"url":                payload.Proxy.URL,
			"mode":               payload.Proxy.Mode,
			"sync_enabled":       payload.Proxy.SyncEnabled,
			"auto_retry_enabled": payload.Proxy.AutoRetryEnabled,
			"controller_url":     payload.Proxy.ControllerURL,
			"controller_secret":  payload.Proxy.ControllerSecret,
			"controller_group":   payload.Proxy.ControllerGroup,
		},
		"cpa": {
			"base_url":        payload.CPA.BaseURL,
			"api_key":         payload.CPA.APIKey,
			"request_timeout": payload.CPA.RequestTimeout,
			"route_strategy":  payload.CPA.RouteStrategy,
		},
		"mail": {
			"enabled":                 payload.Mail.Enabled,
			"smtp_host":               payload.Mail.SMTPHost,
			"smtp_port":               payload.Mail.SMTPPort,
			"username":                payload.Mail.Username,
			"password":                payload.Mail.Password,
			"from_address":            payload.Mail.FromAddress,
			"from_name":               payload.Mail.FromName,
			"use_implicit_tls":        payload.Mail.UseImplicitTLS,
			"code_ttl_minutes":        payload.Mail.CodeTTLMinutes,
			"resend_interval_seconds": payload.Mail.ResendInterval,
		},
		"newapi": {
			"base_url":        payload.NewAPI.BaseURL,
			"username":        payload.NewAPI.Username,
			"password":        payload.NewAPI.Password,
			"access_token":    payload.NewAPI.AccessToken,
			"user_id":         payload.NewAPI.UserID,
			"session_cookie":  payload.NewAPI.SessionCookie,
			"request_timeout": payload.NewAPI.RequestTimeout,
		},
		"sub2api": {
			"base_url":        payload.Sub2API.BaseURL,
			"email":           payload.Sub2API.Email,
			"password":        payload.Sub2API.Password,
			"api_key":         payload.Sub2API.APIKey,
			"group_id":        payload.Sub2API.GroupID,
			"request_timeout": payload.Sub2API.RequestTimeout,
		},
		"log": {
			"log_all_requests": payload.Log.LogAllRequests,
		},
	}
	target := configSaveTarget{
		ConfigBackend: payload.Storage.ConfigBackend,
		RedisAddr:     payload.Storage.RedisAddr,
		RedisPassword: payload.Storage.RedisPassword,
		RedisDB:       payload.Storage.RedisDB,
		RedisPrefix:   payload.Storage.RedisPrefix,
	}
	if err := s.saveConfigOverrides(r.Context(), overrides, target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.reloadRuntimeDependencies(previous); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "saved",
		"config": s.buildConfigPayload(),
	})
}

func (s *Server) handleListRequestLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": s.reqLogs.list(100),
	})
}

func (s *Server) buildConfigPayload() configPayload {
	return s.buildConfigPayloadFromConfig(s.cfg)
}

func (s *Server) buildConfigPayloadFromConfig(cfg *config.Config) configPayload {
	payload := configPayload{}
	payload.App.Name = cfg.App.Name
	payload.App.Version = cfg.App.Version
	payload.App.APIKey = cfg.App.APIKey
	payload.App.AuthKey = cfg.App.AuthKey
	payload.App.ImageFormat = cfg.App.ImageFormat
	payload.App.MaxUploadSizeMB = cfg.App.MaxUploadSizeMB

	payload.Server.Host = cfg.Server.Host
	payload.Server.Port = cfg.Server.Port
	payload.Server.StaticDir = cfg.Server.StaticDir
	payload.Server.MaxImageConcurrency = cfg.Server.MaxImageConcurrency
	payload.Server.ImageQueueLimit = cfg.Server.ImageQueueLimit
	payload.Server.ImageQueueTimeoutSeconds = cfg.Server.ImageQueueTimeoutSeconds

	payload.ChatGPT.Model = cfg.ChatGPT.Model
	payload.ChatGPT.SSETimeout = cfg.ChatGPT.SSETimeout
	payload.ChatGPT.PollInterval = cfg.ChatGPT.PollInterval
	payload.ChatGPT.PollMaxWait = cfg.ChatGPT.PollMaxWait
	payload.ChatGPT.RequestTimeout = cfg.ChatGPT.RequestTimeout
	payload.ChatGPT.ImageMode = cfg.ChatGPT.ImageMode
	payload.ChatGPT.FreeImageRoute = cfg.ChatGPT.FreeImageRoute
	payload.ChatGPT.FreeImageModel = cfg.ChatGPT.FreeImageModel
	payload.ChatGPT.PaidImageRoute = cfg.ChatGPT.PaidImageRoute
	payload.ChatGPT.PaidImageModel = cfg.ChatGPT.PaidImageModel
	payload.ChatGPT.StudioAllowDisabledImageAccounts = cfg.ChatGPT.StudioAllowDisabledImageAccounts

	payload.Accounts.DefaultQuota = cfg.Accounts.DefaultQuota
	payload.Accounts.PreferRemoteRefresh = cfg.Accounts.PreferRemoteRefresh
	payload.Accounts.RefreshWorkers = cfg.Accounts.RefreshWorkers
	payload.Accounts.AutoRefreshEnabled = cfg.Accounts.AutoRefreshEnabled
	payload.Accounts.AutoRefreshInterval = cfg.Accounts.AutoRefreshInterval
	payload.Accounts.ImageQuotaRefreshTTLSeconds = cfg.Accounts.ImageQuotaRefreshTTLSeconds

	payload.Storage.Backend = cfg.Storage.Backend
	payload.Storage.ConfigBackend = cfg.Storage.ConfigBackend
	payload.Storage.AuthDir = cfg.Storage.AuthDir
	payload.Storage.StateFile = cfg.Storage.StateFile
	payload.Storage.SyncStateDir = cfg.Storage.SyncStateDir
	payload.Storage.ImageDir = cfg.Storage.ImageDir
	payload.Storage.ImageStorage = cfg.Storage.ImageStorage
	payload.Storage.ImageConversationStorage = cfg.Storage.ImageConversationStorage
	payload.Storage.ImageDataStorage = cfg.Storage.ImageDataStorage
	payload.Storage.SQLitePath = cfg.Storage.SQLitePath
	payload.Storage.RedisAddr = cfg.Storage.RedisAddr
	payload.Storage.RedisPassword = cfg.Storage.RedisPassword
	payload.Storage.RedisDB = cfg.Storage.RedisDB
	payload.Storage.RedisPrefix = cfg.Storage.RedisPrefix

	payload.Sync.Enabled = cfg.Sync.Enabled
	payload.Sync.BaseURL = cfg.Sync.BaseURL
	payload.Sync.ManagementKey = cfg.Sync.ManagementKey
	payload.Sync.RequestTimeout = cfg.Sync.RequestTimeout
	payload.Sync.Concurrency = cfg.Sync.Concurrency
	payload.Sync.ProviderType = cfg.Sync.ProviderType

	payload.Proxy.Enabled = cfg.Proxy.Enabled
	payload.Proxy.URL = cfg.Proxy.URL
	payload.Proxy.Mode = cfg.Proxy.Mode
	payload.Proxy.SyncEnabled = cfg.Proxy.SyncEnabled
	payload.Proxy.AutoRetryEnabled = cfg.Proxy.AutoRetryEnabled
	payload.Proxy.ControllerURL = cfg.Proxy.ControllerURL
	payload.Proxy.ControllerSecret = cfg.Proxy.ControllerSecret
	payload.Proxy.ControllerGroup = cfg.Proxy.ControllerGroup

	payload.CPA.BaseURL = cfg.CPA.BaseURL
	payload.CPA.APIKey = cfg.CPA.APIKey
	payload.CPA.RequestTimeout = cfg.CPA.RequestTimeout
	payload.CPA.RouteStrategy = cfg.CPA.RouteStrategy

	payload.Mail.Enabled = cfg.Mail.Enabled
	payload.Mail.SMTPHost = cfg.Mail.SMTPHost
	payload.Mail.SMTPPort = cfg.Mail.SMTPPort
	payload.Mail.Username = cfg.Mail.Username
	payload.Mail.Password = cfg.Mail.Password
	payload.Mail.FromAddress = cfg.Mail.FromAddress
	payload.Mail.FromName = cfg.Mail.FromName
	payload.Mail.UseImplicitTLS = cfg.Mail.UseImplicitTLS
	payload.Mail.CodeTTLMinutes = cfg.Mail.CodeTTLMinutes
	payload.Mail.ResendInterval = cfg.Mail.ResendInterval

	payload.NewAPI.BaseURL = cfg.NewAPI.BaseURL
	payload.NewAPI.Username = cfg.NewAPI.Username
	payload.NewAPI.Password = cfg.NewAPI.Password
	payload.NewAPI.AccessToken = cfg.NewAPI.AccessToken
	payload.NewAPI.UserID = cfg.NewAPI.UserID
	payload.NewAPI.SessionCookie = cfg.NewAPI.SessionCookie
	payload.NewAPI.RequestTimeout = cfg.NewAPI.RequestTimeout

	payload.Sub2API.BaseURL = cfg.Sub2API.BaseURL
	payload.Sub2API.Email = cfg.Sub2API.Email
	payload.Sub2API.Password = cfg.Sub2API.Password
	payload.Sub2API.APIKey = cfg.Sub2API.APIKey
	payload.Sub2API.GroupID = cfg.Sub2API.GroupID
	payload.Sub2API.RequestTimeout = cfg.Sub2API.RequestTimeout

	payload.Log.LogAllRequests = cfg.Log.LogAllRequests
	payload.Paths = s.cfg.Paths()
	return payload
}

func (s *Server) saveConfigOverrides(ctx context.Context, values map[string]map[string]any, target configSaveTarget) error {
	effective := resolveConfigSaveTarget(s.cfg, target)
	if strings.EqualFold(strings.TrimSpace(effective.ConfigBackend), "redis") {
		if strings.TrimSpace(effective.RedisAddr) == "" {
			return fmt.Errorf("redis_addr is required when storage.config_backend = redis")
		}
		store := configstore.NewRedis(
			effective.RedisAddr,
			effective.RedisPassword,
			effective.RedisDB,
			effective.RedisPrefix,
		)
		defer store.Close()
		if err := store.Save(ctx, values); err != nil {
			return err
		}
		bootstrapOverrides := map[string]map[string]any{
			"storage": {
				"config_backend": effective.ConfigBackend,
				"redis_addr":     effective.RedisAddr,
				"redis_password": effective.RedisPassword,
				"redis_db":       effective.RedisDB,
				"redis_prefix":   effective.RedisPrefix,
			},
		}
		if err := s.cfg.PersistOverrideFile(bootstrapOverrides); err != nil {
			return err
		}
		return s.cfg.ApplyOverrides(values)
	}
	return s.cfg.SaveOverrides(values)
}

func resolveConfigSaveTarget(current *config.Config, target configSaveTarget) configSaveTarget {
	result := configSaveTarget{
		ConfigBackend: target.ConfigBackend,
		RedisAddr:     target.RedisAddr,
		RedisPassword: target.RedisPassword,
		RedisDB:       target.RedisDB,
		RedisPrefix:   target.RedisPrefix,
	}
	if strings.TrimSpace(result.ConfigBackend) == "" {
		result.ConfigBackend = current.Storage.ConfigBackend
	}
	return result
}
