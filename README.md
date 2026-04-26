# ChatGpt Image Studio

ChatGpt Image Studio is a local image workflow project built on top of the upstream open-source repository.

This customized version keeps the original Studio / CPA image generation capability, and adds:

- stable multi-account image concurrency control
- single-account single-task leasing
- local browser-side free-account grouping policy
- reserve threshold based account rollover
- runtime / startup diagnostics
- one-click Windows start / stop scripts
- desktop shortcut and auto-start helper scripts

## What This Version Adds

### 1. Stable image concurrency

The backend now supports:

- global image concurrency limit
- short queue with timeout
- token-level account lease
- per-request runtime logging

Default runtime values:

- max concurrency: `8`
- queue limit: `32`
- queue timeout: `20s`
- quota refresh TTL: `120s`

### 2. Free account grouping policy

The accounts page now supports a browser-local routing policy for free accounts.

Default behavior:

- sort mode: `imported_at`
- group size: `10`
- enabled groups: first `2`
- reserve threshold: `20%`

The policy is sent in the request header:

- `X-Studio-Account-Policy`

If the header is missing, backend behavior stays compatible with the old selection logic.

### 3. One-click Windows launch

You can now run the project without typing terminal commands every time.

Root-level launch files:

- `start-studio.cmd`
- `stop-studio.cmd`
- `studio-status.cmd`
- `open-studio-logs.cmd`

Helper scripts:

- `create-desktop-shortcuts.cmd`
- `create-startup-shortcut.cmd`
- `remove-startup-shortcut.cmd`

## Repository Layout

```text
.
|- backend/                   Go backend
|  |- api/                    HTTP handlers and image workflow API
|  |- internal/               config, accounts, sync, runtime helpers
|  |- data/                   local runtime config and account data
|  `- go.mod
|- web/                       Vite + React frontend
|  |- src/
|  `- scripts/
|- scripts/                   local dev / build / startup helper scripts
|- start-studio.cmd
|- stop-studio.cmd
|- studio-status.cmd
`- README.md
```

## Requirements

- Windows 10/11 recommended for the one-click launcher flow
- Go `1.25+`
- Node.js `24+`
- npm `10+`

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/peiyizhi0724/ChatGpt-Image-Studio.git
cd ChatGpt-Image-Studio
```

### 2. Prepare config

Runtime config location:

- `backend/data/config.toml`

If the file does not exist, the backend can create it from the default template on first run.

Minimum example:

```toml
[app]
auth_key = "chatgpt2api"
```

If you use Studio mode with a fixed proxy:

```toml
[proxy]
enabled = true
url = "socks5h://127.0.0.1:10808"
mode = "fixed"
sync_enabled = false
```

If you use CPA image mode:

```toml
[chatgpt]
image_mode = "cpa"

[cpa]
base_url = "http://127.0.0.1:8317"
api_key = "your-cpa-key"
request_timeout = 120
route_strategy = "auto"
```

### 3. Start the project

Recommended on Windows:

- double-click `start-studio.cmd`

This will:

1. build frontend static assets
2. sync frontend assets into `backend/static`
3. start backend on the configured port
4. open the browser automatically

Default URL:

- `http://127.0.0.1:7000`

### 4. Stop the project

- double-click `stop-studio.cmd`

### 5. Check logs

- double-click `open-studio-logs.cmd`

Log files:

- `.runtime/studio.out.log`
- `.runtime/studio.err.log`

## Desktop Shortcuts and Auto Start

### Desktop shortcuts

To create Windows desktop shortcuts:

- double-click `create-desktop-shortcuts.cmd`

Generated shortcuts:

- `生图工作台-启动`
- `生图工作台-停止`
- `生图工作台-状态`
- `生图工作台-日志`

### Auto start at sign-in

To create a Windows startup shortcut:

- double-click `create-startup-shortcut.cmd`

To remove it:

- double-click `remove-startup-shortcut.cmd`

## Local Development

### Windows

```powershell
./scripts/dev.ps1
```

### macOS / Linux

```bash
chmod +x ./scripts/*.sh
./scripts/dev.sh
```

## Build

### Windows

```powershell
./scripts/build.ps1
```

### macOS / Linux

```bash
./scripts/build.sh
```

Build output:

- `dist/package/`

## Test

### Windows

```powershell
./scripts/check.ps1
```

### macOS / Linux

```bash
./scripts/check.sh
```

## Important Runtime Pages

- accounts page: account import, quota view, grouping policy
- requests page: route logs, group routing, queue/runtime fields
- settings page: config editing
- startup check page: startup diagnostics

## Free Account Grouping Notes

The grouping policy is intentionally browser-local:

- it is stored in local storage
- it is not written into `/api/config`
- different browsers can use different group selections

Current policy fields:

- `enabled`
- `sortMode`
- `groupSize`
- `enabledGroupIndexes`
- `reserveMode`
- `reservePercent`

Supported sort modes:

- `imported_at`
- `name`
- `quota`

## Runtime Logging

Request logs now expose:

- `queueWaitMs`
- `inflightCountAtStart`
- `leaseAcquired`
- `errorCode`
- `routingPolicyApplied`
- `routingGroupIndex`
- `routingSortMode`
- `routingReservePercent`

## Local Data and Sensitive Files

Do not commit:

- `backend/data/config.toml`
- `backend/data/auths/*.json`
- `backend/data/accounts_state.json`
- `backend/data/sync_state/*.json`
- runtime logs
- temporary image files
- local caches and toolchains

## Syncing With Upstream

See:

- `docs/UPSTREAM-SYNC.md`

## License

This repository is still based on the upstream project and follows the upstream licensing model unless you intentionally change it.
