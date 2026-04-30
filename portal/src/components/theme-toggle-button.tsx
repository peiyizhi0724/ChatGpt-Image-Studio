"use client";

import { Moon, Sparkles, Sun } from "lucide-react";

import { useTheme } from "@/components/theme-provider";
import { cn } from "@/lib/utils";

type ThemeToggleButtonProps = {
  className?: string;
  iconClassName?: string;
};

export function ThemeToggleButton({
  className,
  iconClassName,
}: ThemeToggleButtonProps) {
  const { themeMode, toggleTheme } = useTheme();
  const modeMeta = {
    light: {
      icon: Sun,
      current: "浅色主题",
      next: "浅灰主题",
    },
    graphite: {
      icon: Moon,
      current: "浅灰主题",
      next: "深黑主题",
    },
    dark: {
      icon: Sparkles,
      current: "深黑主题",
      next: "浅色主题",
    },
  } as const;
  const currentMode = modeMeta[themeMode];
  const Icon = currentMode.icon;
  const label = `当前${currentMode.current}，点击切换到${currentMode.next}`;

  return (
    <button
      type="button"
      onClick={toggleTheme}
      className={cn(
        "inline-flex items-center justify-center rounded-2xl border border-stone-200 bg-white text-stone-700 shadow-sm transition hover:bg-stone-50 hover:text-stone-950 dark:border-[var(--studio-border)] dark:bg-[var(--studio-panel-soft)] dark:text-[var(--studio-text-strong)] dark:hover:bg-[var(--studio-panel-muted)]",
        className,
      )}
      aria-label={label}
      title={label}
    >
      <Icon className={cn("size-4", iconClassName)} />
    </button>
  );
}
