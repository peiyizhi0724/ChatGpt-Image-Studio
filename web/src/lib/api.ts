import { httpRequest } from "@/lib/request";
import webConfig from "@/constants/common-env";
import { getStoredAuthKey } from "@/store/auth";
import { buildImageAccountPolicyHeader } from "@/store/image-account-policy";

export type AccountType = "Free" | "Plus" | "Pro" | "Team";
export type AccountStatus = "正常" | "限流" | "异常" | "禁用";
export type SyncStatus =
  | "synced"
  | "pending_upload"
  | "remote_only"
  | "remote_deleted";
export type SyncSource = "cpa" | "newapi" | "sub2api";
export type AccountSourceKind = "auth_file" | "token";
export type ImageModel = "gpt-image-1" | "gpt-image-2";
export type ImageQuality = "low" | "medium" | "high";
export type ImageResponseItem = {
  url?: string;
  b64_json?: string;
  revised_prompt?: string;
  file_id?: string;
  gen_id?: string;
  conversation_id?: string;
  parent_message_id?: string;
  source_account_id?: string;
};

export type InpaintSourceReference = {
  original_file_id: string;
  original_gen_id: string;
  conversation_id?: string;
  parent_message_id?: string;
  source_account_id: string;
};

export type Account = {
  id: string;
  fileName: string;
  access_token: string;
  sourceKind?: AccountSourceKind | null;
  type: AccountType;
  status: AccountStatus;
  quota: number;
  email?: string | null;
  user_id?: string | null;
  limits_progress?: Array<{
    feature_name?: string;
    remaining?: number;
    reset_after?: string;
  }>;
  default_model_slug?: string | null;
  restoreAt?: string | null;
  success: number;
  fail: number;
  lastUsedAt: string | null;
  provider?: string;
  disabled?: boolean;
  note?: string | null;
  priority?: number;
  syncStatus?: SyncStatus | null;
  syncOrigin?: string | null;
  lastSyncedAt?: string | null;
  remoteDisabled?: boolean | null;
  importedAt?: string | null;
};

export type SyncAccount = {
  name: string;
  status: SyncStatus;
  location: "local" | "remote" | "both";
  localDisabled?: boolean | null;
  remoteDisabled?: boolean | null;
};

export type SyncRunResult = {
  ok: boolean;
  running?: boolean;
  source: SyncSource;
  error?: string;
  direction?: string;
  imported: number;
  exported: number;
  skipped: number;
  failed: number;
  inaccessible: number;
  total?: number;
  processed?: number;
  phase?: string;
  current?: string;
  notes?: string[];
  started_at: string;
  finished_at: string;
  updated_at?: string;
};

export type SyncStatusResponse = {
  source: SyncSource;
  label: string;
  configured: boolean;
  pullSupported: boolean;
  pushSupported: boolean;
  local: number;
  remote: number;
  pendingPush: number;
  pendingPull: number;
  inaccessibleRemote: number;
  notes?: string[];
  lastRun?: SyncRunResult | null;
};

type AccountListResponse = {
  items: Account[];
};

type AccountMutationResponse = {
  items: Account[];
  added?: number;
  skipped?: number;
  removed?: number;
  refreshed?: number;
  errors?: Array<{ access_token: string; error: string }>;
};

export type AccountImportResponse = {
  items: Account[];
  imported?: number;
  imported_files?: number;
  refreshed?: number;
  errors?: Array<{ access_token: string; error: string }>;
  duplicates?: Array<{ name: string; reason: string }>;
  failed?: Array<{ name: string; error: string }>;
};

type AccountRefreshResponse = {
  items: Account[];
  refreshed: number;
  errors: Array<{ access_token: string; error: string }>;
};

export type AccountRefreshProgress = {
  ok: boolean;
  running: boolean;
  error?: string;
  total: number;
  processed: number;
  refreshed: number;
  failed: number;
  current?: string;
  started_at: string;
  finished_at: string;
  updated_at?: string;
};

type AccountRefreshAllResponse = {
  progress: AccountRefreshProgress | null;
  alreadyRunning?: boolean;
};

type AccountUpdateResponse = {
  item: Account;
  items: Account[];
};

export type AccountQuotaResponse = {
  id: string;
  email?: string | null;
  status: AccountStatus;
  type: AccountType;
  quota: number;
  image_gen_remaining?: number | null;
  image_gen_reset_after?: string | null;
  refresh_requested: boolean;
  refreshed: boolean;
  refresh_error?: string;
};

export type ImageMode = "studio" | "cpa";

type ImageResponse = {
  created: number;
  data: ImageResponseItem[];
};

