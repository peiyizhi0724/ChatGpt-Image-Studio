"use client";

import { useEffect, useMemo, useState } from "react";
import { Download, LoaderCircle, RefreshCcw, RefreshCw, Save, Settings2, ShieldCheck } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { downloadDiagnosticsExport, fetchConfig, fetchDefaultConfig, updateConfig, type ConfigPayload } from "@/lib/api";
import { clearCachedSyncStatus } from "@/store/sync-status-cache";
import { ImageModeSection } from "./components/image-mode-section";
import { RuntimeSection } from "./components/runtime-section";
import { ServicePathsSection } from "./components/service-paths-section";

function joinDisplayPath(root: string, relativePath: string) {
  const normalizedRoot = String(root || "").trim().replace(/[\\/]+$/, "");
  const normalizedRelative = String(relativePath || "").trim().replace(/^[\\/]+/, "");
  if (!normalizedRoot) {
    return normalizedRelative;
  }
  if (!normalizedRelative) {
    return normalizedRoot;
  }
  const separator = normalizedRoot.includes("\\") ? "\\" : "/";
  return `${normalizedRoot}${separator}${normalizedRelative.replace(/[\\/]+/g, separator)}`;
}

function firstNonEmptyValue(...values: Array<string | null | undefined>) {
  for (const value of values) {
    const trimmed = String(value || "").trim();
    if (trimmed) {
      return trimmed;
    }
  }
  return "";
}

function containsFullWidthComma(value: string) {
  return /[，]/.test(String(value || ""));
}

function looksLikeURL(value: string) {
  return /^(https?:\/\/|socks5h?:\/\/)/i.test(String(value || "").trim());
}

function validateConfigInput(config: ConfigPayload, effectiveCPAImageBaseUrl: string) {
  const errors: string[] = [];
  const warnings: string[] = [];

  if (!Number.isFinite(config.server.port) || config.server.port < 1 || config.server.port > 65535) {
    errors.push("监听端口必须在 1-65535 之间。");
  }
  if (config.server.maxImageConcurrency < 1) {
    errors.push("全局并发上限必须大于 0。");
  }
  if (config.server.imageQueueLimit < 1) {
    errors.push("排队长度上限必须大于 0。");
  }
  if (config.server.imageQueueTimeoutSeconds < 1) {
    errors.push("排队超时必须大于 0 秒。");
  }

  if (config.proxy.enabled) {
    if (!config.proxy.url.trim()) {
      errors.push("已启用代理，但 proxy.url 为空。");
    } else {
      if (containsFullWidthComma(config.proxy.url)) {
        errors.push("proxy.url 含有中文逗号，请改为英文符号。");
      }
      if (!looksLikeURL(config.proxy.url)) {
        errors.push("proxy.url 必须以 http://、https://、socks5:// 或 socks5h:// 开头。");
      }
    }
  }

  if (config.chatgpt.imageMode === "cpa") {
    if (!effectiveCPAImageBaseUrl.trim()) {
      errors.push("CPA 模式下必须配置 cpa.baseUrl 或 sync.baseUrl。");
    }
    if (!config.cpa.apiKey.trim()) {
      errors.push("CPA 模式下 cpa.apiKey 不能为空。");
    }
    if (containsFullWidthComma(config.cpa.apiKey)) {
      errors.push("cpa.apiKey 含有中文逗号，请改为英文符号。");
    }
  }

  if (config.sync.enabled) {
    if (!config.sync.baseUrl.trim()) {
      errors.push("已启用同步，但 sync.baseUrl 为空。");
    }
    if (!config.sync.managementKey.trim()) {
      errors.push("已启用同步，但 sync.managementKey 为空。");
    }
  }

  if (!config.app.authKey.trim()) {
    warnings.push("UI 登录密钥为空，管理接口可能处于无保护状态。");
  }
  if (containsFullWidthComma(config.sync.baseUrl) || containsFullWidthComma(config.cpa.baseUrl)) {
    errors.push("检测到 Base URL 中含有中文逗号，请改为英文符号。");
  }

  return { errors, warnings };
}

