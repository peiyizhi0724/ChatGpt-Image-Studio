import { httpRequest } from "@/lib/request";

export type AccountType = "Free" | "Plus" | "Pro" | "Team";
export type AccountStatus = "正常" | "限流" | "异常" | "禁用";
export type SyncStatus = "synced" | "pending_upload" | "remote_only" | "remote_deleted";
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
  error?: string;
  direction?: string;
  uploaded: number;
  upload_failed: number;
  downloaded: number;
  download_failed: number;
  remote_deleted: number;
  disabled_aligned: number;
  disabled_align_failed: number;
  started_at: string;
  finished_at: string;
};

export type SyncStatusResponse = {
  configured: boolean;
  local: number;
  remote: number;
  summary: Record<SyncStatus, number>;
  accounts: SyncAccount[];
  disabledMismatch: number;
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
  };
  storage: {
    authDir: string;
    stateFile: string;
    syncStateDir: string;
    imageDir: string;
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
  };
  cpa: {
    baseUrl: string;
    apiKey: string;
    requestTimeout: number;
    routeStrategy: "images_api" | "codex_responses" | "auto";
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

export async function fetchAccountQuota(accountId: string, options: { refresh?: boolean } = {}) {
  const refresh = options.refresh ?? true;
  const suffix = refresh ? "" : "?refresh=false";
  return httpRequest<AccountQuotaResponse>(`/api/accounts/${encodeURIComponent(accountId)}/quota${suffix}`);
}

export async function fetchSyncStatus() {
  return httpRequest<SyncStatusResponse>("/api/sync/status");
}

export async function fetchConfig() {
  return httpRequest<ConfigPayload>("/api/config");
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

export async function runSync(direction: "pull" | "push") {
  return httpRequest<{ result: SyncRunResult; status?: SyncStatusResponse }>("/api/sync/run", {
    method: "POST",
    body: { direction },
  });
}

export async function generateImage(prompt: string, model: ImageModel = "gpt-image-2", count = 1) {
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
  return httpRequest<ImageResponse>("/v1/images/generations", {
    method: "POST",
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
  model = "gpt-image-2",
}: {
  prompt: string;
  images: File[];
  mask?: File | null;
  sourceReference?: InpaintSourceReference;
  model?: ImageModel;
}) {
  const formData = new FormData();
  formData.append("prompt", prompt);
  formData.append("model", model);
  formData.append("response_format", "b64_json");
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
    body: formData,
  });
}

export async function upscaleImage({
  image,
  prompt,
  scale,
  model = "gpt-image-2",
}: {
  image: File;
  prompt?: string;
  scale?: string;
  model?: ImageModel;
}) {
  const formData = new FormData();
  formData.append("image", image);
  formData.append("model", model);
  formData.append("response_format", "b64_json");
  formData.append("scale", scale || "2x");
  if (prompt?.trim()) {
    formData.append("prompt", prompt.trim());
  }
  return httpRequest<ImageResponse>("/v1/images/upscale", {
    method: "POST",
    body: formData,
  });
}