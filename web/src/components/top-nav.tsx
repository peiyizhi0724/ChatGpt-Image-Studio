"use client";

import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { Activity, ImageIcon, LogOut, PanelLeftClose, PanelLeftOpen, Settings2, Shield, ShieldCheck, Sparkles } from "lucide-react";

import { fetchVersionInfo } from "@/lib/api";
import { clearStoredAuthKey } from "@/store/auth";
import { cn } from "@/lib/utils";

const repositoryUrl = "https://github.com/peiyizhi0724/ChatGpt-Image-Studio";

function formatVersionLabel(value: string) {
  const normalized = String(value || "")
    .trim()
    .replace(/^v+/i, "");
  return normalized ? `v${normalized}` : "读取中";
}

const navItems = [
  { href: "/startup-check", label: "启动体检", description: "连通性与配置自检", icon: ShieldCheck },
  { href: "/image", label: "图片工作台", description: "生成、编辑与放大", icon: ImageIcon },
  { href: "/accounts", label: "账号管理", description: "号池、额度与同步", icon: Shield },
  { href: "/settings", label: "配置管理", description: "模式、接口与后端配置", icon: Settings2 },
  { href: "/requests", label: "调用请求", description: "查看官方与 CPA 请求方向", icon: Activity },
];

type DesktopTopNavProps = {
  pathname: string;
  defaultCollapsed: boolean;
  versionLabel: string;
  onLogout: () => Promise<void>;
};

function DesktopTopNav({ pathname, defaultCollapsed, versionLabel, onLogout }: DesktopTopNavProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <aside className={cn("hidden shrink-0 transition-[width] duration-200 lg:flex", collapsed ? "w-[92px]" : "w-[228px]")}>
      <div className="flex h-full w-full flex-col rounded-[28px] border border-stone-200 bg-[#f0f0ed] p-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.65)]">
        <div className={cn("gap-2", collapsed ? "flex flex-col items-center" : "flex items-center justify-between")}>
          <Link
            to="/image"
            className={cn(
              "flex items-center rounded-2xl transition hover:bg-white/70",
              collapsed ? "justify-center px-0 py-1" : "min-w-0 flex-1 gap-3 px-3 py-3",
            )}
          >
            <span className={cn("flex items-center justify-center rounded-2xl bg-white text-stone-900 shadow-sm", collapsed ? "size-11" : "size-10")}>
              <Sparkles className={cn(collapsed ? "size-5" : "size-4")} />
            </span>
            {!collapsed ? (
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold tracking-tight text-stone-900">
                  ChatGpt Image Studio
                </span>
                <span className="block truncate text-xs text-stone-500">简洁的图片工作区</span>
              </span>
            ) : null}
          </Link>

          <button
            type="button"
            className={cn(
              "inline-flex items-center justify-center rounded-2xl border border-stone-200 bg-white text-stone-600 transition hover:bg-stone-50 hover:text-stone-900",
              collapsed ? "size-11" : "size-10",
            )}
            onClick={() => setCollapsed((current) => !current)}
            aria-label={collapsed ? "展开导航" : "收起导航"}
          >
            {collapsed ? <PanelLeftOpen className="size-5" /> : <PanelLeftClose className="size-4" />}
          </button>
        </div>

        <nav className="mt-4 space-y-1">
          {navItems.map((item) => {
            const active = pathname === item.href;
            const Icon = item.icon;
            return (
              <Link
                key={item.href}
                to={item.href}
                className={cn(
                  "flex rounded-2xl transition",
                  collapsed ? "justify-center px-0 py-3.5" : "items-center gap-3 px-3 py-3",
                  active ? "bg-white text-stone-950 shadow-sm" : "text-stone-600 hover:bg-white/65 hover:text-stone-900",
                )}
                title={collapsed ? item.label : undefined}
              >
                <span
                  className={cn(
                    "flex items-center justify-center rounded-2xl",
                    collapsed ? "size-11" : "size-9",
                    active ? "bg-stone-950 text-white" : "bg-white/80 text-stone-600",
                  )}
                >
                  <Icon className={cn(collapsed ? "size-5" : "size-4")} />
                </span>
                {!collapsed ? (
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">{item.label}</span>
                    <span className="block truncate text-xs text-stone-500">{item.description}</span>
                  </span>
                ) : null}
              </Link>
            );
          })}
        </nav>

        <div className="mt-auto space-y-3">
          <a
            href={repositoryUrl}
            target="_blank"
            rel="noreferrer"
            className={cn(
              "block rounded-2xl bg-white/70 text-xs text-stone-500 shadow-sm transition hover:bg-white hover:text-stone-700",
              collapsed ? "px-2 py-3 text-center" : "px-4 py-3",
            )}
            title="打开 GitHub 仓库"
          >
            {!collapsed ? <div className="font-medium text-stone-700">版本</div> : null}
            <div className={cn(!collapsed ? "mt-1" : "font-medium")}>{versionLabel}</div>
          </a>
          <button
            type="button"
            className={cn(
              "flex w-full items-center rounded-2xl border border-stone-200 bg-white text-sm font-medium text-stone-700 transition hover:bg-stone-50",
              collapsed ? "justify-center px-0 py-3" : "justify-center gap-2 px-4 py-3",
            )}
            onClick={() => void onLogout()}
            title={collapsed ? "退出登录" : undefined}
          >
            <LogOut className="size-4" />
            {!collapsed ? "退出登录" : null}
          </button>
        </div>
      </div>
    </aside>
  );
}

