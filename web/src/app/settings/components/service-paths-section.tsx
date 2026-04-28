"use client";

import { useState } from "react";
import { LoaderCircle, PlugZap } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import type { Dispatch, SetStateAction } from "react";

import { Input } from "@/components/ui/input";
import { testProxy, type ConfigPayload } from "@/lib/api";

import { ConfigSection, Field, ToggleField, TooltipDetails } from "./shared";

type ServicePathsSectionProps = {
  config: ConfigPayload;
  setConfig: Dispatch<SetStateAction<ConfigPayload>>;
  resolvedStaticDir: string;
  startupErrorPath: string;
};

export function ServicePathsSection({
  config,
  setConfig,
  resolvedStaticDir,
  startupErrorPath,
}: ServicePathsSectionProps) {
  const [isTestingProxy, setIsTestingProxy] = useState(false);

  const handleTestProxy = async () => {
    setIsTestingProxy(true);
    try {
      const result = await testProxy(config.proxy.url);
      if (result.ok) {
        toast.success(`代理可用，HTTP ${result.status}，耗时 ${result.latency} ms`);
      } else {
        toast.error(result.error || `代理测试失败，HTTP ${result.status}`);
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "代理测试失败");
    } finally {
      setIsTestingProxy(false);
    }
  };

  return (
    <ConfigSection title="服务与路径" description="发布版默认从可执行文件同级的 data/ 和 static/ 读写配置与静态资源，路径通常不建议频繁修改。">
      <Field
        label="静态资源目录"
        hint="发布包默认是 static。开发脚本会把 web/dist 同步到 backend/static。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "常见值",
                body: (
                  <>
                    <code>static</code>，或者绝对路径如 <code>D:\\ChatGpt-Image-Studio\\static</code>。
                  </>
                ),
              },
              {
                title: "相对路径规则",
                body: <>如果填写相对路径，会相对于下面显示的“配置根目录”去解析。</>,
              },
              {
                title: "配错表现",
                body: <>页面可能打开后空白、404，或者一直还是旧前端资源。</>,
              },
            ]}
          />
        }
      >
        <Input
          value={config.server.staticDir}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              server: { ...current.server, staticDir: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="解析后的静态目录"
        hint="只读：当前后端实际读取静态页面的路径。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>展示静态资源目录最终解析出来的真实路径，方便确认当前服务到底在读哪里。</>,
              },
              {
                title: "怎么用",
                body: <>如果你怀疑前端改了但页面没更新，先看这里是不是仍然指向旧目录。</>,
              },
            ]}
          />
        }
      >
        <Input value={resolvedStaticDir} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>

      <ToggleField
        label="启用固定代理"
        hint="用于访问官方图片链路。当前只支持 fixed 模式。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "开启后",
                body: <>官方图片链路会走下面填写的代理 URL 发请求。</>,
              },
              {
                title: "关闭后",
                body: <>官方链路直接本机出网，不经过代理；CPA 链路是否走代理还要看下面的同步复用设置。</>,
              },
              {
                title: "适用场景",
                body: <>本机直连官方不稳定、需要代理出海时才建议开启。</>,
              },
            ]}
          />
        }
        checked={config.proxy.enabled}
        onCheckedChange={(checked) =>
          setConfig((current) => ({
            ...current,
            proxy: { ...current.proxy, enabled: checked },
          }))
        }
      />

      <Field
        label="代理 URL"
        hint="支持 socks5 / socks5h / http / https。默认示例是 socks5h://127.0.0.1:10808。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "填写示例",
                body: (
                  <>
                    <code>socks5h://127.0.0.1:10808</code>、<code>http://127.0.0.1:7890</code>。
                  </>
                ),
              },
              {
                title: "socks5h",
                body: <>DNS 解析也走代理，通常比纯 `socks5` 更适合需要代理域名解析的场景。</>,
              },
              {
                title: "注意",
                body: <>只有在“启用固定代理”打开时，这个地址才会实际生效。</>,
              },
            ]}
          />
        }
      >
        <div className="flex gap-2">
          <Input
            value={config.proxy.url}
            onChange={(event) =>
              setConfig((current) => ({
                ...current,
                proxy: { ...current.proxy, url: event.target.value },
              }))
            }
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
          <Button
            type="button"
            variant="outline"
            className="h-11 shrink-0 rounded-2xl border-stone-200 bg-white px-4 text-stone-700 shadow-none"
            onClick={() => void handleTestProxy()}
            disabled={isTestingProxy}
          >
            {isTestingProxy ? <LoaderCircle className="size-4 animate-spin" /> : <PlugZap className="size-4" />}
            测试
          </Button>
        </div>
      </Field>

      <Field
        label="代理模式"
        hint="当前仅支持 fixed。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "当前可选值",
                body: <>目前只有 <code>fixed</code> 真正有实现，表示始终使用固定一条代理。</>,
              },
              {
                title: "建议",
                body: <>除非后端以后新增其他模式，否则这里保持 `fixed` 不要改。</>,
              },
            ]}
          />
        }
      >
        <Input
          value={config.proxy.mode}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              proxy: { ...current.proxy, mode: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <ToggleField
        label="同步复用代理"
        hint="开启后 CPA 同步请求也会复用同一代理。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "开启后",
                body: <>CPA 同步相关请求也会复用同一条代理，适合管理接口也需要代理出网的情况。</>,
              },
              {
                title: "关闭后",
                body: <>只有官方图片链路走代理，CPA 同步管理请求仍直接走本机网络。</>,
              },
              {
                title: "注意",
                body: <>这个开关影响的是同步管理请求，不直接影响 CPA 图片接口本身。</>,
              },
            ]}
          />
        }
        checked={config.proxy.syncEnabled}
        onCheckedChange={(checked) =>
          setConfig((current) => ({
            ...current,
            proxy: { ...current.proxy, syncEnabled: checked },
          }))
        }
      />

      <ToggleField
        label="网络错误自动重试"
        hint="官方链路遇到 EOF、超时、连接失败等网络错误时，会先尝试触发 Mihomo 复测，再自动重试。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "会重试什么",
                body: <>只针对请求建立阶段的网络错误，例如 EOF、i/o timeout、连接失败，不会把上游明确返回的 403/401 当成网络故障。</>,
              },
              {
                title: "怎么切节点",
                body: <>如果下面填了 Mihomo 控制地址，后端会在重试前触发一次对应分组的 delay 复测，让 URLTest 组优先切到更健康的节点。</>,
              },
              {
                title: "默认行为",
                body: <>控制地址留空时，会尝试从代理地址自动推断本机 `:9090` 控制口；推断不到时就只做延迟重试，不强依赖控制口。</>,
              },
            ]}
          />
        }
        checked={config.proxy.autoRetryEnabled}
        onCheckedChange={(checked) =>
          setConfig((current) => ({
            ...current,
            proxy: { ...current.proxy, autoRetryEnabled: checked },
          }))
        }
      />

      <Field
        label="Mihomo 控制地址"
        hint="可选。示例：http://127.0.0.1:9090 或容器场景下的 http://172.24.0.1:9090。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>用于在网络错误重试前主动调用 Mihomo 控制口的 `Proxy/delay`，让 URLTest 组马上重新测速和切节点。</>,
              },
              {
                title: "留空时",
                body: <>程序会优先尝试从代理 URL 自动推断控制口地址，例如 `socks5h://172.24.0.1:7892` 会推断成 `http://172.24.0.1:9090`。</>,
              },
            ]}
          />
        }
      >
        <Input
          value={config.proxy.controllerUrl}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              proxy: { ...current.proxy, controllerUrl: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="Mihomo 控制密钥"
        hint="可选。只有当控制口开启鉴权时才需要填写。"
      >
        <Input
          type="password"
          value={config.proxy.controllerSecret}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              proxy: { ...current.proxy, controllerSecret: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="Mihomo 分组名"
        hint="默认是 Proxy，对应你当前 Mihomo 配置里的 URLTest 分组名称。"
      >
        <Input
          value={config.proxy.controllerGroup}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              proxy: { ...current.proxy, controllerGroup: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="Auth 目录"
        hint="本地认证文件目录。通常保持默认即可。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "填写示例",
                body: (
                  <>
                    <code>data/auths</code>，或者一个你自己维护的绝对目录。
                  </>
                ),
              },
              {
                title: "作用",
                body: <>账号管理页导入的 auth 文件会主要落在这里，后端读取号池时也会从这里扫描。</>,
              },
              {
                title: "改动风险",
                body: <>改到新目录后，如果旧文件没迁过去，账号列表会像“突然空了”一样。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input
          value={config.storage.authDir}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              storage: { ...current.storage, authDir: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="账号状态文件"
        hint="本地号池状态与额度落盘文件。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "填写示例",
                body: (
                  <>
                    <code>data/accounts_state.json</code>。
                  </>
                ),
              },
              {
                title: "作用",
                body: <>本地账号的状态、额度、刷新结果等缓存信息会保存在这里。</>,
              },
              {
                title: "改动风险",
                body: <>如果指向一个全新文件，页面上的部分本地状态会看起来像被重置。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input
          value={config.storage.stateFile}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              storage: { ...current.storage, stateFile: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      <Field
        label="同步状态目录"
        hint="记录 CPA 同步状态的目录。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "填写示例",
                body: (
                  <>
                    <code>data/sync_state</code>。
                  </>
                ),
              },
              {
                title: "作用",
                body: <>本地与 CPA 远端的同步记录、差异状态和中间状态会写到这里。</>,
              },
              {
                title: "删除或改路径后",
                body: <>系统会把它当成新的同步状态目录，可能重新出现大批“待同步/远端独有”提示。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input
          value={config.storage.syncStateDir}
          onChange={(event) =>
            setConfig((current) => ({
              ...current,
              storage: { ...current.storage, syncStateDir: event.target.value },
            }))
          }
          className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
        />
      </Field>

      {config.storage.imageDataStorage === "server" ? (
        <Field
          label="服务器图片目录"
          hint="仅在图片数据存储选择服务器目录时使用。"
          tooltip={
            <TooltipDetails
              items={[
                {
                  title: "填写示例",
                  body: (
                    <>
                      <code>data/tmp/image</code>。
                    </>
                  ),
                },
                {
                  title: "作用",
                  body: <>工作台图片、兼容接口返回的图片文件，以及网关图片地址对应的文件都会写到这里。</>,
                },
                {
                  title: "注意",
                  body: <>这个目录如果不可写，服务器图片存储和 `url` 返回格式都会出问题。</>,
                },
              ]}
            />
          }
          fullWidth
        >
          <Input
            value={config.storage.imageDir}
            onChange={(event) =>
              setConfig((current) => ({
                ...current,
                storage: { ...current.storage, imageDir: event.target.value },
              }))
            }
            className="h-11 rounded-2xl border-stone-200 bg-white shadow-none"
          />
        </Field>
      ) : null}

      <Field
        label="配置根目录"
        hint="只读：当前后端自动识别到的配置根目录。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>所有相对路径都会以这里为基准解析，发布版通常就是可执行文件所在目录。</>,
              },
              {
                title: "怎么用",
                body: <>判断某个相对路径最终落在哪时，先看这里再看对应配置值。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input value={config.paths.root} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>

      <Field
        label="示例配置文件"
        hint="只读：程序启动时自动写出的示例配置路径。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>程序首次启动时会尝试在这个位置生成示例配置，供用户对照查看。</>,
              },
              {
                title: "注意",
                body: <>它只是示例文件，不是你点击“保存配置”时真正写入的那份配置。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input value={config.paths.defaults} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>

      <Field
        label="覆盖配置文件"
        hint="只读：点击保存后实际写入的配置文件。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "作用",
                body: <>配置管理页点击“保存配置”后，真正会改写的是这里显示的文件。</>,
              },
              {
                title: "怎么用",
                body: <>如果你怀疑页面保存了但程序没读到，先打开这个文件确认内容是否真的改了。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input value={config.paths.override} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>

      <Field
        label="启动错误日志"
        hint="只读：程序启动失败时会将中文错误详情写到这里。"
        tooltip={
          <TooltipDetails
            items={[
              {
                title: "用途",
                body: <>二进制包或本地服务启动失败时，最直接的中文错误信息会落在这里。</>,
              },
              {
                title: "常见场景",
                body: <>端口占用、配置损坏、静态资源缺失、首次生成配置失败时都优先看这个文件。</>,
              },
            ]}
          />
        }
        fullWidth
      >
        <Input value={startupErrorPath} readOnly className="h-11 rounded-2xl border-stone-200 bg-stone-50 shadow-none" />
      </Field>
    </ConfigSection>
  );
}
