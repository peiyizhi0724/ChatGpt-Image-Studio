package api

import (
	"encoding/json"
	"net/http"

	"chatgpt2api/internal/config"
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
		Host      string `json:"host"`
		Port      int    `json:"port"`
		StaticDir string `json:"staticDir"`
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
		DefaultQuota        int  `json:"defaultQuota"`
		PreferRemoteRefresh bool `json:"preferRemoteRefresh"`
		RefreshWorkers      int  `json:"refreshWorkers"`
		AutoRefreshEnabled  bool `json:"autoRefreshEnabled"`
		AutoRefreshInterval int  `json:"autoRefreshInterval"`
	} `json:"accounts"`
	Storage struct {
		AuthDir      string `json:"authDir"`
		StateFile    string `json:"stateFile"`
		SyncStateDir string `json:"syncStateDir"`
		ImageDir     string `json:"imageDir"`
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
		Enabled     bool   `json:"enabled"`
		URL         string `json:"url"`
		Mode        string `json:"mode"`
		SyncEnabled bool   `json:"syncEnabled"`
	} `json:"proxy"`
	CPA struct {
		BaseURL        string `json:"baseUrl"`
		APIKey         string `json:"apiKey"`
		RequestTimeout int    `json:"requestTimeout"`
		RouteStrategy  string `json:"routeStrategy"`
	} `json:"cpa"`
	Log struct {
		LogAllRequests bool `json:"logAllRequests"`
	} `json:"log"`
	Paths config.Paths `json:"paths"`
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

	if err := s.cfg.SaveOverrides(map[string]map[string]any{
		"app": {
			"name":               payload.App.Name,
			"version":            payload.App.Version,
			"api_key":            payload.App.APIKey,
			"auth_key":           payload.App.AuthKey,
			"image_format":       payload.App.ImageFormat,
			"max_upload_size_mb": payload.App.MaxUploadSizeMB,
		},
		"server": {
			"host":       payload.Server.Host,
			"port":       payload.Server.Port,
			"static_dir": payload.Server.StaticDir,
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
			"default_quota":                 payload.Accounts.DefaultQuota,
			"prefer_remote_refresh":         payload.Accounts.PreferRemoteRefresh,
			"refresh_workers":               payload.Accounts.RefreshWorkers,
			"auto_refresh_enabled":          payload.Accounts.AutoRefreshEnabled,
			"auto_refresh_interval_minutes": payload.Accounts.AutoRefreshInterval,
		},
		"storage": {
			"auth_dir":       payload.Storage.AuthDir,
			"state_file":     payload.Storage.StateFile,
			"sync_state_dir": payload.Storage.SyncStateDir,
			"image_dir":      payload.Storage.ImageDir,
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
			"enabled":      payload.Proxy.Enabled,
			"url":          payload.Proxy.URL,
			"mode":         payload.Proxy.Mode,
			"sync_enabled": payload.Proxy.SyncEnabled,
		},
		"cpa": {
			"base_url":        payload.CPA.BaseURL,
			"api_key":         payload.CPA.APIKey,
			"request_timeout": payload.CPA.RequestTimeout,
			"route_strategy":  payload.CPA.RouteStrategy,
		},
		"log": {
			"log_all_requests": payload.Log.LogAllRequests,
		},
	}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
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
	payload.App.Version = s.cfg.App.Version
	payload.App.APIKey = cfg.App.APIKey
	payload.App.AuthKey = cfg.App.AuthKey
	payload.App.ImageFormat = cfg.App.ImageFormat
	payload.App.MaxUploadSizeMB = cfg.App.MaxUploadSizeMB

	payload.Server.Host = cfg.Server.Host
	payload.Server.Port = cfg.Server.Port
	payload.Server.StaticDir = cfg.Server.StaticDir

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

	payload.Storage.AuthDir = cfg.Storage.AuthDir
	payload.Storage.StateFile = cfg.Storage.StateFile
	payload.Storage.SyncStateDir = cfg.Storage.SyncStateDir
	payload.Storage.ImageDir = cfg.Storage.ImageDir

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

	payload.CPA.BaseURL = cfg.CPA.BaseURL
	payload.CPA.APIKey = cfg.CPA.APIKey
	payload.CPA.RequestTimeout = cfg.CPA.RequestTimeout
	payload.CPA.RouteStrategy = cfg.CPA.RouteStrategy

	payload.Log.LogAllRequests = cfg.Log.LogAllRequests
	payload.Paths = s.cfg.Paths()
	return payload
}