#!/usr/bin/env bash
set -euo pipefail

API_TARGET="${1:-https://mimo.iqei.cn}"
PORT="${2:-5173}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$REPO_ROOT/web"
API_TARGET="${API_TARGET%/}"

if [ -z "$API_TARGET" ]; then
  echo "Api target cannot be empty." >&2
  exit 1
fi

cd "$WEB_DIR"

if [ ! -d node_modules ]; then
  echo "[1/2] Installing admin frontend dependencies..."
  npm ci
fi

echo "[2/2] Starting admin frontend on http://127.0.0.1:$PORT"
echo "Proxy target: $API_TARGET"
export VITE_API_PROXY_TARGET="$API_TARGET"
unset VITE_API_URL || true
npm run dev -- --port "$PORT"
