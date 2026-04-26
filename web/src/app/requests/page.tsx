"use client";

import { useEffect, useState } from "react";
import { Activity, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { fetchRequestLogs, type RequestLogItem } from "@/lib/api";

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value || "—";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function formatDirection(value: string) {
  if (value === "cpa") {
    return "CPA";
  }
  if (value === "preflight") {
    return "预检";
  }
  if (value === "admission") {
    return "调度";
  }
  return "官方";
}

function formatSortMode(value?: string) {
  if (value === "imported_at") {
    return "导入时间";
  }
  if (value === "name") {
    return "名字字母";
  }
  if (value === "quota") {
    return "剩余额度";
  }
  return value || "—";
}

function formatRoutingSummary(item: RequestLogItem) {
  if (!item.routingPolicyApplied) {
    return "默认选号";
  }
  const parts = [`第 ${Number(item.routingGroupIndex ?? 0) + 1} 组`, formatSortMode(item.routingSortMode)];
  if (typeof item.routingReservePercent === "number") {
    parts.push(`保底 ${item.routingReservePercent}%`);
  }
  return parts.join(" / ");
}

function formatRuntimeSummary(item: RequestLogItem) {
  const parts: string[] = [];
  if (typeof item.inflightCountAtStart === "number") {
    parts.push(`起始并发 ${item.inflightCountAtStart}`);
  }
  if (typeof item.queueWaitMs === "number") {
    parts.push(`排队 ${item.queueWaitMs}ms`);
  }
  if (item.leaseAcquired) {
    parts.push("已租约");
  }
  return parts.length > 0 ? parts.join(" / ") : "—";
}

function formatParams(item: RequestLogItem) {
  const parts: string[] = [];
  if (item.size) {
    parts.push(item.size);
  }
  if (item.quality) {
    parts.push(`quality ${item.quality}`);
  }
  if (typeof item.promptLength === "number" && item.promptLength > 0) {
    parts.push(`prompt ${item.promptLength} 字`);
  }
  return parts.length > 0 ? parts.join(" / ") : "—";
}

function formatAccount(item: RequestLogItem) {
  const main = item.accountEmail || item.accountFile || "—";
  const sub = item.accountType ? `${item.accountType} / ${item.accountFile || "—"}` : item.accountFile || "—";
  return { main, sub };
}

function formatModel(item: RequestLogItem) {
  return {
    requested: item.requestedModel || "—",
    upstream: item.upstreamModel || "—",
    tool: item.imageToolModel ? `tool: ${item.imageToolModel}` : "—",
  };
}

export default function RequestsPage() {
  const [items, setItems] = useState<RequestLogItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  const loadItems = async () => {
    setIsLoading(true);
    try {
      const data = await fetchRequestLogs();
      setItems(data.items);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取调用请求失败");
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadItems();
  }, []);

  return (
    <section className="h-full overflow-y-auto">
      <div className="mx-auto flex max-w-[1520px] flex-col gap-6 px-1 py-1">
        <div className="rounded-[30px] border border-stone-200 bg-white px-5 py-5 shadow-[0_14px_40px_rgba(15,23,42,0.05)] sm:px-6">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0">
              <div className="flex items-start gap-4">
                <div className="inline-flex size-12 shrink-0 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
                  <Activity className="size-5" />
                </div>
                <div className="min-w-0">
                  <h1 className="text-2xl font-semibold tracking-tight text-stone-950">调用请求</h1>
                  <p className="mt-2 max-w-[900px] text-sm leading-7 text-stone-500">
                    这里记录最近的图片请求实际走向。除了官方或 CPA 方向，您还可以直接看到是否启用了本地分组策略、命中了第几组、是否排队以及错误码。
                  </p>
                </div>
              </div>
            </div>
            <Button
              type="button"
              variant="outline"
              className="h-10 rounded-full border-stone-200 bg-white px-4 text-stone-700 shadow-none"
              onClick={() => void loadItems()}
              disabled={isLoading}
            >
              {isLoading ? <RefreshCw className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
              刷新记录
            </Button>
          </div>
        </div>

        <Card className="border-stone-200 bg-white shadow-[0_18px_48px_rgba(15,23,42,0.05)]">
          <CardContent className="p-0">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[1500px] text-left">
                <thead className="border-b border-stone-100 bg-stone-50/80 text-[11px] uppercase tracking-[0.18em] text-stone-400">
                  <tr>
                    <th className="px-4 py-3 whitespace-nowrap">时间</th>
                    <th className="px-4 py-3 whitespace-nowrap">操作</th>
                    <th className="px-4 py-3 whitespace-nowrap">模式</th>
                    <th className="px-4 py-3 whitespace-nowrap">方向</th>
                    <th className="px-4 py-3 whitespace-nowrap">路由</th>
                    <th className="px-4 py-3 whitespace-nowrap">分组路由</th>
                    <th className="px-4 py-3 whitespace-nowrap">CPA 子路由</th>
                    <th className="px-4 py-3 whitespace-nowrap">并发运行</th>
                    <th className="px-4 py-3 whitespace-nowrap">接口</th>
                    <th className="px-4 py-3 whitespace-nowrap">参数</th>
                    <th className="px-4 py-3 whitespace-nowrap">账号</th>
                    <th className="px-4 py-3 whitespace-nowrap">模型</th>
                    <th className="px-4 py-3 whitespace-nowrap">结果</th>
                    <th className="px-4 py-3 whitespace-nowrap">错误</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((item) => {
                    const account = formatAccount(item);
                    const model = formatModel(item);

                    return (
                      <tr key={item.id} className="border-b border-stone-100/80 text-sm text-stone-600 hover:bg-stone-50/70">
                        <td className="px-4 py-3 whitespace-nowrap">
                          <div className="font-medium text-stone-700">{formatTime(item.startedAt)}</div>
                          <div className="text-xs text-stone-400">{item.finishedAt ? formatTime(item.finishedAt) : "进行中"}</div>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">{item.operation || "—"}</td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <Badge variant="secondary" className="rounded-md bg-stone-100 text-stone-700">
                            {item.imageMode || "studio"}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <Badge
                            variant={item.direction === "cpa" ? "info" : item.direction === "official" ? "success" : "secondary"}
                            className="rounded-md px-2 py-1"
                          >
                            {formatDirection(item.direction)}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">{item.route || "—"}</td>
                        <td className="px-4 py-3">
                          <div className="text-stone-700">{formatRoutingSummary(item)}</div>
                          <div className="text-xs text-stone-400">
                            {item.routingPolicyApplied ? "本地浏览器分组策略已生效" : "未使用分组头，按默认规则选号"}
                          </div>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">{item.cpaSubroute || "—"}</td>
                        <td className="px-4 py-3">
                          <div className="text-stone-700">{formatRuntimeSummary(item)}</div>
                          <div className="text-xs text-stone-400">{item.errorCode ? `error_code: ${item.errorCode}` : "error_code: —"}</div>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">{item.endpoint || "—"}</td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <div className="text-stone-700">{formatParams(item)}</div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="max-w-[240px] truncate text-stone-700" title={account.main}>
                            {account.main}
                          </div>
                          <div className="max-w-[240px] truncate text-xs text-stone-400" title={account.sub}>
                            {account.sub}
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="text-stone-700">{model.requested}</div>
                          <div className="text-xs text-stone-400">{model.upstream}</div>
                          <div className="text-xs text-stone-400">{model.tool}</div>
                        </td>
                        <td className="px-4 py-3 whitespace-nowrap">
                          <Badge variant={item.success ? "success" : "danger"} className="rounded-md px-2 py-1">
                            {item.success ? "成功" : "失败"}
                          </Badge>
                        </td>
                        <td className="px-4 py-3">
                          <div className="max-w-[340px] truncate text-xs text-stone-500" title={item.error || ""}>
                            {item.error || "—"}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            {!isLoading && items.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center">
                <div className="rounded-2xl bg-stone-100 p-3 text-stone-500">
                  <Activity className="size-5" />
                </div>
                <div className="space-y-1">
                  <p className="text-sm font-medium text-stone-700">还没有调用记录</p>
                  <p className="text-sm text-stone-500">发起一次生图后，这里就会显示请求走向、分组命中和运行时信息。</p>
                </div>
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
