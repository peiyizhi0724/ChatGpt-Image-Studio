"use client";

import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { LoaderCircle, MailPlus, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { registerPortal, sendPortalRegisterCode } from "@/lib/api";
import { usePortalSession } from "@/store/session";

export default function RegisterPage() {
  const navigate = useNavigate();
  const { applySession } = usePortalSession();
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isSendingCode, setIsSendingCode] = useState(false);
  const [countdown, setCountdown] = useState(0);

  useEffect(() => {
    if (countdown <= 0) {
      return;
    }
    const timer = window.setInterval(() => {
      setCountdown((current) => {
        if (current <= 1) {
          window.clearInterval(timer);
          return 0;
        }
        return current - 1;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [countdown]);

  const handleSendCode = async () => {
    if (!email.trim()) {
      toast.error("请先输入邮箱");
      return;
    }

    setIsSendingCode(true);
    try {
      const payload = await sendPortalRegisterCode(email);
      setCountdown(Math.max(1, payload.resend_in_seconds || 60));
      toast.success("验证码已发送，请检查邮箱");
    } catch (error) {
      const message = error instanceof Error ? error.message : "发送验证码失败";
      toast.error(message);
    } finally {
      setIsSendingCode(false);
    }
  };

  const handleRegister = async () => {
    if (!email.trim()) {
      toast.error("请输入邮箱");
      return;
    }
    if (!code.trim()) {
      toast.error("请输入邮箱验证码");
      return;
    }
    if (password.trim().length < 6) {
      toast.error("密码至少 6 位");
      return;
    }
    if (password !== confirmPassword) {
      toast.error("两次密码不一致");
      return;
    }

    setIsSubmitting(true);
    try {
      const payload = await registerPortal(email, password, code);
      applySession(payload);
      navigate("/workspace", { replace: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : "注册失败";
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
              <div className="mt-1 text-xs text-white/65">共享工作区注册入口</div>
            </div>
          </div>

          <div className="space-y-6">
            <div className="space-y-3">
              <div className="text-sm font-medium uppercase tracking-[0.24em] text-white/55">Register</div>
              <h1 className="max-w-[420px] text-[40px] font-semibold leading-[1.1] tracking-tight">
                创建你的图片工作区账号。
              </h1>
              <p className="max-w-[430px] text-sm leading-7 text-white/72">
                注册成功后会自动登录，你可以直接进入共享图片工作台开始生成、编辑和发布作品。
              </p>
            </div>
          </div>

          <div className="text-xs text-white/50">支持 Docker 部署后的 portal 独立入口。</div>
        </div>

        <div className="flex items-center justify-center px-5 py-8 sm:px-8 lg:px-10">
          <div className="w-full max-w-[420px] space-y-8">
            <div className="space-y-4">
              <div className="inline-flex size-14 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
                <MailPlus className="size-5" />
              </div>
              <div className="space-y-2">
                <h1 className="text-3xl font-semibold tracking-tight text-stone-950">注册 Portal</h1>
                <p className="text-sm leading-7 text-stone-500">创建邮箱账号后即可进入共享图片工作台。</p>
              </div>
            </div>

            <div className="space-y-3">
              <label htmlFor="register-email" className="block text-sm font-medium text-stone-700">
                邮箱
              </label>
              <Input
                id="register-email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="name@example.com"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <div className="space-y-3">
              <label htmlFor="register-code" className="block text-sm font-medium text-stone-700">
                邮箱验证码
              </label>
              <div className="flex gap-3">
                <Input
                  id="register-code"
                  type="text"
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                  placeholder="输入 6 位验证码"
                  className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
                />
                <Button
                  type="button"
                  variant="outline"
                  className="h-13 shrink-0 rounded-2xl px-4"
                  onClick={() => void handleSendCode()}
                  disabled={isSendingCode || countdown > 0}
                >
                  {isSendingCode ? (
                    <LoaderCircle className="size-4 animate-spin" />
                  ) : countdown > 0 ? (
                    `${countdown}s`
                  ) : (
                    "发送验证码"
                  )}
                </Button>
              </div>
            </div>

            <div className="space-y-3">
              <label htmlFor="register-password" className="block text-sm font-medium text-stone-700">
                密码
              </label>
              <Input
                id="register-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="至少 6 位"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <div className="space-y-3">
              <label htmlFor="register-confirm-password" className="block text-sm font-medium text-stone-700">
                确认密码
              </label>
              <Input
                id="register-confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    void handleRegister();
                  }
                }}
                placeholder="再次输入密码"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <Button
              className="h-13 w-full rounded-2xl bg-stone-950 text-white hover:bg-stone-800"
              onClick={() => void handleRegister()}
              disabled={isSubmitting}
            >
              {isSubmitting ? <LoaderCircle className="size-4 animate-spin" /> : null}
              创建并进入工作区
            </Button>

            <div className="text-center text-sm text-stone-500">
              已有账号？{" "}
              <Link to="/login" className="font-medium text-stone-950 underline underline-offset-4">
                返回登录
              </Link>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
