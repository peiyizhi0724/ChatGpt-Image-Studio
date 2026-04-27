"use client";

import { useEffect, useState } from "react";
import { Link2, LoaderCircle } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { testIntegration, type ConfigPayload, type ImageMode, type IntegrationTestResult } from "@/lib/api";

import { ConfigSection, Field, ToggleField, TooltipDetails, type SetConfigSection } from "./shared";

const imageModeOptions: Array<{ label: string; value: ImageMode; hint: string }> = [
  { label: "Studio", value: "studio", hint: "Free 走当前项目官方链路，Plus/Pro/Team 走官方 responses" },
  { label: "CPA", value: "cpa", hint: "所有图片请求都直接走 CPA；本地号池不参与 CPA 实际选路，Free 号大概率无权限" },
];

const imageRouteOptions = [
  { label: "legacy", value: "legacy" },
  { label: "responses", value: "responses" },
];

const responsesCompatibleImageModelOptions = [
  { label: "gpt-5.4-mini", value: "gpt-5.4-mini" },
  { label: "gpt-5.4", value: "gpt-5.4" },
  { label: "gpt-5.5", value: "gpt-5.5" },
  { label: "gpt-5-5-thinking", value: "gpt-5-5-thinking" },
];

const freeLegacyImageModelOptions = [
  { label: "auto（默认）", value: "auto" },
  { label: "gpt-image-2", value: "gpt-image-2" },
  ...responsesCompatibleImageModelOptions,
];

const freeResponsesImageModelOptions = [
  { label: "auto（默认）", value: "auto" },
  ...responsesCompatibleImageModelOptions,
];

const paidLegacyImageModelOptions = [
  { label: "gpt-5.4-mini（默认）", value: "gpt-5.4-mini" },
  { label: "gpt-5.4", value: "gpt-5.4" },
  { label: "gpt-5.5", value: "gpt-5.5" },
  { label: "gpt-5-5-thinking", value: "gpt-5-5-thinking" },
  { label: "gpt-image-2", value: "gpt-image-2" },
];

const paidResponsesImageModelOptions = paidLegacyImageModelOptions.filter((item) => item.value !== "gpt-image-2");

const cpaRouteStrategyOptions: Array<{
  label: string;
  value: ConfigPayload["cpa"]["routeStrategy"];
  hint: string;
}> = [
  { label: "images_api", value: "images_api", hint: "走 CPA 的 /v1/images/* 包装接口，请求模型就是 gpt-image-2。" },
  { label: "codex_responses", value: "codex_responses", hint: "走 CPA 的 codex responses 子链路，主模型为 gpt-5.4-mini，图片工具模型仍为 gpt-image-2。" },
  { label: "auto", value: "auto", hint: "先走 /v1/images/*，命中特定已知错误时再回退到 codex responses。" },
];

function getFreeImageModelOptions(route: string) {
  return route === "responses" ? freeResponsesImageModelOptions : freeLegacyImageModelOptions;
}

function getPaidImageModelOptions(route: string) {
  return route === "responses" ? paidResponsesImageModelOptions : paidLegacyImageModelOptions;
}

function normalizeImageRouteModel(
  value: string,
  options: Array<{ label: string; value: string }>,
  fallback: string,
) {
  const normalizedValue = String(value || "").trim();
  if (options.some((item) => item.value === normalizedValue)) {
    return normalizedValue;
  }
  return fallback;
}

type ImageModeSectionProps = {
  config: ConfigPayload;
  isStudioMode: boolean;
  isCPAMode: boolean;
  effectiveCPAImageBaseUrl: string;
  syncManagementKeyStatus: string;
  setSection: SetConfigSection;
};

