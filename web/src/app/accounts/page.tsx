"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { ComponentProps } from "react";
import { Link } from "react-router-dom";
import {
  Ban,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  CircleAlert,
  CircleOff,
  Copy,
  FileUp,
  LoaderCircle,
  Pencil,
  RefreshCw,
  Search,
  Trash2,
  Shield,
  UserRound,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  deleteAccounts,
  fetchAccountQuota,
  fetchAccounts,
  fetchSyncStatus,
  importAccountFiles,
  refreshAccounts,
  runSync,
  updateAccount,
  type AccountImportResponse,
  type Account,
  type AccountQuotaResponse,
  type AccountStatus,
  type AccountType,
  type SyncAccount,
  type SyncStatus,
  type SyncStatusResponse,
} from "@/lib/api";
import { getCachedSyncStatus, setCachedSyncStatus } from "@/store/sync-status-cache";
import { cn } from "@/lib/utils";

const accountTypeOptions: { label: string; value: AccountType | "all" }[] = [
  { label: "全部类型", value: "all" },
  { label: "Free", value: "Free" },
  { label: "Plus", value: "Plus" },
  { label: "Team", value: "Team" },
  { label: "Pro", value: "Pro" },
];

const accountStatusOptions: { label: string; value: AccountStatus | "all" }[] = [
  { label: "全部状态", value: "all" },
  { label: "正常", value: "正常" },
  { label: "限流", value: "限流" },
  { label: "异常", value: "异常" },
  { label: "禁用", value: "禁用" },
];

const statusMeta: Record<
  AccountStatus,
  {
    icon: typeof CheckCircle2;
    badge: ComponentProps<typeof Badge>["variant"];
  }
> = {
  正常: { icon: CheckCircle2, badge: "success" },
  限流: { icon: CircleAlert, badge: "warning" },
  异常: { icon: CircleOff, badge: "danger" },
  禁用: { icon: Ban, badge: "secondary" },
};

const syncMeta: Record<
  SyncStatus,
  {
    label: string;
    badge: ComponentProps<typeof Badge>["variant"];
  }
> = {
  synced: { label: "已同步", badge: "success" },
  pending_upload: { label: "待上传", badge: "warning" },
  remote_only: { label: "远端独有", badge: "info" },
  remote_deleted: { label: "远端已删", badge: "danger" },
};

const metricCards = [
  { key: "total", label: "账户总数", color: "text-stone-900", icon: UserRound },
  { key: "active", label: "正常账户", color: "text-emerald-600", icon: CheckCircle2 },
  { key: "limited", label: "限流账户", color: "text-orange-500", icon: CircleAlert },
  { key: "abnormal", label: "异常账户", color: "text-rose-500", icon: CircleOff },
  { key: "disabled", label: "禁用账户", color: "text-stone-500", icon: Ban },
  { key: "quota", label: "剩余额度", color: "text-blue-500", icon: RefreshCw },
] as const;

function formatCompact(value: number) {
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}k`;
  }
  return String(value);
}

function formatQuota(value: number) {
  return String(Math.max(0, value));
}

function formatRestoreAt(value?: string | null) {
  if (!value) {
    return { absolute: "—", absoluteShort: "—", relative: "" };
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return { absolute: value, absoluteShort: value, relative: "" };
  }

  const diffMs = Math.max(0, date.getTime() - Date.now());
  const totalHours = Math.ceil(diffMs / (1000 * 60 * 60));
  const days = Math.floor(totalHours / 24);
  const hours = totalHours % 24;
  const relative = diffMs > 0 ? `剩余 ${days}d ${hours}h` : "已到恢复时间";

  const pad = (num: number) => String(num).padStart(2, "0");
  const absoluteShort = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
    date.getHours(),
  )}:${pad(date.getMinutes())}`;
  const absolute = `${absoluteShort}:${pad(date.getSeconds())}`;

  return { absolute, absoluteShort, relative };
}

function formatQuotaSummary(accounts: Account[]) {
  return formatCompact(accounts.reduce((sum, account) => sum + Math.max(0, account.quota), 0));
}

function maskToken(token?: string) {
  if (!token) return "—";
  if (token.length <= 18) return token;
  return `${token.slice(0, 16)}...${token.slice(-8)}`;
}

function normalizeAccounts(items: Account[]): Account[] {
  return items.map((item) => ({
    ...item,
    type:
      item.type === "Plus" || item.type === "Team" || item.type === "Pro" || item.type === "Free"
        ? item.type
        : "Free",
  }));
}

function buildImportSummary(data: AccountImportResponse) {
  const imported = data.imported ?? 0;
  const duplicates = data.duplicates?.length ?? 0;
  const failed = data.failed?.length ?? 0;
  const refreshed = data.refreshed ?? 0;
  return `导入 ${imported} 个，刷新 ${refreshed} 个，重复 ${duplicates} 个，失败 ${failed} 个`;
}

