"use client";

import { useEffect, useMemo, useState } from "react";

import { Input } from "@/components/ui/input";
import type { ConfigPayload } from "@/lib/api";

import { ConfigSection, Field, TooltipDetails, type SetConfigSection } from "./shared";

type APIAccessSectionProps = {
  config: ConfigPayload;
  setSection: SetConfigSection;
};

function buildFallbackOrigin(config: ConfigPayload) {
  const host = String(config.server.host || "").trim();
  const port = Number(config.server.port || 0);
  const normalizedHost =
    host && host !== "0.0.0.0" && host !== "::" ? host : "your-domain.example.com";

  if (!port || port === 80) {
    return `http://${normalizedHost}`;
  }
  if (port === 443) {
    return `https://${normalizedHost}`;
  }
  return `http://${normalizedHost}:${port}`;
}

export function APIAccessSection({ config, setSection }: APIAccessSectionProps) {
  const [currentOrigin, setCurrentOrigin] = useState("");

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    setCurrentOrigin(window.location.origin);
  }, []);

  const apiBaseUrl = useMemo(
    () => currentOrigin || buildFallbackOrigin(config),
    [config, currentOrigin],
  );
  const imageGenerationUrl = useMemo(() => `${apiBaseUrl}/v1/images/generations`, [apiBaseUrl]);

  return (
    <ConfigSection
      title="开放 API 配置"
      description="用于给 Cherry Studio 或其他第三方 OpenAI 兼容客户端接入当前项目的图片 API。建议将第三方调用密钥和后台登录密钥分开管理。"
    >
      <Field
        label="第三方 API Base URL"
        hint="Cherry Studio 的 API 地址直接填这里，不需要手动补 /v1/images/generations。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "怎么填",
                body: <>第三方客户端通常只需要服务根地址，例如 `https://your-domain.example.com`。</>,
              },
              {
                title: "当前显示规则",
                body: <>优先显示你当前打开此页面的域名；如果还拿不到浏览器地址，就回退按监听地址和端口拼一个示例。</>,
              },
            ]}
          />
        }
      >
        <Input value={apiBaseUrl} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>
      <Field
        label="推荐模型名"
        hint="Cherry Studio 等第三方客户端里的模型名建议填这个。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "常用值",
                body: <>当前项目图片 API 的常用模型名是 `gpt-image-2`。</>,
              },
              {
                title: "说明",
                body: <>如果你后续切换了上游策略，这里的填写方式通常也不需要改，仍然以项目对外兼容的模型名为准。</>,
              },
            ]}
          />
        }
      >
        <Input value="gpt-image-2" readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>
      <Field
        label="第三方图片 API Key 列表"
        hint="用于第三方 Bearer 调用图片 API，多个 key 可用英文逗号分隔。建议不要和后台登录密钥复用。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "格式",
                body: <>支持多个 key，用英文逗号分隔，例如 `cherry-a,cherry-b,cherry-c`。</>,
              },
              {
                title: "第三方怎么用",
                body: <>第三方客户端把其中任意一个 key 放进 `Authorization: Bearer ...` 即可调用图片接口。</>,
              },
              {
                title: "安全建议",
                body: <>这是给第三方使用的专用 key；建议和后台登录密钥分开，方便随时单独轮换或停用。</>,
              },
            ]}
          />
        }
      >
        <Input
          value={config.app.apiKey}
          onChange={(event) => setSection("app", { ...config.app, apiKey: event.target.value })}
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="后台登录密钥"
        hint="只用于登录账号管理、配置管理、调用请求页面。不要把这个密钥提供给第三方客户端。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>这个密钥保护的是后台管理页面，不是给 Cherry Studio 之类的第三方客户端使用的。</>,
              },
              {
                title: "建议",
                body: <>即使图片接口也兼容这个密钥，仍然建议后台管理和第三方调用分开配置，避免把后台权限一并暴露出去。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="password"
          value={config.app.authKey}
          onChange={(event) => setSection("app", { ...config.app, authKey: event.target.value })}
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="图片生成接口"
        hint="如果某些第三方工具要求你手动填写完整接口地址，可以直接使用这里。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "常用接口",
                body: <>当前项目还兼容 `/v1/images/edits`、`/v1/images/upscale`、`/v1/models` 等 OpenAI 风格接口。</>,
              },
              {
                title: "Cherry Studio",
                body: <>Cherry Studio 一般只需要填上面的 Base URL 和 API Key，这里通常只是排障或对接其他客户端时备用。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input value={imageGenerationUrl} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>
    </ConfigSection>
  );
}
