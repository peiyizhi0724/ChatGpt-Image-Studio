#!/usr/bin/env bash
set -euo pipefail

API_TARGET="${1:-https://mimo.iqei.cn}"
PORT="${2:-5174}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORTAL_DIR="$REPO_ROOT/portal"
API_TARGET="${API_TARGET%/}"

if [ -z "$API_TARGET" ]; then
  echo "Api target cannot be empty." >&2
  exit 1
fi

cd "$PORTAL_DIR"

if [ ! -d node_modules ]; then
  echo "[1/2] Installing portal frontend dependencies..."
  npm ci
fi

echo "[2/2] Starting portal frontend on http://127.0.0.1:$PORT"
echo "Proxy target: $API_TARGET"
export VITE_API_PROXY_TARGET="$API_TARGET"
unset VITE_API_URL || true
npm run dev -- --port "$PORT"