function extractImageGenLimit(account: Account) {
  const imageGen = account.limits_progress?.find((item) => item.feature_name === "image_gen");
  return {
    remaining: typeof imageGen?.remaining === "number" ? imageGen.remaining : null,
    resetAfter: imageGen?.reset_after || account.restoreAt || null,
  };
}

function mergeImageGenLimit(
  limitsProgress: Account["limits_progress"],
  remaining: number | null | undefined,
  resetAfter: string | null | undefined,
) {
  const next = Array.isArray(limitsProgress) ? [...limitsProgress] : [];
  const currentIndex = next.findIndex((item) => item.feature_name === "image_gen");
  const nextItem = {
    feature_name: "image_gen",
    remaining: typeof remaining === "number" ? remaining : undefined,
    reset_after: resetAfter || undefined,
  };

  if (currentIndex >= 0) {
    next[currentIndex] = {
      ...next[currentIndex],
      ...nextItem,
    };
    return next;
  }

  next.push(nextItem);
  return next;
}

function applyQuotaResultToAccount(account: Account, quota: AccountQuotaResponse): Account {
  return {
    ...account,
    status: quota.status,
    type: quota.type,
    quota: quota.quota,
    restoreAt: quota.image_gen_reset_after || account.restoreAt,
    limits_progress: mergeImageGenLimit(account.limits_progress, quota.image_gen_remaining, quota.image_gen_reset_after),
  };
}

function normalizeSyncStatus(payload: SyncStatusResponse | null) {
  return {
    configured: payload?.configured ?? false,
    local: payload?.local ?? 0,
    remote: payload?.remote ?? 0,
    accounts: payload?.accounts ?? [],
    disabledMismatch: payload?.disabledMismatch ?? 0,
    lastRun: payload?.lastRun ?? null,
    summary: {
      synced: payload?.summary?.synced ?? 0,
      pending_upload: payload?.summary?.pending_upload ?? 0,
      remote_only: payload?.summary?.remote_only ?? 0,
      remote_deleted: payload?.summary?.remote_deleted ?? 0,
    } satisfies Record<SyncStatus, number>,
  };
}