export function ImageModeSection({
  config,
  isStudioMode,
  isCPAMode,
  effectiveCPAImageBaseUrl,
  syncManagementKeyStatus,
  setSection,
}: ImageModeSectionProps) {
  const [isTestingCPA, setIsTestingCPA] = useState(false);
  const freeModelSelectOptions = getFreeImageModelOptions(config.chatgpt.freeImageRoute);
  const paidModelSelectOptions = getPaidImageModelOptions(config.chatgpt.paidImageRoute);
  const normalizedFreeImageModel = normalizeImageRouteModel(
    config.chatgpt.freeImageModel,
    freeModelSelectOptions,
    "auto",
  );
  const normalizedPaidImageModel = normalizeImageRouteModel(
    config.chatgpt.paidImageModel,
    paidModelSelectOptions,
    "gpt-5.4-mini",
  );

  useEffect(() => {
    if (
      normalizedFreeImageModel === config.chatgpt.freeImageModel &&
      normalizedPaidImageModel === config.chatgpt.paidImageModel
    ) {
      return;
    }
    setSection("chatgpt", {
      ...config.chatgpt,
      freeImageModel: normalizedFreeImageModel,
      paidImageModel: normalizedPaidImageModel,
    });
  }, [
    config.chatgpt,
    config.chatgpt.freeImageModel,
    config.chatgpt.paidImageModel,
    normalizedFreeImageModel,
    normalizedPaidImageModel,
    setSection,
  ]);

  const buildIntegrationToastMessage = (result: IntegrationTestResult) => {
    const segments = [result.message];
    if (result.status > 0) {
      segments.push(`HTTP ${result.status}`);
    }
    if (result.latency >= 0) {
      segments.push(`${result.latency} ms`);
    }
    return segments.filter(Boolean).join(" / ");
  };

  const handleTestCPA = async () => {
    setIsTestingCPA(true);
    try {
      const result = await testIntegration("cpa", {
        cpa: {
          ...config.cpa,
          baseUrl: effectiveCPAImageBaseUrl,
        },
      });
      if (result.ok) {
        toast.success(buildIntegrationToastMessage(result));
      } else {
        toast.error(buildIntegrationToastMessage(result));
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "CPA 测试失败");
    } finally {
      setIsTestingCPA(false);
    }
  };

  return (
    <>
      <ConfigSection title="图片模式" description="控制图片请求到底走当前项目官方链路，还是走 CPA 的 OpenAI 图片接口。">
        <Field
          label="图片模式"
          hint={imageModeOptions.find((item) => item.value === config.chatgpt.imageMode)?.hint || ""}
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "Studio",
                  body: (
                    <>
                      Free 账号走当前项目官方链路，Plus / Pro / Team 账号走官方 <code>responses</code> 链路。
                    </>
                  ),
                },
                {
                  title: "CPA",
                  body: <>所有图片请求都直接走 CPA 图片接口；本地号池不参与 CPA 实际选路。Free 账号大概率没有图片权限，但是否成功由 CPA 上游自己决定。</>,
                },
              ]}
            />
          }
        >
          <Select
            value={config.chatgpt.imageMode}
            onValueChange={(value) =>
              setSection("chatgpt", {
                ...config.chatgpt,
                imageMode: value as ImageMode,
              })
            }
          >
            <SelectTrigger className="h-11 rounded-2xl border-stone-200 bg-white shadow-none focus-visible:ring-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {imageModeOptions.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {!isCPAMode ? (
          <Field
            label="默认图片模型"
            hint="当前项目官方图片请求默认使用的模型名。"
            tooltip={
              <TooltipDetails
                items={[
                  {
                    title: "作用",
                    body: <>作为当前项目官方图片生成、编辑请求的默认模型名；未单独覆盖时会优先用这里。</>,
                  },
                  {
                    title: "常见值",
                    body: (
                      <>
                        <code>gpt-image-2</code>、<code>gpt-image-1</code>。
                      </>
                    ),
                  },
                  {
                    title: "建议",
                    body: <>大多数场景保持默认 `gpt-image-2` 即可；只有确认上游支持时再改其他模型。</>,
                  },
                ]}
              />
            }
          >
            <Input
              value={config.chatgpt.model}
              onChange={(event) => setSection("chatgpt", { ...config.chatgpt, model: event.target.value })}
              className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
            />
          </Field>
        ) : (
          <Field
            label="CPA 图片工具模型"
            hint="CPA 模式下图片工具模型固定为 gpt-image-2。若子路由是 codex_responses，顶层主模型当前固定为 gpt-5.4-mini。"
            tooltip={
              <TooltipDetails
                items={[
                  {
                    title: "当前实现",
                    body: <>当前项目对 CPA 的图片工具模型固定为 `gpt-image-2`，不会再读取下面的 Free / Paid 模型配置。</>,
                  },
                  {
                    title: "子路由差异",
                    body: <>如果选的是 `images_api`，请求模型就是 `gpt-image-2`；如果选的是 `codex_responses`，顶层主模型当前会是 `gpt-5.4-mini`，但图片工具模型仍是 `gpt-image-2`。</>,
                  },
                  {
                    title: "作用",
                    body: <>这里做成只读，是为了避免页面上出现“能改但实际不会生效”的模型项。</>,
                  },
                ]}
              />
            }
            fullWidth
          >
            <Input
              value="gpt-image-2"
              readOnly
              className="h-11 rounded-2xl border-stone-200 bg-stone-50 text-stone-500 shadow-none"
            />
          </Field>
        )}
        {isStudioMode ? (
          <Field
            label="Free 账号路由"
            hint="Studio 模式下 Free 账号走 legacy（网页 API 接口）还是 responses。"
            tooltip={
              <TooltipDetails
                items={[
                  {
                    title: "legacy",
                    body: <>走当前项目网页 API 接口链路，本质是网页侧会话接口，兼容性更高，Free 账号通常建议保留这个值。</>,
                  },
                  {
                    title: "responses",
                    body: <>走官方新的 `responses` 链路，但 Free 账号经常没有对应工具权限，可能直接失败。</>,
                  },
                  {
                    title: "建议",
                    body: <>除非你确认 Free 账号在上游具备图片工具权限，否则保持 `legacy` 更稳。</>,
                  },
                ]}
              />
            }
          >
            <Select
              value={config.chatgpt.freeImageRoute}
              onValueChange={(value) =>
                setSection("chatgpt", {
                  ...config.chatgpt,
                  freeImageRoute: value,
                  freeImageModel: normalizeImageRouteModel(
                    config.chatgpt.freeImageModel,
                    getFreeImageModelOptions(value),
                    "auto",
                  ),
                })
              }
            >
              <SelectTrigger className="h-11 rounded-2xl border-stone-200 bg-white shadow-none focus-visible:ring-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {imageRouteOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
        {isStudioMode ? (
          <Field
            label="Free 模型"
            hint={
              config.chatgpt.freeImageRoute === "responses"
                ? "Free 走 responses 时只保留 auto 与 gpt-5.* 兼容模型；gpt-image-2 已移除，避免误选。"
                : "Studio 模式下 Free 账号真正发给官方的模型。legacy 路由仍可选 gpt-image-2。"
            }
            tooltip={
              <TooltipDetails
                items={[
                  {
                    title: "路由绑定",
                    body: (
                      <>
                        <code>legacy</code> 可选 <code>auto</code>、<code>gpt-image-2</code> 和 <code>gpt-5.*</code>；
                        <code>responses</code> 只保留 <code>auto</code> 与 <code>gpt-5.*</code> 兼容模型。
                      </>
                    ),
                  },
                  {
                    title: "auto",
                    body: <>保留旧行为，让上游自己选兼容模型，通常最稳。</>,
                  },
                  {
                    title: "建议",
                    body: <>如果 Free 号经常报模型不支持，优先退回 `auto`。</>,
                  },
                ]}
              />
            }
          >
            <Select
              value={normalizedFreeImageModel}
              onValueChange={(value) => setSection("chatgpt", { ...config.chatgpt, freeImageModel: value })}
            >
              <SelectTrigger className="h-11 rounded-2xl border-stone-200 bg-white shadow-none focus-visible:ring-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {freeModelSelectOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
        {isStudioMode ? (
          <ToggleField
            label="Studio 模式允许禁用未限流账号出图"
            hint="仅对 Studio 模式生效。开启后，已禁用但仍有图片额度、且未异常/未限流的账号也可参与图片生成。"
            tooltip={
              <TooltipDetails
                items={[
                  {
                    title: "开启后",
                    body: <>Studio 模式选号时，会允许“状态禁用但仍有图片额度”的账号继续参与图片生成；异常或已限流账号仍然不会被选中。</>,
                  },
                  {
                    title: "关闭后",
                    body: <>保持当前默认行为：禁用账号不会参与 Studio 图片生成。</>,
                  },
                  {
                    title: "作用范围",
                    body: <>只影响 Studio 模式；CPA 不受这个开关影响。</>,
                  },
                ]}
              />
            }
            checked={config.chatgpt.studioAllowDisabledImageAccounts}
            onCheckedChange={(checked) =>
              setSection("chatgpt", { ...config.chatgpt, studioAllowDisabledImageAccounts: checked })
            }
          />
        ) : null}
        {isStudioMode ? (
          <Field
            label="Paid 账号路由"
            hint="Studio 模式下 Plus / Pro / Team 账号走 legacy（网页 API 接口）还是 responses。"
            tooltip={
              <TooltipDetails
                items={[
                  {
                    title: "responses",
                    body: <>走官方新的付费账号图像链路，功能更完整，当前默认就是这个值。</>,
                  },
                  {
                    title: "legacy",
                    body: <>走当前项目网页 API 接口链路，也就是网页侧会话接口；如果 `responses` 临时异常，可用它做兼容兜底。</>,
                  },
                  {
                    title: "建议",
                    body: <>Paid 账号一般优先保留 `responses`，只有排障时再切到 `legacy`。</>,
                  },
                ]}
              />
            }
          >
            <Select
              value={config.chatgpt.paidImageRoute}
              onValueChange={(value) =>
                setSection("chatgpt", {
                  ...config.chatgpt,
                  paidImageRoute: value,
                  paidImageModel: normalizeImageRouteModel(
                    config.chatgpt.paidImageModel,
                    getPaidImageModelOptions(value),
                    "gpt-5.4-mini",
                  ),
                })
              }
            >
              <SelectTrigger className="h-11 rounded-2xl border-stone-200 bg-white shadow-none focus-visible:ring-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {imageRouteOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
        {isStudioMode ? (
          <Field
            label="Paid 模型"
            hint={
              config.chatgpt.paidImageRoute === "responses"
                ? "Paid 走 responses 时只保留 gpt-5.* 兼容模型；gpt-image-2 已移除，避免触发不兼容请求。"
                : "Studio 模式下 Paid 账号真正发给官方的模型。legacy 路由可继续使用 gpt-image-2 或 gpt-5.*。"
            }
            tooltip={
              <TooltipDetails
                items={[
                  {
                    title: "路由绑定",
                    body: (
                      <>
                        <code>responses</code> 路由只允许 <code>gpt-5.4-mini</code>、<code>gpt-5.4</code>、
                        <code>gpt-5.5</code>、<code>gpt-5-5-thinking</code>；<code>gpt-image-2</code> 已禁用。
                      </>
                    ),
                  },
                  {
                    title: "区别",
                    body: <>`mini` 通常更轻更稳，完整版模型能力更强但上游限制也可能更多。</>,
                  },
                  {
                    title: "排障建议",
                    body: <>如果 Paid 路线报模型不可用，先改回 `gpt-5.4-mini` 再试。</>,
                  },
                ]}
              />
            }
          >
            <Select
              value={normalizedPaidImageModel}
              onValueChange={(value) => setSection("chatgpt", { ...config.chatgpt, paidImageModel: value })}
            >
              <SelectTrigger className="h-11 rounded-2xl border-stone-200 bg-white shadow-none focus-visible:ring-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {paidModelSelectOptions.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
      </ConfigSection>

      <ConfigSection
        title="CPA 配置"
        description="图片请求读取 [cpa].base_url / [cpa].api_key；CPA 管理同步读取 [sync].base_url / [sync].management_key。若 [cpa].base_url 留空，会自动回退使用 [sync].base_url。"
        actions={
          <Button
            type="button"
            variant="outline"
            className="h-10 rounded-full border-stone-200 bg-white px-4 text-stone-700 shadow-none"
            onClick={() => void handleTestCPA()}
            disabled={isTestingCPA}
          >
            {isTestingCPA ? <LoaderCircle className="size-4 animate-spin" /> : <Link2 className="size-4" />}
            测试连接
          </Button>
        }
      >
        <Field
          label="当前生效 CPA 图片地址"
          hint="运行时优先读取 [cpa].base_url；为空时回退 [sync].base_url。这里只是回显当前生效值，不会改写配置文件。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "读取顺序",
                  body: (
                    <>
                      先看 <code>cpa.base_url</code>，如果为空再回退到 <code>sync.base_url</code>。
                    </>
                  ),
                },
                {
                  title: "用途",
                  body: <>这里只是只读回显，方便你确认运行时真正会打到哪个 CPA 地址。</>,
                },
                {
                  title: "排查",
                  body: <>这里显示“未配置”时，CPA 模式会直接报未配置。</>,
                },
              ]}
            />
          }
          fullWidth
        >
          <Input
            value={effectiveCPAImageBaseUrl || "未配置"}
            readOnly
            className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none"
          />
        </Field>
        <Field
          label="CPA 管理 Key 状态"
          hint="对应 [sync].management_key，仅用于账号同步管理接口，不参与图片生成请求。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "作用",
                  body: <>只显示当前是否已经填写管理 Key，本身不会把 Key 明文展示出来。</>,
                },
                {
                  title: "用于哪里",
                  body: <>只用于账号同步管理接口，不用于图片生成、不等于 CPA 图片 API Key。</>,
                },
                {
                  title: "排查",
                  body: <>如果这里未配置，账号管理页的同步状态和推拉同步通常会报 401 或未配置。</>,
                },
              ]}
            />
          }
          fullWidth
        >
          <Input
            value={syncManagementKeyStatus}
            readOnly
            className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none"
          />
        </Field>
        <Field
          label="CPA 图片 Base URL"
          hint="对应 [cpa].base_url，例如 http://127.0.0.1:8317。留空时会回退复用 [sync].base_url。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "填写示例",
                  body: (
                    <>
                      <code>http://127.0.0.1:8317</code>、<code>https://your-cpa.example.com</code>。
                    </>
                  ),
                },
                {
                  title: "怎么填",
                  body: <>填服务根地址即可，通常不需要手动加 `/v1/images` 之类的具体路径。</>,
                },
                {
                  title: "留空效果",
                  body: <>留空后会自动复用下面的 CPA 管理 Base URL。</>,
                },
              ]}
            />
          }
        >
          <Input
            value={config.cpa.baseUrl}
            onChange={(event) => setSection("cpa", { ...config.cpa, baseUrl: event.target.value })}
            placeholder={config.sync.baseUrl || "http://127.0.0.1:8317"}
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <Field
          label="CPA 图片 API Key"
          hint="对应 [cpa].api_key，用于调用 CPA OpenAI 图片接口的 Bearer key。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "填写示例",
                  body: (
                    <>
                      例如 <code>sk-xxxx</code> 或你在 CPA 服务端配置的 Bearer Key。
                    </>
                  ),
                },
                {
                  title: "用途",
                  body: <>只在 CPA 图片接口调用时带上，不参与账号同步管理。</>,
                },
                {
                  title: "未填写影响",
                  body: <>CPA 模式会报“CPA 图片接口未配置”或鉴权失败。</>,
                },
              ]}
            />
          }
        >
          <Input
            type="password"
            value={config.cpa.apiKey}
            onChange={(event) => setSection("cpa", { ...config.cpa, apiKey: event.target.value })}
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <Field
          label="CPA 图片请求超时（秒）"
          hint="对应 [cpa].request_timeout，当前项目调用 CPA 图片接口时使用的 HTTP 超时。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "作用",
                  body: <>控制当前项目请求 CPA 图片接口时，单次 HTTP 请求最多等待多久。</>,
                },
                {
                  title: "建议值",
                  body: <>通常填 `60` 到 `120`；CPA 服务排队慢时可以适当加大。</>,
                },
                {
                  title: "太小的表现",
                  body: <>会更容易出现请求超时，但并不代表 CPA 端一定真的失败了。</>,
                },
              ]}
            />
          }
        >
          <Input
            type="number"
            value={String(config.cpa.requestTimeout)}
            onChange={(event) =>
              setSection("cpa", {
                ...config.cpa,
                requestTimeout: Number(event.target.value || 0),
              })
            }
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <Field
          label="CPA 图片子路由"
          hint={cpaRouteStrategyOptions.find((item) => item.value === config.cpa.routeStrategy)?.hint || ""}
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "images_api",
                  body: <>通过 CPA 的 <code>/v1/images/generations</code> 与 <code>/v1/images/edits</code> 包装层请求图片，请求模型就是 `gpt-image-2`。</>,
                },
                {
                  title: "codex_responses",
                  body: <>直接请求 CPA 的 codex responses 图片子链路。当前顶层主模型会是 `gpt-5.4-mini`，但图片工具模型仍固定为 `gpt-image-2`。</>,
                },
                {
                  title: "auto",
                  body: <>先尝试图片包装层；如果命中特定已知错误，再自动回退到 `codex_responses`，这时日志里的上游模型会显示成 `gpt-5.4-mini (tool: gpt-image-2)`。</>,
                },
              ]}
            />
          }
        >
          <Select
            value={config.cpa.routeStrategy}
            onValueChange={(value) =>
              setSection("cpa", {
                ...config.cpa,
                routeStrategy: value as ConfigPayload["cpa"]["routeStrategy"],
              })
            }
          >
            <SelectTrigger className="h-11 rounded-2xl border-stone-200 bg-white shadow-none focus-visible:ring-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {cpaRouteStrategyOptions.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field
          label="CPA 管理 Base URL"
          hint="对应 [sync].base_url，用于账号同步管理接口，与上面的 CPA 图片地址可相同也可不同。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "填写示例",
                  body: (
                    <>
                      <code>http://127.0.0.1:8317</code>，如果图片接口和管理接口在同一服务上也可以填同一个地址。
                    </>
                  ),
                },
                {
                  title: "用途",
                  body: <>账号管理页的同步状态、从 CPA 同步、同步至 CPA 都会使用这里。</>,
                },
                {
                  title: "注意",
                  body: <>这里是管理接口地址，不等于上面的图片 API Key 鉴权地址。</>,
                },
              ]}
            />
          }
        >
          <Input
            value={config.sync.baseUrl}
            onChange={(event) => setSection("sync", { ...config.sync, baseUrl: event.target.value })}
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <Field
          label="CPA 管理 Key"
          hint="对应 [sync].management_key，只用于 CPA 管理接口同步，不用于图片生成。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "来源",
                  body: <>填 CPA 服务端用于管理接口的 Key，不是图片 API Key，也不是账号 access token。</>,
                },
                {
                  title: "错误现象",
                  body: <>填错时，账号管理页通常会看到 `invalid management key` 或 401。</>,
                },
                {
                  title: "用途",
                  body: <>只影响号池同步、状态拉取和远端管理，不影响图片生成链路。</>,
                },
              ]}
            />
          }
        >
          <Input
            type="password"
            value={config.sync.managementKey}
            onChange={(event) => setSection("sync", { ...config.sync, managementKey: event.target.value })}
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <Field
          label="同步请求超时（秒）"
          hint="对应 [sync].request_timeout，本地和 CPA 管理端同步时使用。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "作用",
                  body: <>只影响同步管理接口，不影响图片生成链路。</>,
                },
                {
                  title: "建议值",
                  body: <>通常 `10` 到 `30` 秒就够；远端管理服务较慢时可以适当加大。</>,
                },
              ]}
            />
          }
        >
          <Input
            type="number"
            value={String(config.sync.requestTimeout)}
            onChange={(event) =>
              setSection("sync", {
                ...config.sync,
                requestTimeout: Number(event.target.value || 0),
              })
            }
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <Field
          label="同步并发"
          hint="对应 [sync].concurrency，批量同步时最多同时处理多少个文件。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "作用",
                  body: <>控制同步时同时上传 / 下载多少个认证文件。</>,
                },
                {
                  title: "建议值",
                  body: <>通常 `2` 到 `6` 更稳；远端服务吃不消时应适当调低。</>,
                },
                {
                  title: "太高的影响",
                  body: <>可能让远端管理服务更容易超时、报错或返回部分失败。</>,
                },
              ]}
            />
          }
        >
          <Input
            type="number"
            value={String(config.sync.concurrency)}
            onChange={(event) =>
              setSection("sync", {
                ...config.sync,
                concurrency: Number(event.target.value || 0),
              })
            }
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <Field
          label="同步 Provider 类型"
          hint="对应 [sync].provider_type，用于筛选同步到远端的认证类型。通常保持 codex。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "常见值",
                  body: (
                    <>
                      一般保持 <code>codex</code>。
                    </>
                  ),
                },
                {
                  title: "作用",
                  body: <>同步时只处理匹配这个 provider 的认证文件，避免不同类型账号混进同一套同步池。</>,
                },
              ]}
            />
          }
        >
          <Input
            value={config.sync.providerType}
            onChange={(event) => setSection("sync", { ...config.sync, providerType: event.target.value })}
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
        <ToggleField
          label="启用 CPA 同步"
          hint="开启后才允许账号管理页执行本地号池与 CPA 之间的双向同步。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "开启后",
                  body: <>账号管理页可以读取远端同步状态，也可以执行“从 CPA 同步 / 同步至 CPA”。</>,
                },
                {
                  title: "关闭后",
                  body: <>同步相关能力会被禁用，但纯图片生成模式本身不受这个开关直接控制。</>,
                },
                {
                  title: "适用场景",
                  body: <>只有你确实需要本地号池和 CPA 远端号池双向同步时才建议开启。</>,
                },
              ]}
            />
          }
          checked={config.sync.enabled}
          onCheckedChange={(checked) => setSection("sync", { ...config.sync, enabled: checked })}
        />
      </ConfigSection>
    </>
  );
}
