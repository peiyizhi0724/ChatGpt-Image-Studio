"use client";

import { Input } from "@/components/ui/input";
import type { ConfigPayload } from "@/lib/api";

import {
  ConfigSection,
  Field,
  ToggleField,
  TooltipDetails,
  type SetConfigSection,
} from "./shared";

type MailSectionProps = {
  config: ConfigPayload;
  setSection: SetConfigSection;
};

export function MailSection({ config, setSection }: MailSectionProps) {
  return (
    <ConfigSection
      title="邮件配置"
      description="多人门户的邮箱注册、验证码发送会走这里。保存后立即生效，不需要额外改 config.toml。"
    >
      <ToggleField
        label="启用邮箱验证码"
        hint="关闭后，门户注册接口会直接返回“mail sender is not configured”，不会再尝试发信。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "开启后",
                body: <>多人门户注册时会先发送邮箱验证码，再允许完成注册。</>,
              },
              {
                title: "关闭后",
                body: <>即使下面 SMTP 参数都填好了，也不会真的发信，适合临时停用注册邮件。</>,
              },
            ]}
          />
        }
        checked={config.mail.enabled}
        onCheckedChange={(checked) =>
          setSection("mail", {
            ...config.mail,
            enabled: checked,
          })
        }
      />

      <Field
        label="SMTP 主机"
        hint="例如 163 邮箱常见是 smtp.163.com。留空时后端会尝试按用户名邮箱自动推断。"
      >
        <Input
          value={config.mail.smtpHost}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              smtpHost: event.target.value,
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="SMTP 端口"
        hint="隐式 TLS 通常用 465，STARTTLS 常见用 587。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "163 邮箱",
                body: (
                  <>
                    常见组合是 <code>smtp.163.com</code> + <code>465</code> + 开启隐式 TLS。
                  </>
                ),
              },
              {
                title: "建议",
                body: <>如果你使用的是授权码登录，先优先按邮箱服务商推荐端口填写，不要盲猜。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.mail.smtpPort)}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              smtpPort: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="登录用户名"
        hint="通常就是发件邮箱地址；部分邮箱服务商也可能要求单独的 SMTP 用户名。"
      >
        <Input
          value={config.mail.username}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              username: event.target.value,
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="SMTP 密码 / 授权码"
        hint="建议填写邮箱服务商提供的 SMTP 授权码，而不是网页登录密码。"
      >
        <Input
          type="password"
          value={config.mail.password}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              password: event.target.value,
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="发件邮箱"
        hint="邮件头里的 From 地址。大多数服务商要求它和 SMTP 登录身份一致。"
      >
        <Input
          type="email"
          value={config.mail.fromAddress}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              fromAddress: event.target.value,
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="发件人名称"
        hint="收件箱里展示的品牌名，例如 Cheilins Studio。"
      >
        <Input
          value={config.mail.fromName}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              fromName: event.target.value,
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <ToggleField
        label="使用隐式 TLS"
        hint="465 端口常用这个模式；关闭时后端会走普通 SMTP + STARTTLS（如果服务端支持）。"
        checked={config.mail.useImplicitTLS}
        onCheckedChange={(checked) =>
          setSection("mail", {
            ...config.mail,
            useImplicitTLS: checked,
          })
        }
      />

      <Field
        label="验证码有效期（分钟）"
        hint="门户注册时，验证码在这个时间内有效。"
      >
        <Input
          type="number"
          min="1"
          value={String(config.mail.codeTTLMinutes)}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              codeTTLMinutes: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="重发间隔（秒）"
        hint="同一个邮箱两次申请验证码之间的最小间隔。"
      >
        <Input
          type="number"
          min="1"
          value={String(config.mail.resendInterval)}
          onChange={(event) =>
            setSection("mail", {
              ...config.mail,
              resendInterval: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
    </ConfigSection>
  );
}
