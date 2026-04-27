"use client";

import { PanelLeftClose, PanelLeftOpen } from "lucide-react";

import { Button } from "@/components/ui/button";
type WorkspaceHeaderProps = {
  historyCollapsed?: boolean;
  selectedConversationTitle?: string | null;
  runningCount?: number;
  maxRunningCount?: number;
  queuedCount?: number;
  workspaceActiveCount?: number;
  compatActiveCount?: number;
  cancelledCount?: number;
  expiredCount?: number;
  onToggleHistory?: () => void;
  showHistoryToggle?: boolean;
};

export function WorkspaceHeader({
  historyCollapsed = false,
  selectedConversationTitle,
  runningCount = 0,
  maxRunningCount = 0,
  queuedCount = 0,
  workspaceActiveCount = 0,
  compatActiveCount = 0,
  cancelledCount = 0,
  expiredCount = 0,
  onToggleHistory,
  showHistoryToggle = true,
}: WorkspaceHeaderProps) {
  return (
    <div className="border-b border-stone-200/80 px-4 py-3 sm:px-5 sm:py-4">
      <div className="min-w-0">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <h1 className="text-lg font-semibold tracking-tight text-stone-950 sm:text-[22px]">
              图片工作台
            </h1>
            {selectedConversationTitle ? (
              <span className="truncate rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-stone-600">
                {selectedConversationTitle}
              </span>
            ) : null}
          </div>
          {showHistoryToggle && onToggleHistory ? (
            <Button
              type="button"
              variant="outline"
              className="h-9 shrink-0 rounded-full border-stone-200 bg-white px-3 text-stone-700 shadow-none sm:h-10 sm:px-4"
              onClick={onToggleHistory}
            >
              {historyCollapsed ? (
                <PanelLeftOpen className="size-4" />
              ) : (
                <PanelLeftClose className="size-4" />
              )}
              <span className="hidden sm:inline">
                {historyCollapsed ? "展开历史" : "收起历史"}
              </span>
            </Button>
          ) : null}
        </div>
        <div className="hide-scrollbar mt-3 flex flex-nowrap items-center gap-2 overflow-x-auto pb-1">
          <span className="rounded-full bg-sky-50 px-3 py-1 text-xs font-medium text-sky-700">
            并发 {runningCount}/{maxRunningCount || 0}
          </span>
          <span className="rounded-full bg-amber-50 px-3 py-1 text-xs font-medium text-amber-700">
            排队 {queuedCount}
          </span>
          <span className="rounded-full bg-emerald-50 px-3 py-1 text-xs font-medium text-emerald-700">
            工作台 {workspaceActiveCount}
          </span>
          <span className="rounded-full bg-indigo-50 px-3 py-1 text-xs font-medium text-indigo-700">
            兼容 {compatActiveCount}
          </span>
          {cancelledCount > 0 ? (
            <span className="rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-stone-600">
              已取消 {cancelledCount}
            </span>
          ) : null}
          {expiredCount > 0 ? (
            <span className="rounded-full bg-rose-50 px-3 py-1 text-xs font-medium text-rose-700">
              已过期 {expiredCount}
            </span>
          ) : null}
        </div>
      </div>
    </div>
  );
}
