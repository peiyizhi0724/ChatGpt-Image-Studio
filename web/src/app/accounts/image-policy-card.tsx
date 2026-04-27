"use client";

import { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronUp, LoaderCircle } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  fetchImageAccountPolicy,
  setCachedImageAccountPolicy,
  updateImageAccountPolicy,
  type Account,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  buildImageAccountGroupPreviews,
  getEffectiveImageAccountPolicy,
  getImageAccountPolicyCollapsed,
  normalizeImageAccountPolicy,
  setImageAccountPolicyCollapsed,
  type ImageAccountSortMode,
  type StoredImageAccountPolicy,
} from "@/store/image-account-policy";

type ImagePolicyCardProps = {
  accounts: Account[];
};

function formatImportedAt(value?: string | null) {
  if (!value) {
    return "未记录";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function ImagePolicyCard({ accounts }: ImagePolicyCardProps) {
  const [imagePolicy, setImagePolicy] = useState<StoredImageAccountPolicy>(
    () => normalizeImageAccountPolicy(null),
  );
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(() =>
    getImageAccountPolicyCollapsed(),
  );

  useEffect(() => {
    const loadPolicy = async () => {
      setIsLoading(true);
      try {
        const policy = await fetchImageAccountPolicy();
        setImagePolicy(policy);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : "读取图片账号分组策略失败",
        );
      } finally {
        setIsLoading(false);
      }
    };
    void loadPolicy();
  }, []);

  useEffect(() => {
    setImageAccountPolicyCollapsed(isCollapsed);
  }, [isCollapsed]);

  const groups = useMemo(
    () => buildImageAccountGroupPreviews(accounts, imagePolicy),
    [accounts, imagePolicy],
  );

  const effectivePolicy = useMemo(
    () =>
      getEffectiveImageAccountPolicy(imagePolicy, { groupCount: groups.length }),
    [groups.length, imagePolicy],
  );

  const configuredGroupSummary = useMemo(
    () =>
      imagePolicy.enabledGroupIndexes.length > 0
        ? imagePolicy.enabledGroupIndexes.map((index) => index + 1).join(" / ")
        : "未选择",
    [imagePolicy.enabledGroupIndexes],
  );

  const effectiveGroupSummary = useMemo(
    () =>
      effectivePolicy.enabledGroupIndexes.length > 0
        ? effectivePolicy.enabledGroupIndexes.map((index) => index + 1).join(" / ")
        : "当前无有效分组",
    [effectivePolicy.enabledGroupIndexes],
  );

  const selectionAdjustedByPool = useMemo(() => {
    const raw = imagePolicy.enabledGroupIndexes.join(",");
    const effective = effectivePolicy.enabledGroupIndexes.join(",");
    return raw !== effective;
  }, [effectivePolicy.enabledGroupIndexes, imagePolicy.enabledGroupIndexes]);

  const persistPolicy = async (nextPolicy: StoredImageAccountPolicy) => {
    const normalized = normalizeImageAccountPolicy(nextPolicy);
    setImagePolicy(normalized);
    setCachedImageAccountPolicy(normalized);
    setIsSaving(true);
    try {
      const saved = await updateImageAccountPolicy(normalized);
      setImagePolicy(saved);
      setCachedImageAccountPolicy(saved);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "保存图片账号分组策略失败",
      );
      try {
        const restored = await fetchImageAccountPolicy();
        setImagePolicy(restored);
        setCachedImageAccountPolicy(restored);
      } catch {
        // ignore restore failure; user already got the save error
      }
    } finally {
      setIsSaving(false);
    }
  };

  const updatePolicy = (patch: Partial<StoredImageAccountPolicy>) => {
    void persistPolicy({
      ...imagePolicy,
      ...patch,
    });
  };

  const toggleGroup = (groupIndex: number, enabled: boolean) => {
    const nextGroupIndexes = enabled
      ? Array.from(new Set([...imagePolicy.enabledGroupIndexes, groupIndex]))
      : imagePolicy.enabledGroupIndexes.filter((value) => value !== groupIndex);
    void persistPolicy({
      ...imagePolicy,
      enabledGroupIndexes: nextGroupIndexes,
    });
  };

  const shouldShowGroups = imagePolicy.enabled && !isCollapsed;

  return (
    <Card className="rounded-2xl border-white/80 bg-white/90 shadow-sm">
      <CardContent className="space-y-4 p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="text-base font-semibold tracking-tight text-stone-950">
                图片账号分组策略
              </h2>
              {isSaving ? (
                <LoaderCircle className="size-4 animate-spin text-stone-400" />
              ) : null}
            </div>
            <p className="mt-1 text-sm leading-6 text-stone-500">
              策略现在跟随账号池后端存储。启用后，图片请求会优先在勾选分组内轮询，并为每个账号保留安全阈值。
            </p>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <Badge variant={imagePolicy.enabled ? "success" : "secondary"}>
              {imagePolicy.enabled ? "已启用" : "未启用"}
            </Badge>
            {imagePolicy.enabled ? (
              <Badge variant="info">实际发送分组：{effectiveGroupSummary}</Badge>
            ) : null}
            <Button
              type="button"
              variant="outline"
              className="h-9 rounded-xl border-stone-200 bg-white px-3 text-stone-700"
              onClick={() => setIsCollapsed((current) => !current)}
              disabled={!imagePolicy.enabled}
            >
              {isCollapsed ? <ChevronDown className="size-4" /> : <ChevronUp className="size-4" />}
              {isCollapsed ? "展开分组" : "收起分组"}
            </Button>
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <div className="rounded-2xl border border-stone-100 bg-stone-50 p-4">
            <div className="mb-2 text-sm font-medium text-stone-800">是否启用</div>
            <div className="flex items-center gap-3">
              <Checkbox
                checked={imagePolicy.enabled}
                onCheckedChange={(checked) =>
                  updatePolicy({ enabled: Boolean(checked) })
                }
              />
              <span className="text-sm text-stone-600">启用分组轮询</span>
            </div>
          </div>
          <div className="rounded-2xl border border-stone-100 bg-stone-50 p-4">
            <div className="mb-2 text-sm font-medium text-stone-800">排序方式</div>
            <Select
              value={imagePolicy.sortMode}
              onValueChange={(value) =>
                updatePolicy({ sortMode: value as ImageAccountSortMode })
              }
              disabled={!imagePolicy.enabled}
            >
              <SelectTrigger className="h-10 rounded-xl border-stone-200 bg-white">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="imported_at">按导入时间</SelectItem>
                <SelectItem value="name">按名称</SelectItem>
                <SelectItem value="quota">按剩余额度</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="rounded-2xl border border-stone-100 bg-stone-50 p-4">
            <div className="mb-2 text-sm font-medium text-stone-800">每组账号数</div>
            <Input
              min={1}
              max={100}
              type="number"
              value={imagePolicy.groupSize}
              onChange={(event) =>
                updatePolicy({ groupSize: Number(event.target.value) || 1 })
              }
              disabled={!imagePolicy.enabled}
              className="h-10 rounded-xl border-stone-200 bg-white"
            />
          </div>
          <div className="rounded-2xl border border-stone-100 bg-stone-50 p-4">
            <div className="mb-2 text-sm font-medium text-stone-800">保底百分比</div>
            <Input
              min={0}
              max={100}
              type="number"
              value={imagePolicy.reservePercent}
              onChange={(event) =>
                updatePolicy({ reservePercent: Number(event.target.value) || 0 })
              }
              disabled={!imagePolicy.enabled}
              className="h-10 rounded-xl border-stone-200 bg-white"
            />
          </div>
        </div>

        {imagePolicy.enabled ? (
          <>
            <div className="grid gap-3 md:grid-cols-2">
              <div className="rounded-2xl border border-stone-100 bg-stone-50 p-4">
                <div className="text-xs font-medium text-stone-400">已保存分组</div>
                <div className="mt-2 text-base font-semibold tracking-tight text-stone-900">
                  {configuredGroupSummary}
                </div>
                <p className="mt-1 text-xs text-stone-500">
                  这是账号池里持久化保存的原始勾选结果。
                </p>
              </div>
              <div className="rounded-2xl border border-emerald-100 bg-emerald-50/60 p-4">
                <div className="text-xs font-medium text-emerald-700">当前实际发送分组</div>
                <div className="mt-2 text-base font-semibold tracking-tight text-emerald-900">
                  {effectiveGroupSummary}
                </div>
                <p className="mt-1 text-xs text-emerald-700">
                  账号池增减后，这里会按当前可见分组临时修正，不会直接覆盖你保存的原始选择。
                </p>
              </div>
            </div>

            {selectionAdjustedByPool ? (
              <div className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-900">
                当前账号池数量变化后，部分已保存分组暂时超出范围。请求会按当前仍有效的分组发送；如果全部超出范围，会临时回退到当前存在的分组，避免直接失效。
              </div>
            ) : null}

            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="outline"
                className="h-9 rounded-xl border-stone-200 bg-white text-stone-700"
                onClick={() =>
                  updatePolicy({
                    enabledGroupIndexes: groups
                      .slice(0, Math.min(2, groups.length))
                      .map((group) => group.index),
                  })
                }
                disabled={groups.length === 0}
              >
                勾选前 2 组
              </Button>
              <Button
                variant="outline"
                className="h-9 rounded-xl border-stone-200 bg-white text-stone-700"
                onClick={() =>
                  updatePolicy({
                    enabledGroupIndexes: groups.map((group) => group.index),
                  })
                }
                disabled={groups.length === 0}
              >
                勾选全部分组
              </Button>
              <Button
                variant="outline"
                className="h-9 rounded-xl border-stone-200 bg-white text-stone-700"
                onClick={() => updatePolicy({ enabledGroupIndexes: [] })}
              >
                清空分组
              </Button>
            </div>
          </>
        ) : null}

        {isLoading ? (
          <div className="rounded-2xl border border-stone-200 bg-stone-50 px-4 py-4 text-sm text-stone-500">
            正在读取图片账号分组策略...
          </div>
        ) : null}

        {shouldShowGroups ? (
          <div className="space-y-3">
            <div className="max-h-[320px] overflow-y-auto pr-1">
              <div className="space-y-2">
                {groups.length === 0 ? (
                  <div className="rounded-2xl border border-dashed border-stone-200 bg-stone-50 px-4 py-6 text-sm text-stone-500">
                    先导入账号，这里才会生成分组预览。
                  </div>
                ) : (
                  groups.map((group) => (
                    <div
                      key={group.index}
                      className={cn(
                        "rounded-2xl border px-3 py-3 transition-colors",
                        group.enabled
                          ? "border-emerald-200 bg-emerald-50/70"
                          : "border-stone-200 bg-stone-50",
                      )}
                    >
                      <div className="flex items-start gap-3">
                        <Checkbox
                          checked={group.enabled}
                          onCheckedChange={(checked) =>
                            toggleGroup(group.index, Boolean(checked))
                          }
                        />
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-sm font-semibold text-stone-900">
                              {group.label}
                            </span>
                            <Badge variant={group.enabled ? "success" : "secondary"}>
                              {group.enabled ? "参与轮询" : "不参与"}
                            </Badge>
                          </div>
                          <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-stone-500">
                            <span>账号 {group.accounts.length}</span>
                            <span>可用 {group.availableCount}</span>
                            <span>总剩余 {group.totalRemaining}</span>
                            <span>平均 {group.averageRemaining}</span>
                            <span>
                              首个 {formatImportedAt(group.accounts[0]?.importedAt)}
                            </span>
                            <span>
                              末个{" "}
                              {formatImportedAt(
                                group.accounts[group.accounts.length - 1]?.importedAt,
                              )}
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
