#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$REPO_ROOT/web"
BACKEND_DIR="$REPO_ROOT/backend"
GO_BIN="$REPO_ROOT/.tools/go1.26.2/go/bin/go.exe"

if [ ! -x "$GO_BIN" ]; then
  GO_BIN="go"
fi

export GOMODCACHE="$REPO_ROOT/.gomodcache"
export GOCACHE="$REPO_ROOT/.gocache"
# Use a reachable module proxy by default in this environment.
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-off}"
mkdir -p "$GOMODCACHE" "$GOCACHE"

echo "[1/3] Building frontend static assets..."
cd "$WEB_DIR"
if [ ! -x node_modules/.bin/vite ] || [ ! -x node_modules/.bin/esbuild ]; then
  npm ci --ignore-scripts
fi
npm run build

echo "[2/3] Starting backend on configured port..."
cd "$BACKEND_DIR"
"$GO_BIN" run .
