#!/usr/bin/env bash
set -euo pipefail

PORT="${1:-7000}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$REPO_ROOT/web"
PORTAL_DIR="$REPO_ROOT/portal"
BACKEND_DIR="$REPO_ROOT/backend"

ensure_node_modules() {
  local dir="$1"
  local label="$2"

  cd "$dir"
  if [ ! -d node_modules ]; then
    echo "Installing $label dependencies..."
    npm ci
  fi
}

start_watcher() {
  local dir="$1"
  local label="$2"
  local out="$dir/vite-watch.out.log"
  local err="$dir/vite-watch.err.log"

  cd "$dir"
  echo "Building $label once before watch..."
  npm run build

  rm -f "$out" "$err"
  npm run build:watch > "$out" 2> "$err" &
  local pid=$!
  sleep 2
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "$label watcher exited early, see $err" >&2
    exit 1
  fi
  echo "$pid"
}

ensure_node_modules "$WEB_DIR" "admin frontend"
ensure_node_modules "$PORTAL_DIR" "portal frontend"

echo "[1/2] Starting frontend asset watchers for local backend mode..."
WEB_WATCH_PID="$(start_watcher "$WEB_DIR" "admin frontend")"
PORTAL_WATCH_PID="$(start_watcher "$PORTAL_DIR" "portal frontend")"
trap 'kill "$WEB_WATCH_PID" "$PORTAL_WATCH_PID" 2>/dev/null || true' EXIT INT TERM

echo "[2/2] Starting backend on http://127.0.0.1:$PORT"
cd "$BACKEND_DIR"
export SERVER_HOST="0.0.0.0"
export SERVER_PORT="$PORT"
go run .
