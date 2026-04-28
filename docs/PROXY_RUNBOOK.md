# Proxy Runbook

这份文档记录 `Studio` 官方链路在生产环境中的代理运维经验，供后续人工排障或其他模型接手时直接参考。

当前内容基于 `2026-04-27` 的线上修复结论整理。

## 适用场景

适用于下面这类部署：

- 后端容器跑在 Docker 中
- 容器内 `proxy.url` 指向宿主机代理，例如 `socks5h://172.24.0.1:7892`
- 宿主机使用 `mihomo` 或兼容 Clash Meta 的服务负责出站
- 图片模式为 `studio`

## 当前生产环境约定

当前线上环境的关键点如下：

- 部署面板：`1Panel`
- 应用容器：`chatgpt-image-studio-studio-1`
- 宿主机代理服务：`mihomo.service`
- 宿主机 `socks-port`：`7892`
- 容器侧访问地址：`172.24.0.1:7892`
- 运行配置文件：`/etc/mihomo/config.yaml`
- 订阅生成脚本：`/usr/local/sbin/update-mihomo-subscription.py`

重要说明：

- `/etc/mihomo/config.yaml` 不是手工长期维护文件，而是会被 `update-mihomo-subscription.py` 重写。
- 如果只修 `/etc/mihomo/config.yaml`，但不修生成脚本，下次订阅更新后问题可能会回来。
- 订阅 URL 可能包含 token，不要把这些敏感地址提交进仓库。

## 常见故障现象

如果代理链路出问题，前端或后端通常会出现这些报错：

- `/backend-api/me failed: ... EOF`
- `sentinel tokens: chat-requirements request: ... EOF`
- `/backend-api/me failed: socks connect tcp 172.24.0.1:7892 ... i/o timeout`

同时，站内诊断页一般会表现为：

- `/api/startup/check`
  - `proxy = pass`
  - `chatgpt = fail`
- `/api/proxy/test`
  - `ok = false`
  - `error` 常见为 `EOF`、超时或上游 SSL 错误

## 这次问题的真实根因

`2026-04-27` 这次线上故障并不是：

- 应用没部署成功
- 账号池没额度
- CPA 链路不可用
- `proxy.url` 写错

而是：

- `172.24.0.1:7892` 这个代理端口本身能连通
- 但 `mihomo` 的 `Proxy` 分组选中了已经失效的上游节点
- 结果容器访问 `chatgpt.com` 时被坏节点拖死，表现为 `EOF` 或超时

这说明：

- `代理端口可连接` 只代表本机代理进程活着
- 不代表代理当前选中的出站节点真的还能访问 ChatGPT

## 排障时的正确判断标准

要区分两件事：

1. 代理进程是否活着
2. 代理选中的具体节点是否真的还能走官方链路

### 可接受的返回

对于匿名探测来说，下列返回通常都可以视作“链路可达”：

- `https://chatgpt.com` 返回 `HTTP 403`
- `https://chatgpt.com/api/auth/csrf` 返回 `HTTP 403`
- `https://chatgpt.com/backend-api/me` 返回 `HTTP 403`
- `https://chatgpt.com/backend-api/sentinel/chat-requirements` 返回 `HTTP 400/401/403/405/409/415/422`

这些状态码往往表示：

- 已经到达 Cloudflare 或 ChatGPT 网关
- 只是因为匿名、缺少 Cookie、风控或请求格式不完整而被拒绝

### 不可接受的返回

下面这些更像是“节点本身不可用”：

- `EOF`
- `i/o timeout`
- `connect timeout`
- `OpenSSL SSL_connect: SSL_ERROR_SYSCALL`
- `curl` 非零退出且没有拿到上游 HTTP 状态码

## 当前推荐的 mihomo 策略

不要再用“极少候选 + fallback 粘住首个节点”的方案。

当前生产环境已经改为：

- 动态候选节点上限：`8`
- 分组类型：`url-test`
- 探测 URL：`https://chatgpt.com/cdn-cgi/trace`
- `interval = 180`
- `tolerance = 100`
- `profile.store-selected = false`

