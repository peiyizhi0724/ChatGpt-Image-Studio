import axios, { AxiosError, type AxiosRequestConfig } from "axios";

import webConfig from "@/constants/common-env";
import { clearStoredAuthKey, getStoredAuthKey } from "@/store/auth";

type RequestConfig = AxiosRequestConfig & {
  redirectOnUnauthorized?: boolean;
};

type ErrorPayload = {
  detail?: { error?: string; code?: string; message?: string };
  error?: string | { message?: string; code?: string };
  message?: string;
  code?: string;
};

export class ApiError extends Error {
  code?: string;
  status?: number;

  constructor(message: string, options: { code?: string; status?: number } = {}) {
    super(message);
    this.name = "ApiError";
    this.code = options.code;
    this.status = options.status;
  }
}

const request = axios.create({
  baseURL: webConfig.apiUrl.replace(/\/$/, ""),
});

request.interceptors.request.use(async (config) => {
  const nextConfig = { ...config };
  const authKey = await getStoredAuthKey();
  const headers = { ...(nextConfig.headers || {}) } as Record<string, string>;
  if (authKey && !headers.Authorization) {
    headers.Authorization = `Bearer ${authKey}`;
  }
  // eslint-disable-next-line @typescript-eslint/ban-ts-comment
  // @ts-expect-error
  nextConfig.headers = headers;
  return nextConfig;
});

request.interceptors.response.use(
  (response) => response,
  async (error: AxiosError<ErrorPayload>) => {
    const status = error.response?.status;
    const shouldRedirect =
      (error.config as RequestConfig | undefined)?.redirectOnUnauthorized !== false;
    if (status === 401 && shouldRedirect && typeof window !== "undefined") {
      await clearStoredAuthKey();
      window.location.href = "/admin/login";
    }

    const payload = error.response?.data;
    const nestedError =
      payload && typeof payload.error === "object" && payload.error
        ? (payload.error as { message?: string; code?: string })
        : null;
    const code =
      payload?.detail?.code ||
      nestedError?.code ||
      payload?.code;
    const message =
      payload?.detail?.error ||
      payload?.detail?.message ||
      nestedError?.message ||
      (typeof payload?.error === "string" ? payload.error : "") ||
      payload?.message ||
      error.message ||
      `请求失败 (${status || 500})`;
    return Promise.reject(new ApiError(message, { code, status }));
  },
);

type RequestOptions = {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
  timeoutMs?: number;
  redirectOnUnauthorized?: boolean;
};

export async function httpRequest<T>(path: string, options: RequestOptions = {}) {
  const {
    method = "GET",
    body,
    headers,
    timeoutMs,
    redirectOnUnauthorized = true,
  } = options;
  const config: RequestConfig = {
    url: path,
    method,
    data: body,
    headers,
    timeout: timeoutMs,
    redirectOnUnauthorized,
  };
  const response = await request.request<T>(config);
  return response.data;
}