function defaultConfigPayload(): ConfigPayload {
  return {
    app: {
      name: "",
      version: "",
      apiKey: "",
      authKey: "",
      imageFormat: "url",
      maxUploadSizeMB: 50,
    },
    server: {
      host: "",
      port: 7000,
      staticDir: "",
      maxImageConcurrency: 8,
      imageQueueLimit: 32,
      imageQueueTimeoutSeconds: 20,
    },
    chatgpt: {
      model: "gpt-image-2",
      sseTimeout: 600,
      pollInterval: 3,
      pollMaxWait: 600,
      requestTimeout: 120,
      imageMode: "studio",
      freeImageRoute: "legacy",
      freeImageModel: "auto",
      paidImageRoute: "responses",
      paidImageModel: "gpt-5.4-mini",
      studioAllowDisabledImageAccounts: false,
    },
    accounts: {
      defaultQuota: 5,
      preferRemoteRefresh: true,
      refreshWorkers: 6,
      imageQuotaRefreshTTLSeconds: 120,
    },
    storage: {
      authDir: "",
      stateFile: "",
      syncStateDir: "",
      imageDir: "",
    },
    sync: {
      enabled: false,
      baseUrl: "",
      managementKey: "",
      requestTimeout: 20,
      concurrency: 4,
      providerType: "codex",
    },
    proxy: {
      enabled: false,
      url: "socks5h://127.0.0.1:10808",
      mode: "fixed",
      syncEnabled: false,
    },
    cpa: {
      baseUrl: "",
      apiKey: "",
      requestTimeout: 60,
      routeStrategy: "images_api",
    },
    log: {
      logAllRequests: false,
    },
    paths: {
      root: "",
      defaults: "",
      override: "",
    },
  };
}