为什么这样配：

- `url-test` 会自动在候选节点里选择可用且延迟更合适的节点
- `store-selected = false` 可以避免 mihomo 把已经失效的旧节点长期缓存成“当前选择”
- 候选池从 `3` 提到 `8` 后，单个节点挂掉时更容易自动切走

## 应用内自动重试

当前项目已经支持：

- 官方图片链路在遇到 `EOF`、`i/o timeout`、连接失败等网络错误时自动重试
- 如果配置了 Mihomo 控制口地址，会在重试前先调用 `Proxy/delay`
- 自动重试不会把 `401 / 403` 这种上游明确拒绝误判成网络故障

默认约定：

- 配置项位于 `[proxy]`
- `auto_retry_enabled = true`
- `controller_group = "Proxy"`
- 如果 `controller_url` 为空，程序会尝试从 `proxy.url` 自动推断同主机的 `:9090` 控制口

## 当前脚本里的关键规则

`/usr/local/sbin/update-mihomo-subscription.py` 目前需要保持这些关键行为：

- `HEALTHCHECK_URL = "https://chatgpt.com/cdn-cgi/trace"`
- `MAX_DYNAMIC_PROXIES = 8`
- 生成的 `Proxy` 组类型为 `url-test`
- 生成的运行配置中包含：
  - `profile:`
  - `store-selected: false`
- `backend-api-me` 探测接受：
  - `200`
  - `401`
  - `403`

如果后续有人改脚本，下面两种改法要避免：

- 把 `MAX_DYNAMIC_PROXIES` 再缩回 `3`
- 把 `url-test` 改回 `fallback` 并打开 `lazy`

## 推荐复查命令

先看代理服务是否活着：

```bash
systemctl status mihomo --no-pager -l
ss -lntp | grep 7892
```

看当前 `Proxy` 组和候选列表：

```bash
curl -s http://127.0.0.1:9090/proxies/Proxy
```

手动触发一次分组测速：

```bash
curl -s "http://127.0.0.1:9090/proxies/Proxy/delay?url=https://chatgpt.com/cdn-cgi/trace&timeout=20000"
```

直接从宿主机验证 ChatGPT 链路：

```bash
curl -I --max-time 20 --proxy socks5h://127.0.0.1:7892 https://chatgpt.com/api/auth/csrf
curl -I --max-time 20 --proxy socks5h://127.0.0.1:7892 https://chatgpt.com
```

在应用侧复查：

```bash
GET  /api/startup/check
POST /api/proxy/test
POST /v1/images/generations
```

## 推荐恢复步骤

如果线上再次出现 `EOF` 或官方生图失败，优先按这个顺序处理：

1. 先确认 `mihomo.service` 和 `7892` 端口是否正常。
2. 再看 `/api/startup/check` 是否是 `proxy = pass` 但 `chatgpt = fail`。
3. 如果是，继续看 `http://127.0.0.1:9090/proxies/Proxy` 当前 `now` 指向哪个节点。
4. 如果 `now` 指向可疑坏节点，先手动触发一次 `Proxy/delay` 复测。
5. 如果配置已被旧脚本覆盖，直接重跑：

```bash
python3 /usr/local/sbin/update-mihomo-subscription.py
```

6. 如果脚本运行后还是只留下很少候选，优先检查脚本里的探测规则，不要只盯着订阅本身。
7. 如果脚本运行后一个候选都留不下，再检查订阅源是否整体失效。

## 这次修复后的预期结果

在 `2026-04-27` 的修复完成后，预期应满足：

- `Proxy` 组为 `URLTest`
- 运行配置中有 `8` 个动态可用美国节点
- `/api/startup/check` 为 `overall = pass`
- `/api/proxy/test` 返回 `ok = true`
- 真实调用 `/v1/images/generations` 可以成功返回图片

如果以后状态偏离这些预期，优先按本文档复查，而不是只看前端报错文案。
