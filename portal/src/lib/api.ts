import { httpRequest } from "@/lib/request";

export type AccountType = "Free" | "Plus" | "Pro" | "Team";
export type AccountStatus = "正常" | "限流" | "异常" | "禁用";
export type ImageModel = "gpt-image-1" | "gpt-image-2";
export type ImageQuality = "low" | "medium" | "high";

export type Account = {
  id: string;
  type: AccountType;
  status: AccountStatus;
  quota: number;
  limits_progress?: Array<{
    feature_name?: string;
    remaining?: number;
    reset_after?: string;
  }>;
  restoreAt?: string | null;
  disabled?: boolean;
};

export type PortalUser = {
  id: string;
  email: string;
  display_name: string;
  avatar_url: string;
  role: "admin" | "user";
  disabled: boolean;
  created_at: string;
  last_login_at?: string;
  usage?: {
    image_requests: number;
    generated_images: number;
    published_works: number;
  };
};

export type PortalQuotaSummary = {
  accounts: number;
  available_accounts: number;
  total_quota: number;
  available_quota: number;
  paid_accounts: number;
};

export type PortalSessionPayload = {
  user: PortalUser;
  quota: PortalQuotaSummary;
};

export type PortalWorkspaceBootstrapResponse = PortalSessionPayload & {
  accounts: Account[];
  workspace: {
    allow_disabled_studio_accounts: boolean;
    image_mode: "studio" | "cpa";
  };
};

export type PortalUsersResponse = {
  items: PortalUser[];
  quota: PortalQuotaSummary;
};

export type PortalGalleryWork = {
  id: string;
  user_id: string;
  user_email: string;
  user_display_name: string;
  user_avatar_url: string;
  title: string;
  prompt: string;
  image_url: string;
  model: string;
  size: string;
  like_count: number;
  comment_count: number;
  created_at: string;
  liked_by_viewer: boolean;
};

export type PortalGalleryComment = {
  id: string;
  work_id: string;
  user_id: string;
  user_email: string;
  user_display_name: string;
  user_avatar_url: string;
  content: string;
  created_at: string;
};

export type PortalGalleryWorksResponse = {
  items: PortalGalleryWork[];
};

export type PortalGalleryWorkResponse = {
  item: PortalGalleryWork;
  comments: PortalGalleryComment[];
};

export type PortalAccountQuotaResponse = {
  id: string;
  status: AccountStatus;
  type: AccountType;
  quota: number;
  image_gen_remaining?: number | null;
  image_gen_reset_after?: string | null;
  refresh_requested: boolean;
  refreshed: boolean;
  refresh_error?: string;
};

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

type ImageResponse = {
  created: number;
  data: ImageResponseItem[];
};

export type VersionInfo = {
  version: string;
  commit?: string;
  buildTime?: string;
};

export async function loginPortal(email: string, password: string) {
  return httpRequest<PortalSessionPayload>("/portal/api/login", {
    method: "POST",
    body: { email, password },
    redirectOnUnauthorized: false,
  });
}

export async function sendPortalRegisterCode(email: string) {
  return httpRequest<{
    ok: boolean;
    expires_in_seconds: number;
    resend_in_seconds: number;
    delivery: "email";
  }>("/portal/api/register/code", {
    method: "POST",
    body: { email },
    redirectOnUnauthorized: false,
  });
}

export async function registerPortal(email: string, password: string, code: string) {
  return httpRequest<PortalSessionPayload>("/portal/api/register", {
    method: "POST",
    body: { email, password, code },
    redirectOnUnauthorized: false,
  });
}

export async function sendPortalPasswordResetCode(email: string) {
  return httpRequest<{
    ok: boolean;
    expires_in_seconds: number;
    resend_in_seconds: number;
    delivery: "email";
  }>("/portal/api/password/code", {
    method: "POST",
    body: { email },
    redirectOnUnauthorized: false,
  });
}

export async function resetPortalPassword(email: string, password: string, code: string) {
  return httpRequest<{ ok: boolean }>("/portal/api/password/reset", {
    method: "POST",
    body: { email, password, code },
    redirectOnUnauthorized: false,
  });
}

