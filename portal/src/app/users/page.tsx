"use client";

import { useEffect, useMemo, useState } from "react";
import { BarChart3, LoaderCircle, ShieldCheck, ShieldX, Sparkles, Upload, UserCog, Users } from "lucide-react";
import { toast } from "sonner";

import { PortalAvatar } from "@/components/portal-avatar";
import { Button } from "@/components/ui/button";
import { fetchPortalUsers, updatePortalUser, type PortalQuotaSummary, type PortalUser } from "@/lib/api";

function formatDate(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function getUserUsage(user: PortalUser) {
  return {
    imageRequests: user.usage?.image_requests ?? 0,
    generatedImages: user.usage?.generated_images ?? 0,
    publishedWorks: user.usage?.published_works ?? 0,
  };
}

export default function UsersPage() {
  const [users, setUsers] = useState<PortalUser[]>([]);
  const [quota, setQuota] = useState<PortalQuotaSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [pendingUserId, setPendingUserId] = useState<string | null>(null);

  const enabledUsers = useMemo(() => users.filter((item) => !item.disabled).length, [users]);
  const adminUsers = useMemo(() => users.filter((item) => item.role === "admin").length, [users]);
  const totalGeneratedImages = useMemo(() => users.reduce((sum, item) => sum + getUserUsage(item).generatedImages, 0), [users]);
  const totalImageRequests = useMemo(() => users.reduce((sum, item) => sum + getUserUsage(item).imageRequests, 0), [users]);
  const totalPublishedWorks = useMemo(() => users.reduce((sum, item) => sum + getUserUsage(item).publishedWorks, 0), [users]);

  const loadUsers = async () => {
    setIsLoading(true);
    try {
      const payload = await fetchPortalUsers();
      setUsers(payload.items);
      setQuota(payload.quota);
    } catch (error) {
      const message = error instanceof Error ? error.message : "加载用户失败";
      toast.error(message);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    void loadUsers();
  }, []);

  const handleUserUpdate = async (userId: string, updates: { role?: "admin" | "user"; disabled?: boolean }) => {
    setPendingUserId(userId);
    try {
      const payload = await updatePortalUser(userId, updates);
      setUsers((current) => current.map((item) => (item.id === userId ? payload.item : item)));
      setQuota(payload.quota);
      toast.success("用户状态已更新");
    } catch (error) {
      const message = error instanceof Error ? error.message : "更新失败";
      toast.error(message);
    } finally {
      setPendingUserId(null);
    }
  };

  return (
    <section className="flex h-full min-h-0 flex-col gap-3">
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-6">
        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3 text-stone-500">
            <Users className="size-5" />
            <span className="text-sm font-medium">总用户数</span>
          </div>
          <div className="mt-4 text-3xl font-semibold tracking-tight text-stone-950">{users.length}</div>
          <div className="mt-2 text-sm text-stone-500">当前可用 {enabledUsers} 人</div>
        </div>

        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3 text-stone-500">
            <UserCog className="size-5" />
            <span className="text-sm font-medium">管理员</span>
          </div>
          <div className="mt-4 text-3xl font-semibold tracking-tight text-stone-950">{adminUsers}</div>
          <div className="mt-2 text-sm text-stone-500">可统一管理用户状态与共享额度</div>
        </div>

        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3 text-stone-500">
            <Sparkles className="size-5" />
            <span className="text-sm font-medium">累计生成</span>
          </div>
          <div className="mt-4 text-3xl font-semibold tracking-tight text-stone-950">{totalGeneratedImages}</div>
          <div className="mt-2 text-sm text-stone-500">按实际返回图片张数累计</div>
        </div>

        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3 text-stone-500">
            <BarChart3 className="size-5" />
            <span className="text-sm font-medium">图片请求</span>
          </div>
          <div className="mt-4 text-3xl font-semibold tracking-tight text-stone-950">{totalImageRequests}</div>
          <div className="mt-2 text-sm text-stone-500">生成、编辑等任务请求次数</div>
        </div>

        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3 text-stone-500">
            <Upload className="size-5" />
            <span className="text-sm font-medium">已发布作品</span>
          </div>
          <div className="mt-4 text-3xl font-semibold tracking-tight text-stone-950">{totalPublishedWorks}</div>
          <div className="mt-2 text-sm text-stone-500">已经进入作品广场的图片数量</div>
        </div>

        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3 text-stone-500">
            <ShieldCheck className="size-5" />
            <span className="text-sm font-medium">可用总额度</span>
          </div>
          <div className="mt-4 text-3xl font-semibold tracking-tight text-stone-950">{quota?.available_quota ?? "-"}</div>
          <div className="mt-2 text-sm text-stone-500">当前可参与出图的共享额度</div>
        </div>

        <div className="rounded-[30px] border border-stone-200 bg-white p-6 shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
          <div className="flex items-center gap-3 text-stone-500">
            <ShieldX className="size-5" />
            <span className="text-sm font-medium">总额度快照</span>
          </div>
          <div className="mt-4 text-3xl font-semibold tracking-tight text-stone-950">{quota?.total_quota ?? "-"}</div>
          <div className="mt-2 text-sm text-stone-500">所有账号累计额度，不做单用户限制</div>
        </div>
      </div>

      <div className="min-h-0 overflow-hidden rounded-[30px] border border-stone-200 bg-white shadow-[0_14px_40px_rgba(15,23,42,0.05)]">
        <div className="flex flex-col gap-3 border-b border-stone-100 px-6 py-5 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-stone-950">用户管理</h1>
            <p className="mt-1 text-sm text-stone-500">管理员可调整角色与启用状态，当前不限制单用户使用次数。</p>
          </div>
          <Button variant="outline" className="w-full rounded-2xl sm:w-auto" onClick={() => void loadUsers()} disabled={isLoading}>
            刷新列表
          </Button>
        </div>

        <div className="hide-scrollbar h-full overflow-auto">
          {isLoading ? (
            <div className="grid min-h-[320px] place-items-center text-stone-500">
              <div className="flex items-center gap-3">
                <LoaderCircle className="size-4 animate-spin" />
                正在加载用户列表...
              </div>
            </div>
          ) : (
            <>
              <div className="grid gap-3 p-4 lg:hidden">
                {users.map((user) => {
                  const pending = pendingUserId === user.id;
                  const usage = getUserUsage(user);
                  return (
                    <article key={user.id} className="rounded-[24px] border border-stone-200 bg-[#fafaf8] p-4 shadow-sm">
                      <div className="flex items-start gap-3">
                        <PortalAvatar
                          src={user.avatar_url}
                          name={user.display_name}
                          email={user.email}
                          className="size-12"
                        />
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-sm font-semibold text-stone-950">{user.display_name || user.email}</div>
                          <div className="mt-1 truncate text-xs text-stone-500">{user.email}</div>
                          <div className="mt-1 truncate text-[11px] text-stone-400">{user.id}</div>
                        </div>
                      </div>

                      <div className="mt-4 flex flex-wrap gap-2">
                        <span className="inline-flex rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-stone-700">
                          {user.role === "admin" ? "管理员" : "普通用户"}
                        </span>
                        <span
                          className={`inline-flex rounded-full px-3 py-1 text-xs font-medium ${
                            user.disabled ? "bg-rose-50 text-rose-700" : "bg-emerald-50 text-emerald-700"
                          }`}
                        >
                          {user.disabled ? "已停用" : "正常"}
                        </span>
                      </div>

                      <div className="mt-4 grid gap-3 sm:grid-cols-2">
                        <div className="rounded-[18px] border border-stone-200 bg-white px-3 py-3 text-sm text-stone-600">
                          <div className="font-medium text-stone-950">生成 {usage.generatedImages}</div>
                          <div className="mt-1 text-xs text-stone-500">请求 {usage.imageRequests} · 发布 {usage.publishedWorks}</div>
                        </div>
                        <div className="rounded-[18px] border border-stone-200 bg-white px-3 py-3 text-xs text-stone-500">
                          <div>注册时间：{formatDate(user.created_at)}</div>
                          <div className="mt-1">最近登录：{formatDate(user.last_login_at)}</div>
                        </div>
                      </div>

                      <div className="mt-4 grid gap-2">
                        <Button
                          variant="outline"
                          className="w-full rounded-2xl"
                          disabled={pending || user.role === "admin"}
                          onClick={() => void handleUserUpdate(user.id, { role: "admin" })}
                        >
                          设为管理员
                        </Button>
                        <Button
                          variant="outline"
                          className="w-full rounded-2xl"
                          disabled={pending || user.role === "user"}
                          onClick={() => void handleUserUpdate(user.id, { role: "user" })}
                        >
                          设为普通用户
                        </Button>
                        <Button
                          variant="outline"
                          className="w-full rounded-2xl"
                          disabled={pending}
                          onClick={() => void handleUserUpdate(user.id, { disabled: !user.disabled })}
                        >
                          {user.disabled ? "启用" : "停用"}
                        </Button>
                      </div>
                    </article>
                  );
                })}
              </div>

              <div className="hidden h-full overflow-auto lg:block">
                <table className="min-w-full text-left text-sm">
                  <thead className="bg-stone-50 text-stone-500">
                    <tr>
                      <th className="px-6 py-4 font-medium">用户</th>
                      <th className="px-6 py-4 font-medium">角色</th>
                      <th className="px-6 py-4 font-medium">状态</th>
                      <th className="px-6 py-4 font-medium">使用量</th>
                      <th className="px-6 py-4 font-medium">注册时间</th>
                      <th className="px-6 py-4 font-medium">最近登录</th>
                      <th className="px-6 py-4 font-medium">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((user) => {
                      const pending = pendingUserId === user.id;
                      const usage = getUserUsage(user);
                      return (
                        <tr key={user.id} className="border-t border-stone-100 align-top">
                          <td className="px-6 py-4">
                            <div className="flex min-w-0 items-center gap-3">
                              <PortalAvatar
                                src={user.avatar_url}
                                name={user.display_name}
                                email={user.email}
                                className="size-11"
                              />
                              <div className="min-w-0">
                                <div className="truncate font-medium text-stone-950">{user.display_name || user.email}</div>
                                <div className="mt-1 truncate text-xs text-stone-500">{user.email}</div>
                                <div className="mt-1 truncate text-xs text-stone-400">{user.id}</div>
                              </div>
                            </div>
                          </td>
                          <td className="px-6 py-4">
                            <span className="inline-flex rounded-full bg-stone-100 px-3 py-1 text-xs font-medium text-stone-700">
                              {user.role === "admin" ? "管理员" : "普通用户"}
                            </span>
                          </td>
                          <td className="px-6 py-4">
                            <span
                              className={`inline-flex rounded-full px-3 py-1 text-xs font-medium ${
                                user.disabled ? "bg-rose-50 text-rose-700" : "bg-emerald-50 text-emerald-700"
                              }`}
                            >
                              {user.disabled ? "已停用" : "正常"}
                            </span>
                          </td>
                          <td className="px-6 py-4">
                            <div className="font-medium text-stone-950">生成 {usage.generatedImages}</div>
                            <div className="mt-1 text-xs text-stone-500">请求 {usage.imageRequests} · 发布 {usage.publishedWorks}</div>
                          </td>
                          <td className="px-6 py-4 text-stone-600">{formatDate(user.created_at)}</td>
                          <td className="px-6 py-4 text-stone-600">{formatDate(user.last_login_at)}</td>
                          <td className="px-6 py-4">
                            <div className="flex flex-wrap gap-2">
                              <Button
                                variant="outline"
                                className="rounded-2xl"
                                disabled={pending || user.role === "admin"}
                                onClick={() => void handleUserUpdate(user.id, { role: "admin" })}
                              >
                                设为管理员
                              </Button>
                              <Button
                                variant="outline"
                                className="rounded-2xl"
                                disabled={pending || user.role === "user"}
                                onClick={() => void handleUserUpdate(user.id, { role: "user" })}
                              >
                                设为普通用户
                              </Button>
                              <Button
                                variant="outline"
                                className="rounded-2xl"
                                disabled={pending}
                                onClick={() => void handleUserUpdate(user.id, { disabled: !user.disabled })}
                              >
                                {user.disabled ? "启用" : "停用"}
                              </Button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      </div>
    </section>
  );
}