export type ConfigPayload = {
  app: {
    name: string;
    version: string;
    apiKey: string;
    authKey: string;
    imageFormat: string;
    maxUploadSizeMB: number;
  };
  server: {
    host: string;
    port: number;
    staticDir: string;
    maxImageConcurrency: number;
    imageQueueLimit: number;
    imageQueueTimeoutSeconds: number;
  };
  chatgpt: {
    model: string;
    sseTimeout: number;
    pollInterval: number;
    pollMaxWait: number;
    requestTimeout: number;
    imageMode: ImageMode;
    freeImageRoute: string;
    freeImageModel: string;
    paidImageRoute: string;
    paidImageModel: string;
    studioAllowDisabledImageAccounts: boolean;
  };
  accounts: {
    defaultQuota: number;
    preferRemoteRefresh: boolean;
    refreshWorkers: number;
    autoRefreshEnabled: boolean;
    autoRefreshInterval: number;
    imageQuotaRefreshTTLSeconds: number;
  };
  storage: {
    backend: string;
    configBackend: "file" | "redis" | string;
    authDir: string;
    stateFile: string;
    syncStateDir: string;
    imageDir: string;
    imageStorage: "browser" | "server" | string;
    imageConversationStorage: "browser" | "server" | string;
    imageDataStorage: "browser" | "server" | string;
    sqlitePath: string;
    redisAddr: string;
    redisPassword: string;
    redisDb: number;
    redisPrefix: string;
  };
  sync: {
    enabled: boolean;
    baseUrl: string;
    managementKey: string;
    requestTimeout: number;
    concurrency: number;
    providerType: string;
  };
  proxy: {
    enabled: boolean;
    url: string;
    mode: string;
    syncEnabled: boolean;
    autoRetryEnabled: boolean;
    controllerUrl: string;
    controllerSecret: string;
    controllerGroup: string;
  };
  cpa: {
    baseUrl: string;
    apiKey: string;
    requestTimeout: number;
    routeStrategy: "images_api" | "codex_responses" | "auto";
  };
  mail: {
    enabled: boolean;
    smtpHost: string;
    smtpPort: number;
    username: string;
    password: string;
    fromAddress: string;
    fromName: string;
    useImplicitTLS: boolean;
    codeTTLMinutes: number;
    resendInterval: number;
  };
  newapi: {
    baseUrl: string;
    username: string;
    password: string;
    accessToken: string;
    userId: number;
    sessionCookie: string;
    requestTimeout: number;
  };
  sub2api: {
    baseUrl: string;
    email: string;
    password: string;
    apiKey: string;
    groupId: string;
    requestTimeout: number;
  };
  log: {
    logAllRequests: boolean;
  };
  paths: {
    root: string;
    defaults: string;
    override: string;
  };
};

export type RequestLogItem = {
  id: string;
  startedAt: string;
  finishedAt: string;
  endpoint: string;
  operation: string;
  imageMode: ImageMode | string;
  direction: "official" | "cpa" | string;
  route: string;
  cpaSubroute?: "images_api" | "codex_responses" | "auto" | string;
  queueWaitMs?: number;
  inflightCountAtStart?: number;
  leaseAcquired?: boolean;
  errorCode?: string;
  routingPolicyApplied?: boolean;
  routingGroupIndex?: number;
  routingSortMode?: string;
  routingReservePercent?: number;
  accountType?: string;
  accountEmail?: string;
  accountFile?: string;
  requestedModel?: string;
  upstreamModel?: string;
  imageToolModel?: string;
  size?: string;
  quality?: string;
  promptLength?: number;
  preferred: boolean;
  success: boolean;
  error?: string;
};

export type VersionInfo = {
  version: string;
  commit?: string;
  buildTime?: string;
};

export type StartupCheckItem = {
  key: string;
  label: string;
  status: "pass" | "warn" | "fail" | string;
  detail: string;
  hint?: string;
  durationMs: number;
};

export type StartupCheckResponse = {
  startedAt: string;
  finishedAt: string;
  mode: "studio" | "cpa" | string;
  overall: "pass" | "warn" | "fail" | string;
  passCount: number;
  warnCount: number;
  failCount: number;
  checks: StartupCheckItem[];
  summaryText: string;
};

export type RuntimeStatusResponse = {
  timestamp: string;
  mode: "studio" | "cpa" | string;
  admission: {
    maxConcurrency: number;
    queueLimit: number;
    queueTimeoutMs: number;
    inflight: number;
    queued: number;
  };
  accounts: {
    total: number;
    available: number;
    availablePaid: number;
  };
  recent: {
    windowSeconds: number;
    failureCount: number;
    lastError?: string;
    lastErrorCode?: string;
    lastErrorAt?: string;
    lastErrorAccount?: string;
  };
};

