"use client";

import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { LoaderCircle, Mail, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { loginPortal } from "@/lib/api";
import { usePortalSession } from "@/store/session";

export default function LoginPage() {
  const navigate = useNavigate();
  const { applySession } = usePortalSession();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleLogin = async () => {
    if (!email.trim()) {
      toast.error("请输入邮箱");
      return;
    }
    if (!password.trim()) {
      toast.error("请输入密码");
      return;
    }

    setIsSubmitting(true);
    try {
      const payload = await loginPortal(email, password);
      applySession(payload);
      navigate("/workspace", { replace: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : "登录失败";
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-[calc(100dvh-1.5rem)] w-full flex-col overflow-y-auto py-4 lg:h-full lg:min-h-0 lg:overflow-hidden">
      <div className="my-auto mx-auto grid w-full max-w-[1120px] overflow-hidden rounded-[32px] border border-stone-200 bg-white shadow-[0_24px_80px_rgba(15,23,42,0.08)] lg:grid-cols-[1.05fr_0.95fr]">
        <div className="hidden bg-[radial-gradient(circle_at_top_left,_rgba(255,255,255,0.78),_rgba(255,255,255,0.18)_38%,_rgba(28,25,23,0.08)_100%),linear-gradient(155deg,#111827_0%,#1f2937_52%,#374151_100%)] p-8 text-white lg:flex lg:flex-col lg:justify-between">
          <div className="flex items-center gap-3">
            <span className="flex size-11 items-center justify-center rounded-2xl bg-white/12 backdrop-blur">
              <Sparkles className="size-4" />
            </span>
            <div>
              <div className="text-sm font-semibold tracking-tight">Cheilins Studio</div>
              <div className="mt-1 text-xs text-white/65">多人共用的图片工作区入口</div>
            </div>
          </div>

          <div className="space-y-6">
            <div className="space-y-3">
              <div className="text-sm font-medium uppercase tracking-[0.24em] text-white/55">Portal</div>
              <h1 className="max-w-[420px] text-[40px] font-semibold leading-[1.1] tracking-tight">
                用邮箱登录，进入共享图片工作台。
              </h1>
              <p className="max-w-[430px] text-sm leading-7 text-white/72">
                工作台延续当前项目的图片生成、编辑和放大体验，管理员还能统一查看所有用户和总额度。
              </p>
            </div>

            <div className="grid gap-3 sm:grid-cols-3">
              {[
                ["注册", "邮箱注册后即可登录使用"],
                ["创作", "生成、改图、放大保留原体验"],
                ["协作", "管理员可统一管理所有用户"],
              ].map(([title, desc]) => (
                <div key={title} className="rounded-2xl border border-white/12 bg-white/6 p-4 backdrop-blur-sm">
                  <div className="text-sm font-semibold">{title}</div>
                  <div className="mt-2 text-xs leading-6 text-white/65">{desc}</div>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div className="flex items-center justify-center px-5 py-8 sm:px-8 lg:px-10">
          <div className="w-full max-w-[420px] space-y-8">
            <div className="space-y-4">
              <div className="inline-flex size-14 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
                <Mail className="size-5" />
              </div>
              <div className="space-y-2">
                <h1 className="text-3xl font-semibold tracking-tight text-stone-950">登录 Portal</h1>
                <p className="text-sm leading-7 text-stone-500">使用邮箱和密码进入多人图片工作区。</p>
              </div>
            </div>

            <div className="space-y-3">
              <label htmlFor="email" className="block text-sm font-medium text-stone-700">
                邮箱
              </label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="name@example.com"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between gap-3">
                <label htmlFor="password" className="block text-sm font-medium text-stone-700">
                  密码
                </label>
                <Link to="/forgot-password" className="text-sm font-medium text-stone-500 underline underline-offset-4 hover:text-stone-900">
                  忘记密码
                </Link>
              </div>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    void handleLogin();
                  }
                }}
                placeholder="至少 6 位"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <Button
              className="h-13 w-full rounded-2xl bg-stone-950 text-white hover:bg-stone-800"
              onClick={() => void handleLogin()}
              disabled={isSubmitting}
            >
              {isSubmitting ? <LoaderCircle className="size-4 animate-spin" /> : null}
              进入工作区
            </Button>

            <div className="text-center text-sm text-stone-500">
              还没有账号？{" "}
              <Link to="/register" className="font-medium text-stone-950 underline underline-offset-4">
                立即注册
              </Link>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