export default function AccountsPage() {
  const didLoadRef = useRef(false);
  const importInputRef = useRef<HTMLInputElement>(null);
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState<AccountType | "all">("all");
  const [statusFilter, setStatusFilter] = useState<AccountStatus | "all">("all");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState("10");
  const [editingAccount, setEditingAccount] = useState<Account | null>(null);
  const [editType, setEditType] = useState<AccountType>("Free");
  const [editStatus, setEditStatus] = useState<AccountStatus>("正常");
  const [editQuota, setEditQuota] = useState("0");
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);
  const [quotaRefreshingId, setQuotaRefreshingId] = useState<string | null>(null);
  const [accountQuotaMap, setAccountQuotaMap] = useState<Record<string, AccountQuotaResponse>>({});
  const [syncStatus, setSyncStatus] = useState<SyncStatusResponse | null>(null);
  const [isSyncLoading, setIsSyncLoading] = useState(true);
  const [syncRunningDirection, setSyncRunningDirection] = useState<"pull" | "push" | null>(null);

  const replaceAccounts = (items: Account[]) => {
    setAccounts(normalizeAccounts(items));
    setAccountQuotaMap({});
    setSelectedIds((prev) => prev.filter((id) => items.some((item) => item.id === id)));
  };

  const loadAccounts = async (silent = false) => {
    if (!silent) {
      setIsLoading(true);
    }
    try {
      const data = await fetchAccounts();
      replaceAccounts(data.items);
    } catch (error) {
      const message = error instanceof Error ? error.message : "加载账户失败";
      toast.error(message);
    } finally {
      if (!silent) {
        setIsLoading(false);
      }
    }
  };

  const loadSync = async ({
    silent = false,
    force = false,
    revalidate = false,
    suppressError = false,
  }: {
    silent?: boolean;
    force?: boolean;
    revalidate?: boolean;
    suppressError?: boolean;
  } = {}) => {
    const cachedStatus = getCachedSyncStatus();
    if (!silent && !force && cachedStatus) {
      setSyncStatus(cachedStatus);
      setIsSyncLoading(false);
      if (revalidate) {
        void loadSync({ silent: true, force: true, suppressError: true });
      }
      return;
    }

    if (!silent) {
      setIsSyncLoading(true);
    }
    try {
      const data = await fetchSyncStatus();
      setCachedSyncStatus(data);
      setSyncStatus(data);
    } catch (error) {
      if (!suppressError) {
        const message = error instanceof Error ? error.message : "读取同步状态失败";
        toast.error(message);
      }
    } finally {
      if (!silent) {
        setIsSyncLoading(false);
      }
    }
  };

  useEffect(() => {
    if (didLoadRef.current) {
      return;
    }
    didLoadRef.current = true;
    void Promise.all([loadAccounts(), loadSync({ revalidate: true })]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filteredAccounts = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return accounts.filter((account) => {
      const searchMatched =
        normalizedQuery.length === 0 ||
        (account.email ?? "").toLowerCase().includes(normalizedQuery) ||
        (account.fileName ?? "").toLowerCase().includes(normalizedQuery) ||
        (account.note ?? "").toLowerCase().includes(normalizedQuery);
      const typeMatched = typeFilter === "all" || account.type === typeFilter;
      const statusMatched = statusFilter === "all" || account.status === statusFilter;
      return searchMatched && typeMatched && statusMatched;
    });
  }, [accounts, query, statusFilter, typeFilter]);

  const pageCount = Math.max(1, Math.ceil(filteredAccounts.length / Number(pageSize)));
  const safePage = Math.min(page, pageCount);
  const startIndex = (safePage - 1) * Number(pageSize);
  const currentRows = filteredAccounts.slice(startIndex, startIndex + Number(pageSize));
  const allCurrentSelected =
    currentRows.length > 0 && currentRows.every((row) => selectedIds.includes(row.id));

  const summary = useMemo(() => {
    const total = accounts.length;
    const active = accounts.filter((item) => item.status === "正常").length;
    const limited = accounts.filter((item) => item.status === "限流").length;
    const abnormal = accounts.filter((item) => item.status === "异常").length;
    const disabled = accounts.filter((item) => item.status === "禁用").length;
    const quota = formatQuotaSummary(accounts);

    return { total, active, limited, abnormal, disabled, quota };
  }, [accounts]);

  const selectedTokens = useMemo(() => {
    const selectedSet = new Set(selectedIds);
    return accounts.filter((item) => selectedSet.has(item.id)).map((item) => item.access_token);
  }, [accounts, selectedIds]);

  const abnormalTokens = useMemo(() => {
    return accounts.filter((item) => item.status === "异常").map((item) => item.access_token);
  }, [accounts]);

  const syncView = useMemo(() => normalizeSyncStatus(syncStatus), [syncStatus]);

  const syncMap = useMemo(() => {
    return syncView.accounts.reduce<Record<string, SyncAccount>>((acc, item) => {
      acc[item.name] = item;
      return acc;
    }, {});
  }, [syncView.accounts]);

  const paginationItems = useMemo(() => {
    const items: (number | "...")[] = [];
    const start = Math.max(1, safePage - 1);
    const end = Math.min(pageCount, safePage + 1);

    if (start > 1) items.push(1);
    if (start > 2) items.push("...");
    for (let current = start; current <= end; current += 1) items.push(current);
    if (end < pageCount - 1) items.push("...");
    if (end < pageCount) items.push(pageCount);

    return items;
  }, [pageCount, safePage]);

  const handleImportFiles = async (files: FileList | null) => {
    const normalizedFiles = files ? Array.from(files) : [];
    if (normalizedFiles.length === 0) {
      return;
    }

    setIsImporting(true);
    try {
      const data = await importAccountFiles(normalizedFiles);
      replaceAccounts(data.items);
      setPage(1);
      await loadSync({ silent: true, force: true });

      const failedMessage = data.failed?.[0]?.error;
      if ((data.failed?.length ?? 0) > 0) {
        toast.error(`${buildImportSummary(data)}${failedMessage ? `，首个错误：${failedMessage}` : ""}`);
      } else if ((data.duplicates?.length ?? 0) > 0) {
        toast.success(`${buildImportSummary(data)}。重复文件已跳过`);
      } else {
        toast.success(buildImportSummary(data));
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "导入认证文件失败";
      toast.error(message);
    } finally {
      setIsImporting(false);
    }
  };

  const handleDeleteTokens = async (tokens: string[]) => {
    if (tokens.length === 0) {
      toast.error("请先选择要删除的账户");
      return;
    }

    setIsDeleting(true);
    try {
      const data = await deleteAccounts(tokens);
      replaceAccounts(data.items);
      await loadSync({ silent: true, force: true });
      toast.success(`删除 ${data.removed ?? 0} 个账户`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "删除账户失败";
      toast.error(message);
    } finally {
      setIsDeleting(false);
    }
  };

  const handleRefreshAccounts = async (accessTokens: string[], mode: "selected" | "all") => {
    if (mode === "selected" && accessTokens.length === 0) {
      toast.error("没有需要刷新的账户");
      return;
    }

    setIsRefreshing(true);
    try {
      const data = await refreshAccounts(accessTokens);
      replaceAccounts(data.items);
      if (data.errors.length > 0) {
        const firstError = data.errors[0]?.error;
        toast.error(
          `刷新成功 ${data.refreshed} 个，失败 ${data.errors.length} 个${firstError ? `，首个错误：${firstError}` : ""}`,
        );
      } else {
        toast.success(mode === "all" ? `已一键刷新全部账户，共 ${data.refreshed} 个` : `刷新成功 ${data.refreshed} 个账户`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "刷新账户失败";
      toast.error(message);
    } finally {
      setIsRefreshing(false);
    }
  };

  const handleRefreshAccountQuota = async (account: Account) => {
    setQuotaRefreshingId(account.id);
    try {
      const data = await fetchAccountQuota(account.id);
      setAccountQuotaMap((prev) => ({
        ...prev,
        [account.id]: data,
      }));
      setAccounts((prev) => prev.map((item) => (item.id === account.id ? applyQuotaResultToAccount(item, data) : item)));

      if (data.refresh_error) {
        toast.error(data.refresh_error);
        return;
      }
      const remainingText =
        typeof data.image_gen_remaining === "number" ? `${data.image_gen_remaining}` : "—";
      toast.success(`图片额度已刷新，剩余 ${remainingText}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "刷新图片额度失败";
      toast.error(message);
    } finally {
      setQuotaRefreshingId(null);
    }
  };

  const handleRunSync = async (direction: "pull" | "push") => {
    setSyncRunningDirection(direction);
    try {
      const data = await runSync(direction);
      if (data.status) {
        setCachedSyncStatus(data.status);
        setSyncStatus(data.status);
      } else {
        await loadSync({ silent: true, force: true });
      }
      const result = data.result;
      if (!result.ok && result.error) {
        toast.error(result.error);
        return;
      }
      if (direction === "pull") {
        toast.success(`从 CPA 同步完成：拉取 ${result.downloaded} 个账号，状态对齐 ${result.disabled_aligned}`);
      } else {
        toast.success(`同步至 CPA 完成：推送 ${result.uploaded} 个账号，状态对齐 ${result.disabled_aligned}`);
      }
      await loadAccounts(true);
    } catch (error) {
      const message = error instanceof Error ? error.message : "执行同步失败";
      toast.error(message);
    } finally {
      setSyncRunningDirection(null);
    }
  };

  const openEditDialog = (account: Account) => {
    setEditingAccount(account);
    setEditType(account.type);
    setEditStatus(account.status);
    setEditQuota(String(account.quota));
  };

  const handleUpdateAccount = async () => {
    if (!editingAccount) {
      return;
    }

    setIsUpdating(true);
    try {
      const data = await updateAccount(editingAccount.access_token, {
        type: editType,
        status: editStatus,
        quota: Number(editQuota || 0),
      });
      replaceAccounts(data.items);
      setEditingAccount(null);
      toast.success("账号信息已更新");
    } catch (error) {
      const message = error instanceof Error ? error.message : "更新账号失败";
      toast.error(message);
    } finally {
      setIsUpdating(false);
    }
  };

  const toggleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedIds((prev) => Array.from(new Set([...prev, ...currentRows.map((item) => item.id)])));
      return;
    }
    setSelectedIds((prev) => prev.filter((id) => !currentRows.some((row) => row.id === id)));
  };

  return (
    <div className="hide-scrollbar h-full min-h-0 overflow-y-auto rounded-[30px] border border-stone-200 bg-[#fcfcfb] px-4 py-5 shadow-[0_14px_40px_rgba(15,23,42,0.05)] sm:px-5 sm:py-6 lg:px-6 lg:py-7">
      <section className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-4">
          <div className="inline-flex size-12 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
            <Shield className="size-5" />
          </div>
          <div className="space-y-1">
            <h1 className="text-2xl font-semibold tracking-tight text-stone-950">号池管理</h1>
          </div>
        </div>
      </section>

      <input
        ref={importInputRef}
        type="file"
        accept="application/json,.json"
        multiple
        className="hidden"
        onChange={(event) => {
          void handleImportFiles(event.target.files);
          event.currentTarget.value = "";
        }}
      />

      <Dialog open={Boolean(editingAccount)} onOpenChange={(open) => (!open ? setEditingAccount(null) : null)}>
        <DialogContent showCloseButton={false} className="rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>编辑账户</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              手动修改账号状态、类型和额度。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">状态</label>
              <Select value={editStatus} onValueChange={(value) => setEditStatus(value as AccountStatus)}>
                <SelectTrigger className="h-11 rounded-xl border-stone-200 bg-white">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {accountStatusOptions
                    .filter((option) => option.value !== "all")
                    .map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">类型</label>
              <Select value={editType} onValueChange={(value) => setEditType(value as AccountType)}>
                <SelectTrigger className="h-11 rounded-xl border-stone-200 bg-white">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {accountTypeOptions
                    .filter((option) => option.value !== "all")
                    .map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium text-stone-700">额度</label>
              <Input
                value={editQuota}
                onChange={(event) => setEditQuota(event.target.value)}
                className="h-11 rounded-xl border-stone-200 bg-white"
              />
            </div>
          </div>
          <DialogFooter className="pt-2">
            <Button
              variant="secondary"
              className="h-10 rounded-xl bg-stone-100 px-5 text-stone-700 hover:bg-stone-200"
              onClick={() => setEditingAccount(null)}
              disabled={isUpdating}
            >
              取消
            </Button>
            <Button
              className="h-10 rounded-xl bg-stone-950 px-5 text-white hover:bg-stone-800"
              onClick={() => void handleUpdateAccount()}
              disabled={isUpdating}
            >
              {isUpdating ? <LoaderCircle className="size-4 animate-spin" /> : null}
              保存修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <section className="mt-6 space-y-4">
        <Card className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
          <CardContent className="space-y-4 p-5">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div className="space-y-1">
                <h2 className="text-lg font-semibold tracking-tight">CPA 同步</h2>
                <p className="text-sm text-stone-500">
                  通过 `CLIProxyAPI /v0/management/auth-files` 双向同步本地 auth 文件。
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  className="h-10 rounded-xl border-stone-200 bg-white px-4 text-stone-700"
                  onClick={() => void loadSync({ force: true })}
                  disabled={isSyncLoading || syncRunningDirection !== null}
                >
                  <RefreshCw className={cn("size-4", isSyncLoading ? "animate-spin" : "")} />
                  刷新同步状态
                </Button>
                <Button
                  className="h-10 rounded-xl bg-stone-950 px-4 text-white hover:bg-stone-800"
                  onClick={() => void handleRunSync("pull")}
                  disabled={!syncView.configured || isSyncLoading || syncRunningDirection !== null}
                >
                  {syncRunningDirection === "pull" ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                  从 CPA 同步
                </Button>
                <Button
                  className="h-10 rounded-xl bg-stone-900 px-4 text-white hover:bg-stone-800"
                  onClick={() => void handleRunSync("push")}
                  disabled={!syncView.configured || isSyncLoading || syncRunningDirection !== null}
                >
                  {syncRunningDirection === "push" ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                  同步至 CPA
                </Button>
              </div>
            </div>

            {isSyncLoading ? (
              <div className="rounded-2xl border border-stone-200 bg-stone-50 px-4 py-4 text-sm leading-6 text-stone-500">
                正在读取 CPA 同步状态...
              </div>
            ) : !syncView.configured ? (
              <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-4 text-sm leading-6 text-amber-700">
                后端还没有配置 CPA 同步。请前往
                <Link to="/settings" className="mx-1 font-medium underline decoration-amber-400 underline-offset-4">
                  配置管理
                </Link>
                开启 <code>sync.enabled = true</code>，并填写 <code>sync.base_url</code> 与{" "}
                <code>sync.management_key</code>。
              </div>
            ) : (
              <>
                <div className="grid gap-3 md:grid-cols-5">
                  {([
                    ["本地", syncView.local],
                    ["远端", syncView.remote],
                    ["待上传", syncView.summary.pending_upload],
                    ["远端独有", syncView.summary.remote_only],
                    ["远端已删", syncView.summary.remote_deleted],
                  ] as const).map(([label, value]) => (
                    <div key={label} className="rounded-2xl border border-stone-100 bg-stone-50 px-4 py-4">
                      <div className="text-xs font-medium text-stone-400">{label}</div>
                      <div className="mt-2 text-2xl font-semibold tracking-tight text-stone-900">{value}</div>
                    </div>
                  ))}
                </div>

                <div className="flex flex-wrap gap-2">
                  {syncView.disabledMismatch > 0 ? (
                    <Badge variant="warning" className="rounded-lg px-3 py-1">
                      状态不一致 {syncView.disabledMismatch}
                    </Badge>
                  ) : null}
                  {syncView.lastRun ? (
                    <Badge variant={syncView.lastRun.ok ? "success" : "danger"} className="rounded-lg px-3 py-1">
                      最近一次同步：{new Date(syncView.lastRun.finished_at).toLocaleString("zh-CN")}
                    </Badge>
                  ) : null}
                </div>

                {syncView.accounts.length > 0 ? (
                  <div className="rounded-2xl border border-stone-100 bg-stone-50 px-4 py-4">
                    <div className="mb-3 text-sm font-medium text-stone-700">待处理文件</div>
                    <div className="flex flex-wrap gap-2">
                      {syncView.accounts
                        .filter((item) => item.status !== "synced")
                        .slice(0, 18)
                        .map((item) => (
                          <Badge key={item.name} variant={syncMeta[item.status].badge} className="rounded-lg px-3 py-1">
                            {syncMeta[item.status].label} · {item.name}
                          </Badge>
                        ))}
                      {syncView.accounts.filter((item) => item.status !== "synced").length === 0 ? (
                        <Badge variant="success" className="rounded-lg px-3 py-1">
                          当前没有待同步文件
                        </Badge>
                      ) : null}
                    </div>
                  </div>
                ) : null}
              </>
            )}
          </CardContent>
        </Card>
      </section>

      <section className="mt-5 space-y-4">
        <div className="grid gap-4 md:grid-cols-3 xl:grid-cols-6">
          {metricCards.map((item) => {
            const Icon = item.icon;
            const value = summary[item.key];
            return (
              <Card key={item.key} className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
                <CardContent className="p-4">
                  <div className="mb-4 flex items-start justify-between">
                    <span className="text-xs font-medium text-stone-400">{item.label}</span>
                    <Icon className="size-4 text-stone-400" />
                  </div>
                  <div className={cn("text-[1.75rem] font-semibold tracking-tight", item.color)}>
                    <span className={typeof value === "number" ? "" : "text-[1.1rem]"}>
                      {typeof value === "number" ? formatCompact(value) : value}
                    </span>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      </section>

      <section className="mt-5 space-y-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-center gap-3">
            <h2 className="text-lg font-semibold tracking-tight">账户列表</h2>
            <Badge variant="secondary" className="rounded-lg bg-stone-200 px-2 py-0.5 text-stone-700">
              {filteredAccounts.length}
            </Badge>
          </div>

          <div className="flex flex-col gap-2 lg:flex-row lg:items-center">
            <div className="relative min-w-[260px]">
              <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-stone-400" />
              <Input
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setPage(1);
                }}
                placeholder="搜索邮箱 / 文件名 / 备注"
                className="h-10 rounded-xl border-stone-200 bg-white/85 pl-10"
              />
            </div>
            <Select
              value={typeFilter}
              onValueChange={(value) => {
                setTypeFilter(value as AccountType | "all");
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full rounded-xl border-stone-200 bg-white/85 lg:w-[150px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {accountTypeOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={statusFilter}
              onValueChange={(value) => {
                setStatusFilter(value as AccountStatus | "all");
                setPage(1);
              }}
            >
              <SelectTrigger className="h-10 w-full rounded-xl border-stone-200 bg-white/85 lg:w-[150px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {accountStatusOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              className="h-10 rounded-xl bg-stone-950 px-4 text-white hover:bg-stone-800"
              onClick={() => importInputRef.current?.click()}
              disabled={isImporting}
            >
              {isImporting ? <LoaderCircle className="size-4 animate-spin" /> : <FileUp className="size-4" />}
              导入认证文件
            </Button>
          </div>
        </div>

        {isLoading && accounts.length === 0 ? (
          <Card className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
            <CardContent className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
              <div className="rounded-xl bg-stone-100 p-3 text-stone-500">
                <LoaderCircle className="size-5 animate-spin" />
              </div>
              <div className="space-y-1">
                <p className="text-sm font-medium text-stone-700">正在加载账户</p>
                <p className="text-sm text-stone-500">从后端读取本地 auth 文件和运行状态。</p>
              </div>
            </CardContent>
          </Card>
        ) : null}

        <Card
          className={cn(
            "overflow-hidden rounded-2xl border-white/80 bg-white/90 shadow-sm",
            isLoading && accounts.length === 0 ? "hidden" : "",
          )}
        >
          <CardContent className="space-y-0 p-0">
            <div className="flex flex-col gap-3 border-b border-stone-100 px-4 py-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex flex-wrap items-center gap-2 text-sm text-stone-500">
                <Button
                  variant="ghost"
                  className="h-8 rounded-lg px-3 text-stone-500 hover:bg-stone-100"
                  onClick={() => void handleRefreshAccounts([], "all")}
                  disabled={accounts.length === 0 || isRefreshing}
                >
                  {isRefreshing ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                  一键刷新全部额度
                </Button>
                <Button
                  variant="ghost"
                  className="h-8 rounded-lg px-3 text-stone-500 hover:bg-stone-100"
                  onClick={() => void handleRefreshAccounts(selectedTokens, "selected")}
                  disabled={selectedTokens.length === 0 || isRefreshing}
                >
                  {isRefreshing ? <LoaderCircle className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
                  刷新选中账号信息
                </Button>
                <Button
                  variant="ghost"
                  className="h-8 rounded-lg px-3 text-rose-500 hover:bg-rose-50 hover:text-rose-600"
                  onClick={() => void handleDeleteTokens(abnormalTokens)}
                  disabled={abnormalTokens.length === 0 || isDeleting}
                >
                  {isDeleting ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                  移除异常账号
                </Button>
                <Button
                  variant="ghost"
                  className="h-8 rounded-lg px-3 text-rose-500 hover:bg-rose-50 hover:text-rose-600"
                  onClick={() => void handleDeleteTokens(selectedTokens)}
                  disabled={selectedTokens.length === 0 || isDeleting}
                >
                  {isDeleting ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                  删除所选
                </Button>
                {selectedIds.length > 0 ? (
                  <span className="rounded-lg bg-stone-100 px-2.5 py-1 text-xs font-medium text-stone-600">
                    已选择 {selectedIds.length} 项
                  </span>
                ) : null}
              </div>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full min-w-[1120px] text-left">
                <thead className="border-b border-stone-100 text-[11px] text-stone-400 uppercase tracking-[0.18em]">
                  <tr>
                    <th className="w-12 px-4 py-3 text-center">
                      <Checkbox
                        checked={allCurrentSelected}
                        onCheckedChange={(checked) => toggleSelectAll(Boolean(checked))}
                      />
                    </th>
                    <th className="w-80 px-4 py-3 whitespace-nowrap">账号 / Token</th>
                    <th className="w-28 px-4 py-3 text-center whitespace-nowrap">类型</th>
                    <th className="w-24 px-4 py-3 text-center whitespace-nowrap">状态</th>
                    <th className="w-28 px-4 py-3 text-center whitespace-nowrap">同步</th>
                    <th className="w-32 px-4 py-3 text-center whitespace-nowrap">图片额度</th>
                    <th className="w-44 px-4 py-3 text-center whitespace-nowrap">图片重置</th>
                    <th className="w-18 px-4 py-3 text-center whitespace-nowrap">成功</th>
                    <th className="w-18 px-4 py-3 text-center whitespace-nowrap">失败</th>
                    <th className="w-24 px-4 py-3 text-center whitespace-nowrap">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {currentRows.map((account) => {
                    const status = statusMeta[account.status];
                    const StatusIcon = status.icon;
                    const syncState = syncMap[account.fileName]?.status || account.syncStatus;
                    const liveQuota = accountQuotaMap[account.id];
                    const imageGenLimit = extractImageGenLimit(account);
                    const imageGenRemaining = liveQuota?.image_gen_remaining ?? imageGenLimit.remaining;
                    const imageGenRestore = formatRestoreAt(liveQuota?.image_gen_reset_after || imageGenLimit.resetAfter);
                    const isQuotaRefreshing = quotaRefreshingId === account.id;

                    return (
                      <tr
                        key={account.id}
                        className="border-b border-stone-100/80 text-sm text-stone-600 transition-colors hover:bg-stone-50/70"
                      >
                        <td className="px-4 py-3 text-center">
                          <Checkbox
                            checked={selectedIds.includes(account.id)}
                            onCheckedChange={(checked) => {
                              setSelectedIds((prev) =>
                                checked
                                  ? Array.from(new Set([...prev, account.id]))
                                  : prev.filter((item) => item !== account.id),
                              );
                            }}
                          />
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex items-center gap-2 whitespace-nowrap">
                            <span className="font-medium tracking-tight text-stone-700">
                              {maskToken(account.access_token)}
                            </span>
                            <button
                              type="button"
                              className="rounded-lg p-1 text-stone-400 transition hover:bg-stone-100 hover:text-stone-700"
                              onClick={() => {
                                void navigator.clipboard.writeText(account.access_token);
                                toast.success("token 已复制");
                              }}
                            >
                              <Copy className="size-4" />
                            </button>
                          </div>
                          <div className="mt-1 space-y-0.5 text-xs">
                            <div className="truncate text-stone-500" title={account.email ?? ""}>
                              {account.email ?? "—"}
                            </div>
                            <div className="truncate text-stone-400" title={account.fileName}>
                              {account.fileName}
                            </div>
                            {account.note ? (
                              <div className="truncate text-stone-400" title={account.note}>
                                {account.note}
                              </div>
                            ) : null}
                          </div>
                        </td>
                        <td className="px-4 py-3 text-center whitespace-nowrap">
                          <Badge variant="secondary" className="rounded-md bg-stone-100 text-stone-700">
                            {account.type}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-center whitespace-nowrap">
                          <Badge
                            variant={status.badge}
                            className="inline-flex items-center gap-1 rounded-md px-2 py-1"
                          >
                            <StatusIcon className="size-3.5" />
                            {account.status}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-center whitespace-nowrap">
                          {syncState ? (
                            <Badge variant={syncMeta[syncState].badge} className="rounded-md px-2 py-1">
                              {syncMeta[syncState].label}
                            </Badge>
                          ) : (
                            <span className="text-xs text-stone-400">—</span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-center">
                          <div className="flex flex-col items-center gap-2">
                            <div className="flex items-center gap-2">
                              <Badge variant="info" className="rounded-md">
                                {imageGenRemaining == null ? "—" : formatQuota(imageGenRemaining)}
                              </Badge>
                              <button
                                type="button"
                                className="rounded-lg p-1 text-stone-400 transition hover:bg-stone-100 hover:text-stone-700"
                                onClick={() => void handleRefreshAccountQuota(account)}
                                disabled={isQuotaRefreshing}
                              >
                                <RefreshCw className={cn("size-3.5", isQuotaRefreshing ? "animate-spin" : "")} />
                              </button>
                            </div>
                            <div className="text-[11px] text-stone-400">本地额度 {formatQuota(account.quota)}</div>
                          </div>
                        </td>
                        <td className="px-4 py-3 text-center text-xs text-stone-500 whitespace-nowrap">
                          {imageGenRestore.relative ? (
                            <div
                              className="flex items-center justify-center gap-2"
                              title={imageGenRestore.absolute !== "—" ? imageGenRestore.absolute : undefined}
                            >
                              <span className="font-medium text-stone-700">{imageGenRestore.relative}</span>
                              <span className="text-stone-300">·</span>
                              <span className="font-mono tabular-nums text-stone-400">{imageGenRestore.absoluteShort}</span>
                            </div>
                          ) : (
                            <div className="truncate font-mono tabular-nums text-stone-400" title={imageGenRestore.absolute}>
                              {imageGenRestore.absoluteShort}
                            </div>
                          )}
                        </td>
                        <td className="px-4 py-3 text-center text-stone-500 whitespace-nowrap">{account.success}</td>
                        <td className="px-4 py-3 text-center text-stone-500 whitespace-nowrap">{account.fail}</td>
                        <td className="px-4 py-3 text-center">
                          <div className="flex items-center justify-center gap-1 text-stone-400">
                            <button
                              type="button"
                              className="rounded-lg p-2 transition hover:bg-stone-100 hover:text-stone-700"
                              onClick={() => openEditDialog(account)}
                              disabled={isUpdating}
                            >
                              <Pencil className="size-4" />
                            </button>
                            <button
                              type="button"
                              className="rounded-lg p-2 transition hover:bg-rose-50 hover:text-rose-500"
                              onClick={() => void handleDeleteTokens([account.access_token])}
                              disabled={isDeleting}
                            >
                              <Trash2 className="size-4" />
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>

              {!isLoading && currentRows.length === 0 ? (
                <div className="flex flex-col items-center justify-center gap-3 px-6 py-14 text-center">
                  <div className="rounded-xl bg-stone-100 p-3 text-stone-500">
                    <Search className="size-5" />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium text-stone-700">没有匹配的账户</p>
                    <p className="text-sm text-stone-500">调整筛选条件或搜索关键字后重试。</p>
                  </div>
                </div>
              ) : null}
            </div>

            <div className="border-t border-stone-100 px-4 py-4">
              <div className="flex items-center justify-center gap-3 overflow-x-auto whitespace-nowrap">
                <div className="shrink-0 text-sm text-stone-500">
                  显示第 {filteredAccounts.length === 0 ? 0 : startIndex + 1} -{" "}
                  {Math.min(startIndex + Number(pageSize), filteredAccounts.length)} 条，共 {filteredAccounts.length} 条
                </div>

                <span className="shrink-0 text-sm leading-none text-stone-500">
                  {safePage} / {pageCount} 页
                </span>
                <Select
                  value={pageSize}
                  onValueChange={(value) => {
                    setPageSize(value);
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="h-10 w-[108px] shrink-0 rounded-lg border-stone-200 bg-white text-sm leading-none">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="10">10 / 页</SelectItem>
                    <SelectItem value="20">20 / 页</SelectItem>
                    <SelectItem value="50">50 / 页</SelectItem>
                    <SelectItem value="100">100 / 页</SelectItem>
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  size="icon"
                  className="size-10 shrink-0 rounded-lg border-stone-200 bg-white"
                  disabled={safePage <= 1}
                  onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                >
                  <ChevronLeft className="size-4" />
                </Button>
                {paginationItems.map((item, index) =>
                  item === "..." ? (
                    <span key={`ellipsis-${index}`} className="px-1 text-sm text-stone-400">
                      ...
                    </span>
                  ) : (
                    <Button
                      key={item}
                      variant={item === safePage ? "default" : "outline"}
                      className={cn(
                        "h-10 min-w-10 shrink-0 rounded-lg px-3",
                        item === safePage
                          ? "bg-stone-950 text-white hover:bg-stone-800"
                          : "border-stone-200 bg-white text-stone-700",
                      )}
                      onClick={() => setPage(item)}
                    >
                      {item}
                    </Button>
                  ),
                )}
                <Button
                  variant="outline"
                  size="icon"
                  className="size-10 shrink-0 rounded-lg border-stone-200 bg-white"
                  disabled={safePage >= pageCount}
                  onClick={() => setPage((prev) => Math.min(pageCount, prev + 1))}
                >
                  <ChevronRight className="size-4" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}