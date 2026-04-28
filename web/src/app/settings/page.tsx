"use client";

import { useEffect, useMemo, useState } from "react";
import {
  LoaderCircle,
  RefreshCcw,
  RefreshCw,
  Save,
  Settings2,
  Stethoscope,
} from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  fetchConfig,
  fetchDefaultConfig,
  updateConfig,
  type ConfigPayload,
} from "@/lib/api";
import {
  exportLocalImageConversationsSnapshot,
  exportServerImageConversationsSnapshot,
  importImageConversationsToServerTarget,
  migrateImageConversationStorage,
  setCachedImageConversationStorageMode,
} from "@/store/image-conversations";
import { clearCachedSyncStatus } from "@/store/sync-status-cache";
import { ImageModeSection } from "./components/image-mode-section";
import { IntegrationSection } from "./components/integration-section";
import { RuntimeSection } from "./components/runtime-section";
import { ServicePathsSection } from "./components/service-paths-section";

function joinDisplayPath(root: string, relativePath: string) {
  const normalizedRoot = String(root || "")
    .trim()
    .replace(/[\\/]+$/, "");
  const normalizedRelative = String(relativePath || "")
    .trim()
    .replace(/^[\\/]+/, "");
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
      maxImageConcurrency: 8,
      imageQueueLimit: 32,
      imageQueueTimeoutSeconds: 20,
      imageTaskQueueTtlSeconds: 600,
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
      backend: "current",
      configBackend: "file",
      authDir: "",
      stateFile: "",
      syncStateDir: "",
      imageDir: "",
      imageStorage: "browser",
      imageConversationStorage: "browser",
      imageDataStorage: "browser",
      sqlitePath: "",
      redisAddr: "127.0.0.1:6379",
      redisPassword: "",
      redisDb: 0,
      redisPrefix: "chatgpt2api:studio",
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
    newapi: {
      baseUrl: "",
      username: "",
      password: "",
      accessToken: "",
      userId: 0,
      sessionCookie: "",
      requestTimeout: 20,
    },
    sub2api: {
      baseUrl: "",
      email: "",
      password: "",
      apiKey: "",
      groupId: "",
      requestTimeout: 20,
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

function normalizeConfigPayload(
  payload: Partial<ConfigPayload> | null | undefined,
): ConfigPayload {
  const defaults = defaultConfigPayload();
  const next = payload ?? {};
  const chatgpt = {
    ...defaults.chatgpt,
    ...next.chatgpt,
  };
  const storage = {
    ...defaults.storage,
    ...next.storage,
  };
  if (chatgpt.imageMode !== "studio" && chatgpt.imageMode !== "cpa") {
    chatgpt.imageMode = "studio";
  }
  const legacyImageStorage =
    storage.imageStorage === "server" ? "server" : "browser";
  storage.imageConversationStorage =
    storage.imageConversationStorage === "server"
      ? "server"
      : legacyImageStorage;
  storage.imageDataStorage =
    storage.imageDataStorage === "server"
      ? "server"
      : storage.imageConversationStorage;
  storage.imageStorage = storage.imageConversationStorage;

  return {
    ...defaults,
    ...next,
    app: { ...defaults.app, ...next.app },
    server: { ...defaults.server, ...next.server },
    chatgpt,
    accounts: { ...defaults.accounts, ...next.accounts },
    storage,
    sync: { ...defaults.sync, ...next.sync },
    proxy: { ...defaults.proxy, ...next.proxy },
    cpa: { ...defaults.cpa, ...next.cpa },
    newapi: { ...defaults.newapi, ...next.newapi },
    sub2api: { ...defaults.sub2api, ...next.sub2api },
    log: { ...defaults.log, ...next.log },
    paths: { ...defaults.paths, ...next.paths },
  };
}

export default function SettingsPage() {
  const [config, setConfig] = useState<ConfigPayload>(defaultConfigPayload);
  const [defaultConfig, setDefaultConfig] =
    useState<ConfigPayload>(defaultConfigPayload);
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
    if (
      /^[A-Za-z]:[\\/]/.test(staticDir) ||
      staticDir.startsWith("/") ||
      staticDir.startsWith("\\\\")
    ) {
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
    () =>
      String(config.sync.managementKey || "").trim() ? "已配置" : "未配置",
    [config.sync.managementKey],
  );
  const storageMigrationNotice = useMemo(() => {
    if (!savedConfig) {
      return "";
    }
    const previousConversationStorage =
      savedConfig.storage.imageConversationStorage === "server"
        ? "服务器存储"
        : "浏览器存储";
    const nextConversationStorage =
      config.storage.imageConversationStorage === "server"
        ? "服务器存储"
        : "浏览器存储";
    const previousImageDataStorage =
      savedConfig.storage.imageDataStorage === "server"
        ? "服务器目录"
        : "浏览器 local";
    const nextImageDataStorage =
      config.storage.imageDataStorage === "server"
        ? "服务器目录"
        : "浏览器 local";
    const previousAccountStorage =
      savedConfig.storage.backend === "sqlite"
        ? "SQLite 数据库"
        : savedConfig.storage.backend === "redis"
          ? "Redis"
          : "本地文件";
    const nextAccountStorage =
      config.storage.backend === "sqlite"
        ? "SQLite 数据库"
        : config.storage.backend === "redis"
          ? "Redis"
          : "本地文件";
    const previousConfigStorage =
      savedConfig.storage.configBackend === "redis"
        ? "Redis"
        : "本地 config.toml";
    const nextConfigStorage =
      config.storage.configBackend === "redis" ? "Redis" : "本地 config.toml";

    const messages: string[] = [];
    if (previousAccountStorage !== nextAccountStorage) {
      messages.push(
        `账号池会从${previousAccountStorage}迁移到${nextAccountStorage}。`,
      );
    }
    if (previousConfigStorage !== nextConfigStorage) {
      messages.push(
        `配置文件会从${previousConfigStorage}迁移到${nextConfigStorage}。`,
      );
    }
    if (previousConversationStorage !== nextConversationStorage) {
      messages.push(
        `图片会话记录会从${previousConversationStorage}迁移到${nextConversationStorage}。`,
      );
    }
    if (previousImageDataStorage !== nextImageDataStorage) {
      messages.push(
        `图片数据会从${previousImageDataStorage}迁移到${nextImageDataStorage}。`,
      );
    }
    if (
      savedConfig.storage.imageConversationStorage === "server" &&
      config.storage.imageConversationStorage === "browser"
    ) {
      messages.push(
        "这次迁移需要把服务器图片重新下载回当前浏览器，历史图片较多时会更慢。",
      );
    }
    return messages.join(" ");
  }, [config, savedConfig]);

  const loadConfig = async () => {
    setIsLoading(true);
    try {
      const [currentConfig, defaults] = await Promise.all([
        fetchConfig(),
        fetchDefaultConfig(),
      ]);
      const normalizedConfig = normalizeConfigPayload(currentConfig);
      const normalizedDefaults = normalizeConfigPayload(defaults);
      setCachedImageConversationStorageMode(
        normalizedConfig.storage.imageConversationStorage === "server"
          ? "server"
          : "browser",
      );
      setConfig(normalizedConfig);
      setSavedConfig(normalizedConfig);
      setDefaultConfig(normalizedDefaults);
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
      const previousConfig = savedConfig
        ? normalizeConfigPayload(savedConfig)
        : normalizeConfigPayload(config);
      const previousConversationStorage =
        previousConfig.storage.imageConversationStorage === "server"
          ? "server"
          : "browser";
      const nextConversationStorage =
        config.storage.imageConversationStorage === "server"
          ? "server"
          : "browser";
      const needsServerHistoryMigration =
        (previousConversationStorage === "server" &&
          nextConversationStorage === "server" &&
          previousConfig.storage.backend !== config.storage.backend) ||
        previousConversationStorage !== nextConversationStorage;
      const sourceItems = needsServerHistoryMigration
        ? previousConversationStorage === "server"
          ? await exportServerImageConversationsSnapshot()
          : await exportLocalImageConversationsSnapshot()
        : null;
      if (needsServerHistoryMigration && sourceItems) {
        if (nextConversationStorage === "server") {
          await importImageConversationsToServerTarget(
            sourceItems,
            config.storage,
          );
        } else {
          await migrateImageConversationStorage({
            from: previousConversationStorage,
            to: nextConversationStorage,
            targetImageDataStorage:
              config.storage.imageDataStorage === "server"
                ? "server"
                : "browser",
            sourceItems,
          });
        }
      }
      const result = await updateConfig(config);
      const normalizedConfig = normalizeConfigPayload(result.config);
      const migratedCount = sourceItems?.length ?? 0;
      setCachedImageConversationStorageMode(
        normalizedConfig.storage.imageConversationStorage === "server"
          ? "server"
          : "browser",
      );
      clearCachedSyncStatus();
      setConfig(normalizedConfig);
      setSavedConfig(normalizedConfig);
      const migrationMessages: string[] = [];
      if (previousConfig.storage.backend !== normalizedConfig.storage.backend) {
        migrationMessages.push("账号池已迁移");
      }
      if (
        previousConfig.storage.configBackend !==
        normalizedConfig.storage.configBackend
      ) {
        migrationMessages.push("配置文件已迁移");
      }
      if (migratedCount > 0) {
        migrationMessages.push(`图片会话已迁移 ${migratedCount} 条`);
      }
      toast.success(
        migrationMessages.length > 0
          ? `配置已保存并立即生效：${migrationMessages.join("，")}`
          : "配置已保存并立即生效",
      );
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
    <section className="h-full">
      <div className="hide-scrollbar h-full min-h-0 overflow-y-auto rounded-[30px] border border-stone-200 bg-[#fcfcfb] px-4 pb-5 pt-0 shadow-[0_14px_40px_rgba(15,23,42,0.05)] transition-colors duration-200 dark:border-[var(--studio-border)] dark:bg-[var(--studio-panel)] sm:px-5 sm:pb-6 sm:pt-0 lg:px-6 lg:pb-7 lg:pt-0">
        <div className="sticky top-0 z-20 -mx-4 bg-[#fcfcfb] px-4 pt-5 pb-4 transition-colors duration-200 dark:bg-[var(--studio-panel)] sm:-mx-5 sm:px-5 sm:pt-6 sm:pb-4 lg:-mx-6 lg:px-6 lg:pt-7 lg:pb-5">
          <section className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex items-center gap-4">
              <div className="inline-flex size-12 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
                <Settings2 className="size-5" />
              </div>
              <div className="space-y-1">
                <h1 className="text-2xl font-semibold tracking-tight text-stone-950">
                  配置管理
                </h1>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              <Button
                type="button"
                variant="outline"
                className="h-10 w-full justify-center rounded-full border-stone-200 bg-white px-3 text-[13px] text-stone-700 shadow-none sm:w-auto"
                onClick={() => {
                  window.location.href = "/startup-check";
                }}
                disabled={isLoading || isSaving}
              >
                <Stethoscope className="size-4" />
                启动体检
              </Button>
              <Button
                type="button"
                variant="outline"
                className="h-10 w-full justify-center rounded-full border-stone-200 bg-white px-3 text-[13px] text-stone-700 shadow-none sm:w-auto"
                onClick={() => void loadConfig()}
                disabled={isLoading || isSaving}
              >
                {isLoading ? (
                  <LoaderCircle className="size-4 animate-spin" />
                ) : (
                  <RefreshCw className="size-4" />
                )}
                重新读取
              </Button>
              <Button
                type="button"
                variant="outline"
                className="h-10 w-full justify-center rounded-full border-stone-200 bg-white px-3 text-[13px] text-stone-700 shadow-none sm:w-auto"
                onClick={restoreDefaults}
                disabled={isLoading || isSaving}
              >
                <RefreshCcw className="size-4" />
                恢复默认
              </Button>
              <Button
                type="button"
                className="h-10 w-full justify-center rounded-full bg-stone-950 px-3 text-[13px] text-white hover:bg-stone-800 sm:w-auto"
                onClick={() => void saveConfig()}
                disabled={!isDirty || isLoading || isSaving}
              >
                {isSaving ? (
                  <LoaderCircle className="size-4 animate-spin" />
                ) : (
                  <Save className="size-4" />
                )}
                保存配置
              </Button>
            </div>
          </section>
        </div>

        {storageMigrationNotice ? (
          <div className="mt-1 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900">
            {storageMigrationNotice}
          </div>
        ) : null}

        <div className="mt-5 space-y-5">
          <ImageModeSection
            config={config}
            isStudioMode={isStudioMode}
            isCPAMode={isCPAMode}
            effectiveCPAImageBaseUrl={effectiveCPAImageBaseUrl}
            syncManagementKeyStatus={syncManagementKeyStatus}
            setSection={setSection}
          />

          <IntegrationSection config={config} setSection={setSection} />

          <RuntimeSection config={config} setSection={setSection} />

          <ServicePathsSection
            config={config}
            setConfig={setConfig}
            resolvedStaticDir={resolvedStaticDir}
            startupErrorPath={startupErrorPath}
          />
        </div>
      </div>
    </section>
  );
}
