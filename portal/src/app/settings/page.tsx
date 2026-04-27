"use client";

import { ChangeEvent, useEffect, useMemo, useState } from "react";
import { BadgeCheck, KeyRound, LoaderCircle, Save, Upload, UserRound, X } from "lucide-react";
import { toast } from "sonner";

import { PortalAvatar } from "@/components/portal-avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { changePortalPassword, updatePortalProfile } from "@/lib/api";
import { usePortalSession } from "@/store/session";

function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(new Error("读取图片失败"));
    reader.readAsDataURL(file);
  });
}

export default function SettingsPage() {
  const { user, quota, applySession } = usePortalSession();
  const [displayName, setDisplayName] = useState("");
  const [avatarUrl, setAvatarUrl] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isSavingProfile, setIsSavingProfile] = useState(false);
  const [isSavingPassword, setIsSavingPassword] = useState(false);

  useEffect(() => {
    setDisplayName(user?.display_name || "");
    setAvatarUrl(user?.avatar_url || "");
  }, [user?.avatar_url, user?.display_name, user?.id]);

  const profileDirty = useMemo(() => {
    return (displayName.trim() || "") !== (user?.display_name?.trim() || "") || avatarUrl !== (user?.avatar_url || "");
  }, [avatarUrl, displayName, user?.avatar_url, user?.display_name]);

  const usage = user?.usage;

  const handleAvatarChange = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) {
      return;
    }
    if (!file.type.startsWith("image/")) {
      toast.error("请选择图片文件");
      return;
    }
    if (file.size > 1024 * 1024) {
      toast.error("头像图片请控制在 1MB 以内");
      return;
    }

    try {
      const result = await readFileAsDataURL(file);
      setAvatarUrl(result);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取头像失败");
    }
  };

  const handleSaveProfile = async () => {
    if (!user) {
      return;
    }

    setIsSavingProfile(true);
    try {
      const payload = await updatePortalProfile({
        display_name: displayName,
        avatar_url: avatarUrl,
      });
      applySession(payload);
      toast.success("个人资料已保存");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存资料失败");
    } finally {
      setIsSavingProfile(false);
    }
  };

  const handleChangePassword = async () => {
    if (!currentPassword.trim()) {
      toast.error("请输入当前密码");
      return;
    }
    if (newPassword.trim().length < 6) {
      toast.error("新密码至少 6 位");
      return;
    }
    if (newPassword !== confirmPassword) {
      toast.error("两次输入的新密码不一致");
      return;
    }

    setIsSavingPassword(true);
    try {
      await changePortalPassword(currentPassword, newPassword);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      toast.success("密码已更新");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "修改密码失败");
    } finally {
      setIsSavingPassword(false);
    }
  };

  return (
    <section className="flex h-full min-h-0 flex-col gap-3">
      <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_340px]">
        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-start gap-4">
            <span className="flex size-12 shrink-0 items-center justify-center rounded-2xl bg-stone-950 text-white">
              <UserRound className="size-5" />
            </span>
            <div className="min-w-0">
              <h1 className="text-xl font-semibold tracking-tight text-stone-950">个人设置</h1>
              <p className="mt-2 text-sm leading-6 text-stone-600">
                在这里维护你的头像、昵称与登录密码。更新后的资料会展示在导航栏、作品广场和评论区。
              </p>
            </div>
          </div>
        </div>

        <div className="grid gap-3 sm:grid-cols-3 xl:grid-cols-1">
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">当前角色</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">
              {user?.role === "admin" ? "管理员" : "普通用户"}
            </div>
          </div>
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">可用总额度</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">{quota?.available_quota ?? "-"}</div>
          </div>
          <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
            <div className="text-sm font-medium text-stone-500">累计发布作品</div>
            <div className="mt-3 text-3xl font-semibold tracking-tight text-stone-950">{usage?.published_works ?? 0}</div>
          </div>
        </div>
      </div>

      <div className="grid min-h-0 gap-3 xl:grid-cols-[minmax(0,1fr)_420px]">
        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3">
            <BadgeCheck className="size-5 text-stone-500" />
            <h2 className="text-lg font-semibold tracking-tight text-stone-950">个人资料</h2>
          </div>

          <div className="mt-6 flex flex-col gap-6 lg:flex-row lg:items-start">
            <div className="mx-auto w-full max-w-[320px] rounded-[28px] border border-stone-200 bg-[#f7f7f4] p-5 lg:mx-0">
              <PortalAvatar
                src={avatarUrl}
                name={displayName || user?.display_name}
                email={user?.email}
                className="size-28"
                textClassName="text-2xl"
              />
              <div className="mt-4 text-center text-sm font-medium text-stone-700">头像预览</div>
              <div className="mt-2 text-center text-xs leading-5 text-stone-500">推荐使用 1:1 图片，支持 JPG / PNG / WEBP。</div>
              <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:justify-center">
                <label className="inline-flex w-full cursor-pointer items-center justify-center gap-2 rounded-full border border-stone-200 bg-white px-4 py-2 text-sm font-medium text-stone-700 transition hover:bg-stone-50 sm:w-auto">
                  <Upload className="size-4" />
                  上传头像
                  <input type="file" accept="image/*" className="hidden" onChange={(event) => void handleAvatarChange(event)} />
                </label>
                <Button type="button" variant="outline" className="w-full rounded-full sm:w-auto" onClick={() => setAvatarUrl("")} disabled={!avatarUrl}>
                  <X className="size-4" />
                  清除
                </Button>
              </div>
            </div>

            <div className="min-w-0 flex-1 space-y-5">
              <div className="space-y-3">
                <label htmlFor="profile-display-name" className="block text-sm font-medium text-stone-700">
                  显示名称
                </label>
                <Input
                  id="profile-display-name"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  placeholder="作品广场和评论区展示的昵称"
                  className="h-12 rounded-2xl border-stone-200 bg-stone-50 px-4"
                />
                <div className="text-xs leading-5 text-stone-500">留空时会默认使用邮箱前缀作为昵称，最多 32 个字符。</div>
              </div>

              <div className="space-y-3">
                <label htmlFor="profile-email" className="block text-sm font-medium text-stone-700">
                  登录邮箱
                </label>
                <Input id="profile-email" value={user?.email || ""} readOnly className="h-12 rounded-2xl border-stone-200 bg-stone-100 px-4 text-stone-500" />
              </div>

              <div className="grid gap-3 sm:grid-cols-3">
                <div className="rounded-[22px] border border-stone-200 bg-stone-50 px-4 py-4">
                  <div className="text-xs font-medium text-stone-500">累计请求</div>
                  <div className="mt-2 text-2xl font-semibold tracking-tight text-stone-950">{usage?.image_requests ?? 0}</div>
                </div>
                <div className="rounded-[22px] border border-stone-200 bg-stone-50 px-4 py-4">
                  <div className="text-xs font-medium text-stone-500">累计生成</div>
                  <div className="mt-2 text-2xl font-semibold tracking-tight text-stone-950">{usage?.generated_images ?? 0}</div>
                </div>
                <div className="rounded-[22px] border border-stone-200 bg-stone-50 px-4 py-4">
                  <div className="text-xs font-medium text-stone-500">累计发布</div>
                  <div className="mt-2 text-2xl font-semibold tracking-tight text-stone-950">{usage?.published_works ?? 0}</div>
                </div>
              </div>

              <div className="flex flex-col gap-3 rounded-[24px] border border-stone-200 bg-[#fafaf8] px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="text-sm leading-6 text-stone-600">保存后会立即更新当前会话，并同步显示到作品广场与评论区。</div>
                <Button
                  type="button"
                  className="w-full rounded-full bg-stone-950 text-white hover:bg-stone-800 sm:w-auto"
                  onClick={() => void handleSaveProfile()}
                  disabled={!profileDirty || isSavingProfile}
                >
                  {isSavingProfile ? <LoaderCircle className="size-4 animate-spin" /> : <Save className="size-4" />}
                  保存资料
                </Button>
              </div>
            </div>
          </div>
        </div>

        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3">
            <KeyRound className="size-5 text-stone-500" />
            <h2 className="text-lg font-semibold tracking-tight text-stone-950">修改密码</h2>
          </div>

          <div className="mt-2 text-sm leading-6 text-stone-500">
            已登录状态下可直接修改密码；如果忘记当前密码，也可以在登录页通过“忘记密码”用邮箱验证码重置。
          </div>

          <div className="mt-6 space-y-4">
            <div className="space-y-3">
              <label htmlFor="current-password" className="block text-sm font-medium text-stone-700">
                当前密码
              </label>
              <Input
                id="current-password"
                type="password"
                value={currentPassword}
                onChange={(event) => setCurrentPassword(event.target.value)}
                placeholder="输入当前登录密码"
                className="h-12 rounded-2xl border-stone-200 bg-stone-50 px-4"
              />
            </div>

            <div className="space-y-3">
              <label htmlFor="new-password" className="block text-sm font-medium text-stone-700">
                新密码
              </label>
              <Input
                id="new-password"
                type="password"
                value={newPassword}
                onChange={(event) => setNewPassword(event.target.value)}
                placeholder="至少 6 位"
                className="h-12 rounded-2xl border-stone-200 bg-stone-50 px-4"
              />
            </div>

            <div className="space-y-3">
              <label htmlFor="confirm-password" className="block text-sm font-medium text-stone-700">
                确认新密码
              </label>
              <Input
                id="confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(event) => setConfirmPassword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    void handleChangePassword();
                  }
                }}
                placeholder="再次输入新密码"
                className="h-12 rounded-2xl border-stone-200 bg-stone-50 px-4"
              />
            </div>
          </div>

          <div className="mt-6 rounded-[24px] border border-stone-200 bg-[#fafaf8] px-5 py-4">
            <div className="text-sm leading-6 text-stone-600">密码更新后会立即生效，后续登录请使用新密码。</div>
            <Button
              type="button"
              className="mt-4 w-full rounded-2xl bg-stone-950 text-white hover:bg-stone-800"
              onClick={() => void handleChangePassword()}
              disabled={isSavingPassword}
            >
              {isSavingPassword ? <LoaderCircle className="size-4 animate-spin" /> : <KeyRound className="size-4" />}
              更新密码
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}