export type ProxyTestResult = {
  ok: boolean;
  status: number;
  latency: number;
  error?: string;
};

export type IntegrationTestResult = {
  ok: boolean;
  source: SyncSource | "cpa";
  message: string;
  status: number;
  latency: number;
  userId?: number;
  username?: string;
  email?: string;
  groupCount?: number;
};

export type NewAPITokenDiscoverResult = {
  ok: boolean;
  message: string;
  latency: number;
  accessToken?: string;
  userId?: number;
};

export type Sub2APIGroupOption = {
  id: string;
  name: string;
  description: string;
  platform: string;
  status: string;
};

export type Sub2APIGroupsResult = {
  ok: boolean;
  message: string;
  latency: number;
  groups: Sub2APIGroupOption[];
};

export async function login(authKey: string) {
  const normalizedAuthKey = String(authKey || "").trim();
  return httpRequest<{ ok: boolean }>("/auth/login", {
    method: "POST",
    body: {},
    headers: {
      Authorization: `Bearer ${normalizedAuthKey}`,
    },
    redirectOnUnauthorized: false,
  });
}

export async function fetchAccounts() {
  return httpRequest<AccountListResponse>("/api/accounts");
}

export async function createAccounts(tokens: string[]) {
  return httpRequest<AccountMutationResponse>("/api/accounts", {
    method: "POST",
    body: { tokens },
  });
}

export async function importAccountFiles(files: File[]) {
  const formData = new FormData();
  files.forEach((file) => formData.append("file", file));
  return httpRequest<AccountImportResponse>("/api/accounts/import", {
    method: "POST",
    body: formData,
  });
}

export async function deleteAccounts(tokens: string[]) {
  return httpRequest<AccountMutationResponse>("/api/accounts", {
    method: "DELETE",
    body: { tokens },
  });
}

export async function refreshAccounts(accessTokens: string[]) {
  return httpRequest<AccountRefreshResponse>("/api/accounts/refresh", {
    method: "POST",
    body: { access_tokens: accessTokens },
  });
}

export async function refreshAllAccounts() {
  return httpRequest<AccountRefreshAllResponse>("/api/accounts/refresh-all", {
    method: "POST",
    body: {},
  });
}

export async function fetchAccountRefreshProgress() {
  return httpRequest<AccountRefreshAllResponse>(
    "/api/accounts/refresh-progress",
  );
}

export async function updateAccount(
  accessToken: string,
  updates: {
    type?: AccountType;
    status?: AccountStatus;
    quota?: number;
    note?: string;
  },
) {
  return httpRequest<AccountUpdateResponse>("/api/accounts/update", {
    method: "POST",
    body: {
      access_token: accessToken,
      ...updates,
    },
  });
}

export async function fetchAccountQuota(
  accountId: string,
  options: { refresh?: boolean } = {},
) {
  const refresh = options.refresh ?? true;
  const suffix = refresh ? "" : "?refresh=false";
  return httpRequest<AccountQuotaResponse>(
    `/api/accounts/${encodeURIComponent(accountId)}/quota${suffix}`,
  );
}

export async function fetchSyncStatus(
  source: SyncSource = "cpa",
  options: { progressOnly?: boolean } = {},
) {
  const params = new URLSearchParams({ source });
  if (options.progressOnly) {
    params.set("progress_only", "1");
  }
  return httpRequest<SyncStatusResponse>(
    `/api/sync/status?${params.toString()}`,
  );
}

export async function fetchConfig() {
  return httpRequest<ConfigPayload>("/api/config");
}

export async function testProxy(url?: string) {
  return httpRequest<ProxyTestResult>("/api/proxy/test", {
    method: "POST",
    body: { url: url ?? "" },
  });
}

export async function testIntegration(
  source: "cpa" | "newapi" | "sub2api",
  payload: {
    cpa?: ConfigPayload["cpa"];
    newapi?: ConfigPayload["newapi"];
    sub2api?: ConfigPayload["sub2api"];
  },
) {
  return httpRequest<IntegrationTestResult>("/api/integration/test", {
    method: "POST",
    body: {
      source,
      cpa: payload.cpa,
      newapi: payload.newapi,
      sub2api: payload.sub2api,
    },
  });
}

export async function discoverNewAPIToken(newapi: ConfigPayload["newapi"]) {
  return httpRequest<NewAPITokenDiscoverResult>(
    "/api/integration/newapi/token",
    {
      method: "POST",
      body: { newapi },
    },
  );
}

