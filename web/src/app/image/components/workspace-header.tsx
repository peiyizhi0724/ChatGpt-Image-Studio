"use client";

import { PanelLeftClose, PanelLeftOpen } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { RuntimeStatusResponse } from "@/lib/api";

type WorkspaceHeaderProps = {
  historyCollapsed: boolean;
  selectedConversationTitle?: string | null;
  activeTaskCount: number;
  runtimeStatus?: RuntimeStatusResponse | null;
  onToggleHistory: () => void;
};

export function WorkspaceHeader({
  historyCollapsed,
  selectedConversationTitle,
  activeTaskCount,
  runtimeStatus,
  onToggleHistory,
}: WorkspaceHeaderProps) {
  return (
    <div className="border-b border-stone-200/80 px-5 py-4 sm:px-6">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            className="h-10 rounded-full border-stone-200 bg-white px-4 text-stone-700 shadow-none"
            onClick={onToggleHistory}
          >
            {historyCollapsed ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
            {historyCollapsed ? "展开历史" : "收起历史"}
          </Button>
          <h1 className="text-xl font-semibold tracking-tight text-stone-950 sm:text-[22px]">图片工作台</h1>
          {selectedConversationTitle ? (
            <span className="truncate rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-stone-600">
              {selectedConversationTitle}
            </span>
          ) : null}
          {activeTaskCount > 0 ? (
            <span className="rounded-full bg-amber-100 px-3 py-1 text-xs font-medium text-amber-700">
              进行中 {activeTaskCount}
            </span>
          ) : null}
        </div>

        {runtimeStatus ? (
          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs">
            <span className="rounded-full bg-stone-100 px-3 py-1 text-stone-700">
              并发 {runtimeStatus.admission.inflight}/{runtimeStatus.admission.maxConcurrency}
            </span>
            <span className="rounded-full bg-stone-100 px-3 py-1 text-stone-700">
              排队 {runtimeStatus.admission.queued}/{runtimeStatus.admission.queueLimit}
            </span>
            <span className="rounded-full bg-stone-100 px-3 py-1 text-stone-700">
              可用账号 {runtimeStatus.accounts.available}/{runtimeStatus.accounts.total}
            </span>
            <span className="rounded-full bg-stone-100 px-3 py-1 text-stone-700">
              近10分钟失败 {runtimeStatus.recent.failureCount}
            </span>
            {runtimeStatus.recent.lastError ? (
              <span
                className="max-w-[560px] truncate rounded-full bg-rose-100 px-3 py-1 text-rose-700"
                title={runtimeStatus.recent.lastError}
              >
                最近错误: {runtimeStatus.recent.lastError}
              </span>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}
