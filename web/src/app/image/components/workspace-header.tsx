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
    <div className="border-b border-stone-200/80 py-2.5 transition-colors duration-200 dark:border-[var(--studio-border)] sm:py-3">
      <div className="mx-auto min-w-0 max-w-[1120px] px-4 sm:px-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <h1 className="text-lg font-semibold tracking-tight text-stone-950 dark:text-[var(--studio-text-strong)] sm:text-[20px]">
              图片工作台
            </h1>
            {selectedConversationTitle ? (
              <span className="max-w-[220px] truncate rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-stone-600 dark:bg-[var(--studio-panel-muted)] dark:text-[var(--studio-text)]">
                {selectedConversationTitle}
              </span>
            ) : null}
            <div className="hide-scrollbar min-w-0 flex flex-nowrap items-center gap-2 overflow-x-auto pb-0.5">
              <span className="rounded-full bg-sky-50 px-2.5 py-1 text-[11px] font-medium text-sky-700 dark:bg-[var(--studio-panel-muted)] dark:text-[var(--studio-text)]">
            并发 {runningCount}/{maxRunningCount || 0}
              </span>
              <span className="rounded-full bg-amber-50 px-2.5 py-1 text-[11px] font-medium text-amber-700 dark:bg-[var(--studio-panel-muted)] dark:text-[var(--studio-text)]">
            排队 {queuedCount}
              </span>
              <span className="rounded-full bg-emerald-50 px-2.5 py-1 text-[11px] font-medium text-emerald-700 dark:bg-[var(--studio-panel-muted)] dark:text-[var(--studio-text)]">
            工作台 {workspaceActiveCount}
              </span>
              <span className="rounded-full bg-indigo-50 px-2.5 py-1 text-[11px] font-medium text-indigo-700 dark:bg-[var(--studio-panel-muted)] dark:text-[var(--studio-text)]">
            兼容 {compatActiveCount}
              </span>
              {cancelledCount > 0 ? (
                <span className="rounded-full bg-stone-100 px-2.5 py-1 text-[11px] font-medium text-stone-600 dark:bg-[var(--studio-panel-muted)] dark:text-[var(--studio-text)]">
                  已取消 {cancelledCount}
                </span>
              ) : null}
              {expiredCount > 0 ? (
                <span className="rounded-full bg-rose-50 px-2.5 py-1 text-[11px] font-medium text-rose-700 dark:bg-[var(--studio-panel-muted)] dark:text-[var(--studio-text)]">
                  已过期 {expiredCount}
                </span>
              ) : null}
            </div>
          </div>
          {showHistoryToggle && onToggleHistory ? (
            <Button
              type="button"
              variant="outline"
              className="h-8 shrink-0 rounded-full border-stone-200 bg-white px-3 text-stone-700 shadow-none dark:border-[var(--studio-border)] dark:bg-[var(--studio-panel-soft)] dark:text-[var(--studio-text)] sm:h-9 sm:px-4"
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
      </div>
    </div>
  );
}