export async function fetchSub2APIGroups(sub2api: ConfigPayload["sub2api"]) {
  return httpRequest<Sub2APIGroupsResult>("/api/integration/sub2api/groups", {
    method: "POST",
    body: { sub2api },
  });
}

export async function fetchDefaultConfig() {
  return httpRequest<ConfigPayload>("/api/config/defaults");
}

export async function updateConfig(config: ConfigPayload) {
  return httpRequest<{ status: string; config: ConfigPayload }>("/api/config", {
    method: "PUT",
    body: config,
  });
}

export async function fetchRequestLogs() {
  return httpRequest<{ items: RequestLogItem[] }>("/api/requests");
}

export async function fetchVersionInfo() {
  return httpRequest<VersionInfo>("/version", {
    redirectOnUnauthorized: false,
  });
}

export async function fetchStartupCheck() {
  return httpRequest<StartupCheckResponse>("/api/startup/check");
}

export async function fetchRuntimeStatus() {
  return httpRequest<RuntimeStatusResponse>("/api/runtime/status");
}

export async function downloadDiagnosticsExport() {
  const authKey = await getStoredAuthKey();
  const response = await fetch(
    `${webConfig.apiUrl.replace(/\/$/, "")}/api/diagnostics/export`,
    {
      method: "GET",
      headers: authKey ? { Authorization: `Bearer ${authKey}` } : {},
    },
  );
  if (!response.ok) {
    let message = `download failed (${response.status})`;
    try {
      const payload = (await response.json()) as {
        error?: string;
        message?: string;
        detail?: { message?: string };
      };
      message =
        payload?.detail?.message || payload?.message || payload?.error || message;
    } catch {
      // ignore json parse errors
    }
    throw new Error(message);
  }
  const blob = await response.blob();
  const disposition = response.headers.get("content-disposition") || "";
  const match = disposition.match(/filename="([^"]+)"/i);
  const fileName =
    match?.[1] || `chatgpt-image-studio-diagnostics-${Date.now()}.json`;
  return { blob, fileName };
}

export async function runSync(
  direction: "pull" | "push",
  source: SyncSource = "cpa",
) {
  return httpRequest<{ result: SyncRunResult; status?: SyncStatusResponse }>(
    "/api/sync/run",
    {
      method: "POST",
      body: { direction, source },
    },
  );
}

export async function generateImage(
  prompt: string,
  model: ImageModel = "gpt-image-2",
  count = 1,
) {
  return generateImageWithOptions(prompt, { model, count });
}

export async function generateImageWithOptions(
  prompt: string,
  options: {
    model?: ImageModel;
    count?: number;
    size?: string;
    quality?: ImageQuality;
  } = {},
) {
  const { model = "gpt-image-2", count = 1, size, quality = "high" } = options;
  const policyHeader = buildImageAccountPolicyHeader();
  return httpRequest<ImageResponse>("/v1/images/generations", {
    method: "POST",
    headers: policyHeader
      ? { "X-Studio-Account-Policy": policyHeader }
      : undefined,
    body: {
      prompt,
      model,
      n: Math.max(1, count),
      size: size?.trim() || undefined,
      quality,
      response_format: "b64_json",
    },
  });
}

export async function editImage({
  prompt,
  images,
  mask,
  sourceReference,
  size,
  quality,
  model = "gpt-image-2",
}: {
  prompt: string;
  images: File[];
  mask?: File | null;
  sourceReference?: InpaintSourceReference;
  size?: string;
  quality?: ImageQuality;
  model?: ImageModel;
}) {
  const formData = new FormData();
  const policyHeader = buildImageAccountPolicyHeader();
  formData.append("prompt", prompt);
  formData.append("model", model);
  formData.append("response_format", "b64_json");
  if (size?.trim()) {
    formData.append("size", size.trim());
  }
  if (quality) {
    formData.append("quality", quality);
  }
  images.forEach((file) => formData.append("image", file));
  if (mask) {
    formData.append("mask", mask);
  }
  if (sourceReference) {
    formData.append("original_file_id", sourceReference.original_file_id);
    formData.append("original_gen_id", sourceReference.original_gen_id);
    formData.append("source_account_id", sourceReference.source_account_id);
    if (sourceReference.conversation_id) {
      formData.append("conversation_id", sourceReference.conversation_id);
    }
    if (sourceReference.parent_message_id) {
      formData.append("parent_message_id", sourceReference.parent_message_id);
    }
  }
  return httpRequest<ImageResponse>("/v1/images/edits", {
    method: "POST",
    headers: policyHeader
      ? { "X-Studio-Account-Policy": policyHeader }
      : undefined,
    body: formData,
  });
}