export function TopNav() {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const isImageRoute = pathname === "/image" || pathname?.startsWith("/image/");
  const [versionLabel, setVersionLabel] = useState("读取中");

  useEffect(() => {
    let cancelled = false;

    const loadVersion = async () => {
      try {
        const payload = await fetchVersionInfo();
        if (!cancelled) {
          setVersionLabel(formatVersionLabel(payload.version));
        }
      } catch {
        if (!cancelled) {
          setVersionLabel("未知版本");
        }
      }
    };

    void loadVersion();
    return () => {
      cancelled = true;
    };
  }, []);

  const handleLogout = async () => {
    await clearStoredAuthKey();
    navigate("/login", { replace: true });
  };

  if (pathname === "/login" || pathname === "/login.html" || pathname.startsWith("/login/")) {
    return null;
  }

  return (
    <>
      <header className="lg:hidden">
        <div className="rounded-[26px] border border-stone-200 bg-[#f0f0ed] p-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.65)]">
          <div className="flex items-center justify-between gap-3">
            <Link
              to="/image"
              className="flex min-w-0 items-center gap-3 rounded-2xl px-1 py-1 transition hover:bg-white/70"
            >
              <span className="flex size-10 items-center justify-center rounded-2xl bg-white text-stone-900 shadow-sm">
                <Sparkles className="size-4" />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold tracking-tight text-stone-900">
                  ChatGpt Image Studio
                </span>
                <span className="block truncate text-xs text-stone-500">简洁的图片工作区</span>
              </span>
            </Link>
            <button
              type="button"
              className="inline-flex h-10 shrink-0 items-center justify-center rounded-2xl border border-stone-200 bg-white px-3 text-sm font-medium text-stone-700 transition hover:bg-stone-50"
              onClick={() => void handleLogout()}
            >
              <LogOut className="size-4" />
            </button>
          </div>

          <nav className="mt-3 grid grid-cols-2 gap-2">
            {navItems.map((item) => {
              const active = pathname === item.href;
              const Icon = item.icon;
              return (
                <Link
                  key={item.href}
                  to={item.href}
                  className={cn(
                    "flex items-center gap-3 rounded-2xl px-3 py-3 transition",
                    active ? "bg-white text-stone-950 shadow-sm" : "bg-white/60 text-stone-600 hover:bg-white hover:text-stone-900",
                  )}
                >
                  <span
                    className={cn(
                      "flex size-9 items-center justify-center rounded-xl",
                      active ? "bg-stone-950 text-white" : "bg-white text-stone-600",
                    )}
                  >
                    <Icon className="size-4" />
                  </span>
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium">{item.label}</span>
                    <span className="block truncate text-xs text-stone-500">{item.description}</span>
                  </span>
                </Link>
              );
            })}
          </nav>
        </div>
      </header>
      <DesktopTopNav
        key={isImageRoute ? "image-route" : "non-image-route"}
        pathname={pathname}
        defaultCollapsed={isImageRoute}
        versionLabel={versionLabel}
        onLogout={handleLogout}
      />
    </>
  );
}