export default function SettingsPage() {
  const navigate = useNavigate();
  const [config, setConfig] = useState<ConfigPayload>(defaultConfigPayload);
  const [defaultConfig, setDefaultConfig] = useState<ConfigPayload>(defaultConfigPayload);
  const [savedConfig, setSavedConfig] = useState<ConfigPayload | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isDownloadingDiagnostics, setIsDownloadingDiagnostics] = useState(false);

  const isStudioMode = config.chatgpt.imageMode === "studio";
  const isCPAMode = config.chatgpt.imageMode === "cpa";

  const isDirty = useMemo(() => {
    if (!savedConfig) {
      return false;
    }
    return JSON.stringify(config) !== JSON.stringify(savedConfig);
  }, [config, savedConfig]);

  const resolvedStaticDir = useMemo(() => {
    const staticDir = String(config.server.staticDir || "").trim();
    if (!staticDir) {
      return "";
    }
    if (/^[A-Za-z]:[\\/]/.test(staticDir) || staticDir.startsWith("/") || staticDir.startsWith("\\\\")) {
      return staticDir;
    }
    return joinDisplayPath(config.paths.root, staticDir);
  }, [config.paths.root, config.server.staticDir]);

  const startupErrorPath = useMemo(
    () => joinDisplayPath(config.paths.root, "data/last-startup-error.txt"),
    [config.paths.root],
  );
  const effectiveCPAImageBaseUrl = useMemo(
    () => firstNonEmptyValue(config.cpa.baseUrl, config.sync.baseUrl),
    [config.cpa.baseUrl, config.sync.baseUrl],
  );
  const syncManagementKeyStatus = useMemo(
    () => (String(config.sync.managementKey || "").trim() ? "已配置" : "未配置"),
    [config.sync.managementKey],
  );

  const validation = useMemo(
    () => validateConfigInput(config, effectiveCPAImageBaseUrl),
    [config, effectiveCPAImageBaseUrl],
  );

  const loadConfig = async () => {
    setIsLoading(true);
    try {
      const [currentConfig, defaults] = await Promise.all([fetchConfig(), fetchDefaultConfig()]);
      setConfig(currentConfig);
      setSavedConfig(currentConfig);
      setDefaultConfig(defaults);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取配置失败");
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadConfig();
  }, []);

  const setSection = <K extends keyof ConfigPayload>(
    section: K,
    nextValue: ConfigPayload[K],
  ) => {
    setConfig((current) => ({
      ...current,
      [section]: nextValue,
    }));
  };

  const saveConfig = async () => {
    if (validation.errors.length > 0) {
      toast.error("配置存在错误，请先修正后再保存");
      return;
    }
    setIsSaving(true);
    try {
      const result = await updateConfig(config);
      clearCachedSyncStatus();
      setConfig(result.config);
      setSavedConfig(result.config);
      toast.success("配置已保存并立即生效");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存配置失败");
    } finally {
      setIsSaving(false);
    }
  };

  const exportDiagnostics = async () => {
    setIsDownloadingDiagnostics(true);
    try {
      const { blob, fileName } = await downloadDiagnosticsExport();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = fileName;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(url);
      toast.success("诊断包已下载");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "诊断包下载失败");
    } finally {
      setIsDownloadingDiagnostics(false);
    }
  };

  const restoreDefaults = () => {
    setConfig(defaultConfig);
    toast.success("已恢复为默认配置草稿，点击“保存配置”后才会真正生效");
  };

  return (
    <section className="h-full overflow-y-auto">
      <div className="mx-auto flex max-w-[1440px] flex-col gap-6 px-1 py-1">
        <div className="rounded-[30px] border border-stone-200 bg-white px-5 py-5 shadow-[0_14px_40px_rgba(15,23,42,0.05)] sm:px-6">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0">
              <div className="flex items-start gap-4">
                <div className="inline-flex size-12 shrink-0 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
                  <Settings2 className="size-5" />
                </div>
                <div className="min-w-0">
                  <h1 className="text-2xl font-semibold tracking-tight text-stone-950">配置管理</h1>
                  <p className="mt-2 max-w-[820px] text-sm leading-7 text-stone-500">
                    所有字段都先在页面本地编辑，只有点击“保存配置”后才会写入
                    <span className="mx-1 rounded bg-stone-100 px-1.5 py-0.5 text-stone-700">data/config.toml</span>
                    并立即在后端生效。发布版默认以
                    <span className="mx-1 rounded bg-stone-100 px-1.5 py-0.5 text-stone-700">可执行文件所在目录</span>
                    作为配置根目录。
                  </p>
                </div>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-full border-stone-200 bg-white px-3 text-[13px] text-stone-700 shadow-none"
                onClick={() => navigate("/startup-check")}
                disabled={isLoading || isSaving}
              >
                <ShieldCheck className="size-4" />
                启动体检
              </Button>
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-full border-stone-200 bg-white px-3 text-[13px] text-stone-700 shadow-none"
                onClick={() => void exportDiagnostics()}
                disabled={isLoading || isSaving || isDownloadingDiagnostics}
              >
                {isDownloadingDiagnostics ? <LoaderCircle className="size-4 animate-spin" /> : <Download className="size-4" />}
                导出诊断包
              </Button>
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-full border-stone-200 bg-white px-3 text-[13px] text-stone-700 shadow-none"
                onClick={() => void loadConfig()}
                disabled={isLoading || isSaving}
              >
                {isLoading ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                重新读取
              </Button>
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-full border-stone-200 bg-white px-3 text-[13px] text-stone-700 shadow-none"
                onClick={restoreDefaults}
                disabled={isLoading || isSaving}
              >
                <RefreshCcw className="size-4" />
                恢复默认
              </Button>
              <Button
                type="button"
                className="h-10 rounded-full bg-stone-950 px-3 text-[13px] text-white hover:bg-stone-800"
                onClick={() => void saveConfig()}
                disabled={!isDirty || isLoading || isSaving || validation.errors.length > 0}
              >
                {isSaving ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}
                保存配置
              </Button>
            </div>
          </div>
        </div>

        {validation.errors.length > 0 || validation.warnings.length > 0 ? (
          <div className="rounded-[24px] border border-amber-200 bg-amber-50 px-5 py-4 text-sm text-amber-900">
            <div className="font-semibold">实时校验</div>
            {validation.errors.length > 0 ? (
              <ul className="mt-2 list-disc space-y-1 pl-5">
                {validation.errors.map((message) => (
                  <li key={`error-${message}`}>{message}</li>
                ))}
              </ul>
            ) : null}
            {validation.warnings.length > 0 ? (
              <ul className="mt-2 list-disc space-y-1 pl-5 text-amber-800">
                {validation.warnings.map((message) => (
                  <li key={`warning-${message}`}>{message}</li>
                ))}
              </ul>
            ) : null}
          </div>
        ) : null}

        <ImageModeSection
          config={config}
          isStudioMode={isStudioMode}
          isCPAMode={isCPAMode}
          effectiveCPAImageBaseUrl={effectiveCPAImageBaseUrl}
          syncManagementKeyStatus={syncManagementKeyStatus}
          setSection={setSection}
        />

        <RuntimeSection config={config} setSection={setSection} />

        <ServicePathsSection
          config={config}
          setConfig={setConfig}
          resolvedStaticDir={resolvedStaticDir}
          startupErrorPath={startupErrorPath}
        />
      </div>
    </section>
  );
}