export async function logoutPortal() {
  return httpRequest<{ ok: boolean }>("/portal/api/logout", {
    method: "POST",
    redirectOnUnauthorized: false,
  });
}

export async function fetchPortalMe() {
  return httpRequest<PortalSessionPayload>("/portal/api/me", {
    redirectOnUnauthorized: false,
  });
}

export async function updatePortalProfile(payload: { display_name?: string; avatar_url?: string }) {
  return httpRequest<PortalSessionPayload>("/portal/api/me/profile", {
    method: "PATCH",
    body: payload,
  });
}

export async function changePortalPassword(currentPassword: string, newPassword: string) {
  return httpRequest<{ ok: boolean }>("/portal/api/me/password", {
    method: "POST",
    body: {
      current_password: currentPassword,
      new_password: newPassword,
    },
  });
}

export async function fetchPortalWorkspaceBootstrap() {
  return httpRequest<PortalWorkspaceBootstrapResponse>("/portal/api/workspace/bootstrap");
}

export async function fetchPortalUsers() {
  return httpRequest<PortalUsersResponse>("/portal/api/admin/users");
}

export async function updatePortalUser(userId: string, updates: { role?: "admin" | "user"; disabled?: boolean }) {
  return httpRequest<{ item: PortalUser; quota: PortalQuotaSummary }>(
    `/portal/api/admin/users/${encodeURIComponent(userId)}`,
    {
      method: "PATCH",
      body: updates,
    },
  );
}

export async function fetchPortalGalleryWorks(params: { sort?: string; query?: string } = {}) {
  const search = new URLSearchParams();
  if (params.sort?.trim()) {
    search.set("sort", params.sort.trim());
  }
  if (params.query?.trim()) {
    search.set("query", params.query.trim());
  }
  const suffix = search.toString() ? `?${search.toString()}` : "";
  return httpRequest<PortalGalleryWorksResponse>(`/portal/api/gallery/works${suffix}`);
}

export async function fetchPortalGalleryWork(workId: string) {
  return httpRequest<PortalGalleryWorkResponse>(`/portal/api/gallery/works/${encodeURIComponent(workId)}`);
}

export async function publishPortalGalleryWork(payload: {
  title?: string;
  prompt: string;
  image_data_url?: string;
  image_url?: string;
  model?: string;
  size?: string;
}) {
  return httpRequest<{ item: PortalGalleryWork }>("/portal/api/gallery/works", {
    method: "POST",
    body: payload,
  });
}

export async function togglePortalGalleryLike(workId: string) {
  return httpRequest<{ liked: boolean; like_count: number }>(
    `/portal/api/gallery/works/${encodeURIComponent(workId)}/likes/toggle`,
    {
      method: "POST",
    },
  );
}

export async function createPortalGalleryComment(workId: string, content: string) {
  return httpRequest<{ item: PortalGalleryComment; comment_count: number }>(
    `/portal/api/gallery/works/${encodeURIComponent(workId)}/comments`,
    {
      method: "POST",
      body: { content },
    },
  );
}

export async function fetchAccounts() {
  const payload = await fetchPortalWorkspaceBootstrap();
  return { items: payload.accounts };
}

export async function fetchConfig() {
  const payload = await fetchPortalWorkspaceBootstrap();
  return {
    chatgpt: {
      imageMode: payload.workspace.image_mode,
      studioAllowDisabledImageAccounts: payload.workspace.allow_disabled_studio_accounts,
    },
  };
}

export async function fetchAccountQuota(accountId: string, options: { refresh?: boolean } = {}) {
  const refresh = options.refresh ?? true;
  const suffix = refresh ? "" : "?refresh=false";
  return httpRequest<PortalAccountQuotaResponse>(
    `/portal/api/workspace/accounts/${encodeURIComponent(accountId)}/quota${suffix}`,
  );
}

export async function fetchVersionInfo() {
  return httpRequest<VersionInfo>("/version", {
    redirectOnUnauthorized: false,
  });
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
