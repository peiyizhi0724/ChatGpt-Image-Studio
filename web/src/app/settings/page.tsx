"use client";

import { useEffect, useMemo, useState } from "react";
import { LoaderCircle, RefreshCcw, RefreshCw, Save, Settings2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { fetchConfig, fetchDefaultConfig, updateConfig, type ConfigPayload } from "@/lib/api";
import { clearCachedSyncStatus } from "@/store/sync-status-cache";
import { APIAccessSection } from "./components/api-access-section";
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
      autoRefreshEnabled: true,
      autoRefreshInterval: 30,
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
  const [config, setConfig] = useState<ConfigPayload>(defaultConfigPayload);
  const [defaultConfig, setDefaultConfig] = useState<ConfigPayload>(defaultConfigPayload);
  const [savedConfig, setSavedConfig] = useState<ConfigPayload | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);

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
            <div className="flex flex-nowrap items-center gap-2 whitespace-nowrap">
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
                disabled={!isDirty || isLoading || isSaving}
              >
                {isSaving ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}
                保存配置
              </Button>
            </div>
          </div>
        </div>

        <ImageModeSection
          config={config}
          isStudioMode={isStudioMode}
          isCPAMode={isCPAMode}
          effectiveCPAImageBaseUrl={effectiveCPAImageBaseUrl}
          syncManagementKeyStatus={syncManagementKeyStatus}
          setSection={setSection}
        />

        <APIAccessSection config={config} setSection={setSection} />

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