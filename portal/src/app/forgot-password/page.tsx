"use client";

import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { KeyRound, LoaderCircle, Sparkles } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { resetPortalPassword, sendPortalPasswordResetCode } from "@/lib/api";

export default function ForgotPasswordPage() {
  const navigate = useNavigate();
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
      const payload = await sendPortalPasswordResetCode(email);
      setCountdown(Math.max(1, payload.resend_in_seconds || 60));
      toast.success("验证码已发送，请检查邮箱");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "发送验证码失败");
    } finally {
      setIsSendingCode(false);
    }
  };

  const handleResetPassword = async () => {
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
      await resetPortalPassword(email, password, code);
      toast.success("密码已重置，请重新登录");
      navigate("/login", { replace: true });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "重置密码失败");
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
              <div className="mt-1 text-xs text-white/65">邮箱找回密码入口</div>
            </div>
          </div>

          <div className="space-y-6">
            <div className="space-y-3">
              <div className="text-sm font-medium uppercase tracking-[0.24em] text-white/55">Password Reset</div>
              <h1 className="max-w-[420px] text-[40px] font-semibold leading-[1.1] tracking-tight">
                通过邮箱验证码，安全重设账号密码。
              </h1>
              <p className="max-w-[430px] text-sm leading-7 text-white/72">
                系统会向你的注册邮箱发送验证码，验证通过后即可重新设置登录密码。
              </p>
            </div>

            <div className="grid gap-3 sm:grid-cols-3">
              {[
                ["验证", "输入注册邮箱获取验证码"],
                ["重设", "设置新的登录密码"],
                ["返回", "完成后重新登录工作区"],
              ].map(([title, desc]) => (
                <div key={title} className="rounded-2xl border border-white/12 bg-white/6 p-4 backdrop-blur-sm">
                  <div className="text-sm font-semibold">{title}</div>
                  <div className="mt-2 text-xs leading-6 text-white/65">{desc}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="text-xs text-white/50">支持在登录失效或忘记密码时自助恢复访问。</div>
        </div>

        <div className="flex items-center justify-center px-5 py-8 sm:px-8 lg:px-10">
          <div className="w-full max-w-[420px] space-y-8">
            <div className="space-y-4">
              <div className="inline-flex size-14 items-center justify-center rounded-[18px] bg-stone-950 text-white shadow-sm">
                <KeyRound className="size-5" />
              </div>
              <div className="space-y-2">
                <h1 className="text-3xl font-semibold tracking-tight text-stone-950">忘记密码</h1>
                <p className="text-sm leading-7 text-stone-500">使用邮箱验证码重置密码，成功后回到登录页重新进入工作区。</p>
              </div>
            </div>

            <div className="space-y-3">
              <label htmlFor="forgot-email" className="block text-sm font-medium text-stone-700">
                注册邮箱
              </label>
              <Input
                id="forgot-email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="name@example.com"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <div className="space-y-3">
              <label htmlFor="forgot-code" className="block text-sm font-medium text-stone-700">
                邮箱验证码
              </label>
              <div className="flex gap-3">
                <Input
                  id="forgot-code"
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
              <label htmlFor="forgot-password" className="block text-sm font-medium text-stone-700">
                新密码
              </label>
              <Input
                id="forgot-password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="至少 6 位"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <div className="space-y-3">
              <label htmlFor="forgot-confirm-password" className="block text-sm font-medium text-stone-700">
                确认新密码
              </label>
              <Input
                id="forgot-confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    void handleResetPassword();
                  }
                }}
                placeholder="再次输入新密码"
                className="h-13 rounded-2xl border-stone-200 bg-stone-50 px-4 shadow-none focus-visible:ring-1"
              />
            </div>

            <Button
              className="h-13 w-full rounded-2xl bg-stone-950 text-white hover:bg-stone-800"
              onClick={() => void handleResetPassword()}
              disabled={isSubmitting}
            >
              {isSubmitting ? <LoaderCircle className="size-4 animate-spin" /> : null}
              重置密码
            </Button>

            <div className="text-center text-sm text-stone-500">
              想起密码了？{" "}
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
