#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-start}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$REPO_ROOT/backend"
WEB_DIR="$REPO_ROOT/web"
RUNTIME_DIR="$REPO_ROOT/.runtime"
PID_FILE="$RUNTIME_DIR/studio.pid"
LOG_FILE="$RUNTIME_DIR/studio.log"
GO_BIN="$REPO_ROOT/.tools/go1.26.2/go/bin/go.exe"

if [ ! -x "$GO_BIN" ]; then
  GO_BIN="go"
fi

mkdir -p "$RUNTIME_DIR" "$REPO_ROOT/.gomodcache" "$REPO_ROOT/.gocache"
export GOMODCACHE="$REPO_ROOT/.gomodcache"
export GOCACHE="$REPO_ROOT/.gocache"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-off}"

read_server_port() {
  local cfg="$REPO_ROOT/backend/data/config.toml"
  if [ ! -f "$cfg" ]; then
    echo "7000"
    return
  fi
  awk '
    /^\[server\]$/ { in_server = 1; next }
    /^\[/ { in_server = 0 }
    in_server && $1 == "port" {
      gsub(/[^0-9]/, "", $3)
      if ($3 != "") { print $3; exit }
    }
  ' "$cfg" | head -n 1
}

kill_pid_if_running() {
  local pid="$1"
  if [ -z "$pid" ]; then
    return
  fi
  if kill -0 "$pid" >/dev/null 2>&1; then
    kill "$pid" >/dev/null 2>&1 || true
    sleep 0.3
    kill -9 "$pid" >/dev/null 2>&1 || true
  fi
}

kill_by_port() {
  local port="$1"
  if [ -z "$port" ]; then
    return
  fi
  if command -v lsof >/dev/null 2>&1; then
    local pids
    pids="$(lsof -ti "tcp:$port" 2>/dev/null || true)"
    if [ -n "$pids" ]; then
      echo "$pids" | xargs -r kill >/dev/null 2>&1 || true
      sleep 0.3
      echo "$pids" | xargs -r kill -9 >/dev/null 2>&1 || true
    fi
    return
  fi
  if command -v netstat >/dev/null 2>&1; then
    local pids
    pids="$(netstat -ano 2>/dev/null | grep ":$port " | awk '{print $NF}' | tr -d '\r' | sort -u || true)"
    if [ -n "$pids" ]; then
      echo "$pids" | xargs -r kill >/dev/null 2>&1 || true
      sleep 0.3
      echo "$pids" | xargs -r kill -9 >/dev/null 2>&1 || true
    fi
  fi
}

kill_repo_processes() {
  if ! command -v ps >/dev/null 2>&1; then
    return
  fi
  local me="$$"
  ps -eo pid=,args= | while read -r pid cmd; do
    if [ -z "$pid" ] || [ "$pid" = "$me" ]; then
      continue
    fi
    case "$cmd" in
      *"$REPO_ROOT"*)
        if echo "$cmd" | grep -Eqi "(vite|node|npm|go run|chatgpt-image-studio)"; then
          kill "$pid" >/dev/null 2>&1 || true
        fi
        ;;
    esac
  done
}

build_frontend() {
  echo "[studioctl] building frontend static assets..."
  cd "$WEB_DIR"
  if [ ! -x node_modules/.bin/vite ] || [ ! -x node_modules/.bin/esbuild ]; then
    npm ci --ignore-scripts
  fi
  npm run build
}

start_backend() {
  local port
  port="$(read_server_port)"
  if [ -z "$port" ]; then
    port="7000"
  fi

  if [ -f "$PID_FILE" ]; then
    kill_pid_if_running "$(cat "$PID_FILE" 2>/dev/null || true)"
    rm -f "$PID_FILE"
  fi

  kill_by_port "$port"
  kill_repo_processes
  build_frontend

  echo "[studioctl] starting backend on port $port..."
  cd "$BACKEND_DIR"
  nohup "$GO_BIN" run . >> "$LOG_FILE" 2>&1 &
  echo "$!" > "$PID_FILE"
  sleep 1
  if ! kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
    echo "[studioctl] failed to start backend. check log: $LOG_FILE"
    exit 1
  fi
  echo "[studioctl] started. pid=$(cat "$PID_FILE"), log=$LOG_FILE"
}

stop_backend() {
  local port
  port="$(read_server_port)"
  if [ -f "$PID_FILE" ]; then
    kill_pid_if_running "$(cat "$PID_FILE" 2>/dev/null || true)"
    rm -f "$PID_FILE"
  fi
  kill_by_port "${port:-7000}"
  kill_repo_processes
  echo "[studioctl] stopped."
}

status_backend() {
  if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
    echo "[studioctl] running. pid=$(cat "$PID_FILE"), log=$LOG_FILE"
    return
  fi
  echo "[studioctl] not running."
}

case "$ACTION" in
  start)
    start_backend
    ;;
  stop)
    stop_backend
    ;;
  restart)
    stop_backend
    start_backend
    ;;
  status)
    status_backend
    ;;
  *)
    echo "Usage: scripts/studioctl.sh {start|stop|restart|status}"
    exit 1
    ;;
esac
