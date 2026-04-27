"use client";

import { Input } from "@/components/ui/input";
import type { ConfigPayload } from "@/lib/api";

import { ConfigSection, Field, ToggleField, TooltipDetails, type SetConfigSection } from "./shared";

type RuntimeSectionProps = {
  config: ConfigPayload;
  setSection: SetConfigSection;
};

export function RuntimeSection({ config, setSection }: RuntimeSectionProps) {
  return (
    <ConfigSection title="基础运行配置" description="后端服务监听地址、端口、上传大小、超时和账号刷新等常用配置。修改监听地址或端口后，需要重启程序才会生效。">
      <Field
        label="当前版本"
        hint="只读：当前后端返回的版本号。发布版会由构建流程注入。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "来源",
                body: <>由后端运行时返回；发布版会在构建时写入版本号、提交号和构建时间。</>,
              },
              {
                title: "用途",
                body: <>排查“用户下载的到底是哪一版”时，优先看这里，不要只看前端角落的展示版本。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input value={config.app.version} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>
      <Field
        label="监听地址"
        hint="默认 0.0.0.0。只想本机访问时可改成 127.0.0.1。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "常用值",
                body: (
                  <>
                    <code>0.0.0.0</code> 表示局域网可访问，<code>127.0.0.1</code> 表示只允许本机访问。
                  </>
                ),
              },
              {
                title: "建议",
                body: <>只是自己本机使用时填 `127.0.0.1` 更收敛；需要局域网访问再用 `0.0.0.0`。</>,
              },
            ]}
          />
        }
      >
        <Input
          value={config.server.host}
          onChange={(event) => setSection("server", { ...config.server, host: event.target.value })}
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="监听端口"
        hint="程序启动失败提示端口占用时，通常先改这里。保存后会写入 data/config.toml，但要重启程序才会真正切到新端口。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "填写示例",
                body: (
                  <>
                    <code>7000</code>、<code>8080</code>、<code>9000</code>。
                  </>
                ),
              },
              {
                title: "改完后",
                body: <>配置会立即写入文件，但服务需要重启后才会真正切到新端口。</>,
              },
              {
                title: "排查",
                body: <>如果启动时报端口占用，最直接的处理就是改成一个空闲端口再重启。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.server.port)}
          onChange={(event) =>
            setSection("server", {
              ...config.server,
              port: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="图片并发上限"
        hint="同时允许多少个图片生成/编辑请求进入执行。"
      >
        <Input
          type="number"
          value={String(config.server.maxImageConcurrency)}
          onChange={(event) =>
            setSection("server", {
              ...config.server,
              maxImageConcurrency: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="图片排队上限"
        hint="超出并发上限后，最多允许多少个请求继续排队等待。"
      >
        <Input
          type="number"
          value={String(config.server.imageQueueLimit)}
          onChange={(event) =>
            setSection("server", {
              ...config.server,
              imageQueueLimit: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="图片准入排队超时（秒）"
        hint="超出并发上限后，在 admission 队列里等待超过这个时间，请求会直接返回超时。"
      >
        <Input
          type="number"
          value={String(config.server.imageQueueTimeoutSeconds)}
          onChange={(event) =>
            setSection("server", {
              ...config.server,
              imageQueueTimeoutSeconds: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="任务队列过期（秒）"
        hint="任务创建后如果一直没有真正开跑，超过这个时间会在工作台里标记为已过期。建议明显大于准入排队超时。"
      >
        <Input
          type="number"
          value={String(config.server.imageTaskQueueTtlSeconds)}
          onChange={(event) =>
            setSection("server", {
              ...config.server,
              imageTaskQueueTtlSeconds: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="UI 登录密钥"
        hint="账号管理、配置管理、调用请求页面使用的 Bearer 密钥。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "填写示例",
                body: <>建议填一个你自己定义的长随机串，不要继续使用默认弱口令。</>,
              },
              {
                title: "作用",
                body: <>进入管理页面时会校验这个 Bearer 密钥；它保护的是后台管理，不是图片 API。</>,
              },
              {
                title: "改完影响",
                body: <>保存后新请求就会按新密钥校验，旧页面可能需要重新登录。</>,
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
        label="图片 API Key 列表"
        hint="用于调用当前项目图片接口的 Bearer key，多个可逗号分隔。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "格式",
                body: <>支持多个 key，用英文逗号分隔，例如 `key-a,key-b,key-c`。</>,
              },
              {
                title: "作用",
                body: <>调用当前项目对外暴露的图片 API 时，会校验请求头里的 Bearer token 是否命中这里。</>,
              },
              {
                title: "留空效果",
                body: <>留空时图片 API 不做鉴权，任何人都能调，公网环境下不建议这么配。</>,
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
        label="图片返回格式"
        hint="当前项目自身图片接口默认返回格式。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "可选值",
                body: (
                  <>
                    <code>url</code> 或 <code>b64_json</code>。
                  </>
                ),
              },
              {
                title: "url",
                body: <>返回图片访问地址，响应体更小，适合网页或普通 API 转发。</>,
              },
              {
                title: "b64_json",
                body: <>直接返回 Base64 图片数据，方便一次性拿到完整内容，但响应会更大。</>,
              },
            ]}
          />
        }
      >
        <Input
          value={config.app.imageFormat}
          onChange={(event) => setSection("app", { ...config.app, imageFormat: event.target.value })}
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="最大上传大小（MB）"
        hint="当前项目图片上传的最大体积限制。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>限制单张上传图片的最大体积，超出后会在进入模型前就被后端拦掉。</>,
              },
              {
                title: "建议值",
                body: <>通常 `20` 到 `100` MB 足够；上传原图较大时可适当调高。</>,
              },
              {
                title: "太小的表现",
                body: <>编辑或参考图上传会直接失败，即使图片本身格式没问题也会被拒绝。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.app.maxUploadSizeMB)}
          onChange={(event) =>
            setSection("app", {
              ...config.app,
              maxUploadSizeMB: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="图片请求超时（秒）"
        hint="当前项目请求官方图片接口时的超时。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>控制普通官方 HTTP 请求的超时，例如非流式接口和常规下载请求。</>,
              },
              {
                title: "建议值",
                body: <>默认是 `120` 秒；网络较差或上游响应慢时可以在这个基础上继续加大。</>,
              },
              {
                title: "不要混淆",
                body: <>这个值不是长时间图片生成等待的总超时，长等待主要看下面的 SSE 超时。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.chatgpt.requestTimeout)}
          onChange={(event) =>
            setSection("chatgpt", {
              ...config.chatgpt,
              requestTimeout: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="SSE 超时（秒）"
        hint="官方 SSE 图像链路的整体等待超时。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>控制长时间流式图像任务能等多久，生成慢图时主要看这个值。</>,
              },
              {
                title: "建议值",
                body: <>默认是 `600` 秒；通常应明显大于普通请求超时，长图或排队场景建议保留较大值。</>,
              },
              {
                title: "太小的表现",
                body: <>长图或高峰期排队时更容易出现“timed out waiting for async image generation”。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.chatgpt.sseTimeout)}
          onChange={(event) =>
            setSection("chatgpt", {
              ...config.chatgpt,
              sseTimeout: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="轮询间隔（秒）"
        hint="legacy 图像链路轮询间隔。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>只影响 `legacy` 图像链路，控制后端多久去问一次任务是否完成。</>,
              },
              {
                title: "调小后",
                body: <>结果会更快被发现，但请求次数会更多，更容易增加噪音和上游压力。</>,
              },
              {
                title: "建议值",
                body: <>`2` 到 `5` 秒比较常见，默认 `3` 秒通常够用。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.chatgpt.pollInterval)}
          onChange={(event) =>
            setSection("chatgpt", {
              ...config.chatgpt,
              pollInterval: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="轮询最大等待（秒）"
        hint="legacy 图像链路最长等待时间。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>给 `legacy` 轮询链路设一个总等待上限，超过后本次请求会被判定超时。</>,
              },
              {
                title: "建议值",
                body: <>通常应明显大于轮询间隔，例如 `120` 到 `300` 秒。</>,
              },
              {
                title: "太小的表现",
                body: <>任务其实还在上游处理中，但本地会先报超时，看起来像“图还没回来就失败”。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.chatgpt.pollMaxWait)}
          onChange={(event) =>
            setSection("chatgpt", {
              ...config.chatgpt,
              pollMaxWait: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="默认额度"
        hint="本地未刷新到真实额度时的默认 quota。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>当后端暂时拿不到真实图片额度时，会先用这个数值做本地初始显示和兜底。</>,
              },
              {
                title: "建议值",
                body: <>小号测试可以填 `5` 或 `10`；它不是官方真实额度，只是本地兜底值。</>,
              },
              {
                title: "注意",
                body: <>填得再大也不会真的让账号有更多图片额度，只会影响本地展示与初始判断。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.accounts.defaultQuota)}
          onChange={(event) =>
            setSection("accounts", {
              ...config.accounts,
              defaultQuota: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="刷新并发"
        hint="账号刷新信息时的 worker 数量。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>控制批量刷新账号信息时同时开多少个 worker 去请求上游。</>,
              },
              {
                title: "建议值",
                body: <>一般 `4` 到 `8` 比较稳；账号很多时可适当加大，但不建议无限拉高。</>,
              },
              {
                title: "太高的影响",
                body: <>可能更快触发限流、网络抖动或让刷新结果看起来更不稳定。</>,
              },
            ]}
          />
        }
      >
        <Input
          type="number"
          value={String(config.accounts.refreshWorkers)}
          onChange={(event) =>
            setSection("accounts", {
              ...config.accounts,
              refreshWorkers: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <Field
        label="额度刷新 TTL（秒）"
        hint="账号刚刷新过额度时，在这个时间窗口内优先复用结果，减少重复打上游。"
      >
        <Input
          type="number"
          value={String(config.accounts.imageQuotaRefreshTTLSeconds)}
          onChange={(event) =>
            setSection("accounts", {
              ...config.accounts,
              imageQuotaRefreshTTLSeconds: Number(event.target.value || 0),
            })
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>
      <ToggleField
        label="优先远端刷新额度"
        hint="开启后，后端会优先尝试刷新真实额度状态，而不是只依赖本地扣减。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "开启后",
                body: <>会优先尝试去远端拿真实额度与状态，显示更准，但刷新耗时通常更长。</>,
              },
              {
                title: "关闭后",
                body: <>更依赖本地缓存和扣减，速度更快，但显示可能比真实情况更滞后。</>,
              },
              {
                title: "建议",
                body: <>如果你更在意额度展示准确性，建议保持开启。</>,
              },
            ]}
          />
        }
        checked={config.accounts.preferRemoteRefresh}
        onCheckedChange={(checked) => setSection("accounts", { ...config.accounts, preferRemoteRefresh: checked })}
      />
      <ToggleField
        label="记录所有请求日志"
        hint="开启后后端会输出更详细的请求日志，排障更方便，但噪音也会更大。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "开启后",
                body: <>控制台和日志里会输出更细的请求方向、路由、账号类型等信息，排障更方便。</>,
              },
              {
                title: "关闭后",
                body: <>日志更干净，适合日常稳定运行，但定位复杂问题时信息会少一些。</>,
              },
              {
                title: "注意",
                body: <>日志量会明显增加，长期运行时要注意不要把本地日志刷得太大。</>,
              },
            ]}
          />
        }
        checked={config.log.logAllRequests}
        onCheckedChange={(checked) => setSection("log", { ...config.log, logAllRequests: checked })}
      />
    </ConfigSection>
  );
}
