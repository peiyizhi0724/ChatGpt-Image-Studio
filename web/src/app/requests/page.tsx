"use client";

import { useEffect, useMemo, useState } from "react";
import { Activity, ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { fetchRequestLogs, type RequestLogItem } from "@/lib/api";
import { cn } from "@/lib/utils";

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

export default function RequestsPage() {
  const [items, setItems] = useState<RequestLogItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState("20");

  const loadItems = async () => {
    setIsLoading(true);
    try {
      const data = await fetchRequestLogs();
      setItems(data.items);
      setPage(1);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取调用请求失败");
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadItems();
  }, []);

  const pageCount = Math.max(1, Math.ceil(items.length / Number(pageSize)));
  const safePage = Math.min(page, pageCount);
  const startIndex = (safePage - 1) * Number(pageSize);
  const currentRows = items.slice(startIndex, startIndex + Number(pageSize));

  const paginationItems = useMemo(() => {
    const nextItems: (number | "...")[] = [];
    const start = Math.max(1, safePage - 1);
    const end = Math.min(pageCount, safePage + 1);

    if (start > 1) nextItems.push(1);
    if (start > 2) nextItems.push("...");
    for (let current = start; current <= end; current += 1) {
      nextItems.push(current);
    }
    if (end < pageCount - 1) nextItems.push("...");
    if (end < pageCount) nextItems.push(pageCount);

    return nextItems;
  }, [pageCount, safePage]);

  return (
    <section className="h-full">
      <div className="hide-scrollbar h-full min-h-0 overflow-y-auto rounded-[30px] border border-stone-200 bg-[#fcfcfb] px-4 py-5 shadow-[0_14px_40px_rgba(15,23,42,0.05)] transition-colors duration-200 dark:border-[var(--studio-border)] dark:bg-[var(--studio-panel)] sm:px-5 sm:py-6 lg:flex lg:min-h-0 lg:flex-col lg:px-6 lg:py-7">
        <section className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-center gap-4">
            <div className="inline-flex size-12 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
              <Activity className="size-5" />
            </div>
            <div className="space-y-1">
              <h1 className="text-2xl font-semibold tracking-tight text-stone-950">
                调用请求
              </h1>
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            className="h-10 w-full rounded-full border-stone-200 bg-white px-4 text-stone-700 shadow-none sm:w-auto"
            onClick={() => void loadItems()}
            disabled={isLoading}
          >
            {isLoading ? (
              <RefreshCw className="size-4 animate-spin" />
            ) : (
              <RefreshCw className="size-4" />
            )}
            刷新记录
          </Button>
        </section>

        <Card className="mt-5 overflow-hidden rounded-2xl border-white/80 bg-white/90 shadow-sm lg:flex-1 lg:min-h-0">
          <CardContent className="p-0 lg:flex lg:h-full lg:min-h-0 lg:flex-col">
                <div className="space-y-4 p-4 lg:hidden">
                  {currentRows.map((item) => (
                    <div
                      key={item.id}
                      className="rounded-2xl border border-stone-200/80 bg-stone-50/60 p-4"
                    >
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="min-w-0">
                          <div className="font-medium text-stone-700">
                            {formatTime(item.startedAt)}
                          </div>
                          <div className="text-xs text-stone-400">
                            {item.finishedAt
                              ? formatTime(item.finishedAt)
                              : "进行中"}
                          </div>
                        </div>
                        <Badge
                          variant={item.success ? "success" : "danger"}
                          className="w-fit shrink-0 rounded-md px-2 py-1"
                        >
                          {item.success ? "成功" : "失败"}
                        </Badge>
                      </div>

                      <div className="mt-3 flex flex-wrap gap-2">
                        <Badge
                          variant="secondary"
                          className="rounded-md bg-stone-100 text-stone-700"
                        >
                          {item.operation || "—"}
                        </Badge>
                        <Badge
                          variant="secondary"
                          className="rounded-md bg-stone-100 text-stone-700"
                        >
                          {item.imageMode || "studio"}
                        </Badge>
                        <Badge
                          variant={
                            item.direction === "cpa" ? "info" : "success"
                          }
                          className="rounded-md px-2 py-1"
                        >
                          {item.direction === "cpa" ? "CPA" : "官方"}
                        </Badge>
                      </div>

                      <div className="mt-3 grid grid-cols-1 gap-3 text-sm text-stone-600 sm:grid-cols-2">
                        <div className="rounded-xl bg-white px-3 py-2">
                          <div className="text-[11px] uppercase tracking-[0.14em] text-stone-400">
                            路由
                          </div>
                          <div className="mt-1 text-stone-700">
                            {item.route || "—"}
                          </div>
                          <div className="mt-1 text-xs text-stone-400">
                            CPA 子路由：{item.cpaSubroute || "—"}
                          </div>
                        </div>
                        <div className="rounded-xl bg-white px-3 py-2">
                          <div className="text-[11px] uppercase tracking-[0.14em] text-stone-400">
                            接口
                          </div>
                          <div className="mt-1 break-all text-stone-700">
                            {item.endpoint || "—"}
                          </div>
                        </div>
                        <div className="rounded-xl bg-white px-3 py-2">
                          <div className="text-[11px] uppercase tracking-[0.14em] text-stone-400">
                            参数
                          </div>
                          <div className="mt-1 text-stone-700">
                            {item.size || "—"}
                          </div>
                          <div className="mt-1 text-xs text-stone-400">
                            {item.quality
                              ? `quality: ${item.quality}`
                              : "quality: —"}
                          </div>
                          <div className="text-xs text-stone-400">
                            {typeof item.promptLength === "number" &&
                            item.promptLength > 0
                              ? `prompt: ${item.promptLength} 字`
                              : "prompt: —"}
                          </div>
                        </div>
                        <div className="rounded-xl bg-white px-3 py-2">
                          <div className="text-[11px] uppercase tracking-[0.14em] text-stone-400">
                            账号
                          </div>
                          <div
                            className="mt-1 truncate text-stone-700"
                            title={item.accountEmail || item.accountFile || ""}
                          >
                            {item.accountEmail || "—"}
                          </div>
                          <div
                            className="mt-1 truncate text-xs text-stone-400"
                            title={item.accountFile || ""}
                          >
                            {item.accountType
                              ? `${item.accountType} · ${item.accountFile || "—"}`
                              : item.accountFile || "—"}
                          </div>
                        </div>
                        <div className="rounded-xl bg-white px-3 py-2">
                          <div className="text-[11px] uppercase tracking-[0.14em] text-stone-400">
                            模型
                          </div>
                          <div className="mt-1 break-all text-stone-700">
                            {item.requestedModel || "—"}
                          </div>
                          <div className="mt-1 text-xs text-stone-400">
                            {item.upstreamModel || "—"}
                          </div>
                          <div className="text-xs text-stone-400">
                            {item.imageToolModel
                              ? `tool: ${item.imageToolModel}`
                              : "—"}
                          </div>
                        </div>
                        <div className="rounded-xl bg-white px-3 py-2 sm:col-span-2">
                          <div className="text-[11px] uppercase tracking-[0.14em] text-stone-400">
                            错误
                          </div>
                          <div className="mt-1 break-words text-xs leading-6 text-stone-500">
                            {item.error || "—"}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>

                <div className="hidden lg:flex lg:min-h-0 lg:flex-1 lg:flex-col">
                  <div className="min-h-0 flex-1 overflow-auto border-t border-stone-100">
                    <div className="min-h-full">
                      <table className="w-full min-w-[1240px] text-left">
                        <thead className="sticky top-0 z-10 border-b border-stone-100 bg-white/95 text-[11px] uppercase tracking-[0.18em] text-stone-400 backdrop-blur-sm">
                          <tr>
                            <th className="px-4 py-3 whitespace-nowrap">时间</th>
                            <th className="px-4 py-3 whitespace-nowrap">操作</th>
                            <th className="px-4 py-3 whitespace-nowrap">模式</th>
                            <th className="px-4 py-3 whitespace-nowrap">方向</th>
                            <th className="px-4 py-3 whitespace-nowrap">路由</th>
                            <th className="px-4 py-3 whitespace-nowrap">
                              CPA 子路由
                            </th>
                            <th className="px-4 py-3 whitespace-nowrap">接口</th>
                            <th className="px-4 py-3 whitespace-nowrap">参数</th>
                            <th className="px-4 py-3 whitespace-nowrap">账号</th>
                            <th className="px-4 py-3 whitespace-nowrap">模型</th>
                            <th className="px-4 py-3 whitespace-nowrap">结果</th>
                            <th className="px-4 py-3">错误</th>
                          </tr>
                        </thead>
                        <tbody>
                          {currentRows.map((item) => (
                            <tr
                              key={item.id}
                              className="border-b border-stone-100/80 text-sm text-stone-600 hover:bg-stone-50/70"
                            >
                              <td className="px-4 py-3 whitespace-nowrap">
                                <div className="font-medium text-stone-700">
                                  {formatTime(item.startedAt)}
                                </div>
                                <div className="text-xs text-stone-400">
                                  {item.finishedAt
                                    ? formatTime(item.finishedAt)
                                    : "进行中"}
                                </div>
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                {item.operation || "—"}
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                <Badge
                                  variant="secondary"
                                  className="rounded-md bg-stone-100 text-stone-700"
                                >
                                  {item.imageMode || "studio"}
                                </Badge>
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                <Badge
                                  variant={
                                    item.direction === "cpa" ? "info" : "success"
                                  }
                                  className="rounded-md px-2 py-1"
                                >
                                  {item.direction === "cpa" ? "CPA" : "官方"}
                                </Badge>
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                {item.route || "—"}
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                {item.cpaSubroute || "—"}
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                {item.endpoint || "—"}
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                <div className="text-stone-700">
                                  {item.size || "—"}
                                </div>
                                <div className="text-xs text-stone-400">
                                  {item.quality
                                    ? `quality: ${item.quality}`
                                    : "quality: —"}
                                </div>
                                <div className="text-xs text-stone-400">
                                  {typeof item.promptLength === "number" &&
                                  item.promptLength > 0
                                    ? `prompt: ${item.promptLength} 字`
                                    : "prompt: —"}
                                </div>
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                <div
                                  className="truncate text-stone-700"
                                  title={
                                    item.accountEmail || item.accountFile || ""
                                  }
                                >
                                  {item.accountEmail || "—"}
                                </div>
                                <div
                                  className="truncate text-xs text-stone-400"
                                  title={item.accountFile || ""}
                                >
                                  {item.accountType
                                    ? `${item.accountType} · ${item.accountFile || "—"}`
                                    : item.accountFile || "—"}
                                </div>
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                <div className="text-stone-700">
                                  {item.requestedModel || "—"}
                                </div>
                                <div className="text-xs text-stone-400">
                                  {item.upstreamModel || "—"}
                                </div>
                                <div className="text-xs text-stone-400">
                                  {item.imageToolModel
                                    ? `tool: ${item.imageToolModel}`
                                    : "—"}
                                </div>
                              </td>
                              <td className="px-4 py-3 whitespace-nowrap">
                                <Badge
                                  variant={item.success ? "success" : "danger"}
                                  className="rounded-md px-2 py-1"
                                >
                                  {item.success ? "成功" : "失败"}
                                </Badge>
                              </td>
                              <td className="px-4 py-3">
                                <div
                                  className="max-w-[320px] truncate text-xs text-stone-500"
                                  title={item.error || ""}
                                >
                                  {item.error || "—"}
                                </div>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>

                {items.length > 0 ? (
                  <div className="border-t border-stone-100 px-4 py-4">
                    <div className="flex items-center justify-center gap-3 overflow-x-auto whitespace-nowrap">
                      <div className="shrink-0 text-sm text-stone-500">
                        显示第 {startIndex + 1} -{" "}
                        {Math.min(startIndex + Number(pageSize), items.length)}{" "}
                        条，共 {items.length} 条
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
                          <span
                            key={`ellipsis-${index}`}
                            className="px-1 text-sm text-stone-400"
                          >
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
                        onClick={() =>
                          setPage((prev) => Math.min(pageCount, prev + 1))
                        }
                      >
                        <ChevronRight className="size-4" />
                      </Button>
                    </div>
                  </div>
                ) : null}

                {!isLoading && items.length === 0 ? (
                  <div className="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center lg:flex-1">
                    <div className="rounded-2xl bg-stone-100 p-3 text-stone-500">
                      <Activity className="size-5" />
                    </div>
                    <div className="space-y-1">
                      <p className="text-sm font-medium text-stone-700">
                        还没有调用记录
                      </p>
                      <p className="text-sm text-stone-500">
                        发起一次图片请求后，这里会显示它到底走的是官方还是 CPA。
                      </p>
                    </div>
                  </div>
                ) : null}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
