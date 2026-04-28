#!/usr/bin/env bash

set -euo pipefail

APP_DIR="${APP_DIR:-$(pwd)}"
DEPLOY_BRANCH="${DEPLOY_BRANCH:-production}"
DEPLOY_REPO_URL="${DEPLOY_REPO_URL:-}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-http://127.0.0.1:7000/health}"

normalize_healthcheck_url() {
  case "$1" in
    http://127.0.0.1:*/image|http://localhost:*/image|https://127.0.0.1:*/image|https://localhost:*/image)
      printf '%s/health\n' "${1%/image}"
      ;;
    *)
      printf '%s\n' "$1"
      ;;
  esac
}

docker_registry_ready() {
  getent hosts registry-1.docker.io >/dev/null 2>&1 || return 1
  curl -sSI --max-time 10 https://registry-1.docker.io/v2/ \
    | head -n 1 \
    | grep -Eq 'HTTP/[0-9.]+ (200|401)' || return 1
}

ensure_docker_registry_ready() {
  if docker_registry_ready; then
    return 0
  fi

  echo "[deploy] docker registry preflight failed"
  if command -v ss >/dev/null 2>&1; then
    echo "[deploy] udp sockets: $(ss -u -a -n | wc -l)"
  fi

  echo "[deploy] docker registry preflight still failing" >&2
  return 1
}

dump_diagnostics() {
  echo "[deploy] dumping diagnostics"
  docker compose ps || true
  docker compose logs --tail=200 studio || true
  if [ -n "${HEALTHCHECK_URL:-}" ]; then
    echo "[deploy] probe: $HEALTHCHECK_URL"
    curl -iS --max-time 10 "$HEALTHCHECK_URL" || true
  fi
}

HEALTHCHECK_URL="$(normalize_healthcheck_url "$HEALTHCHECK_URL")"
trap 'status=$?; echo "[deploy] failed with exit code $status" >&2; dump_diagnostics; exit $status' ERR

mkdir -p "$APP_DIR"
cd "$APP_DIR"

echo "[deploy] app dir: $APP_DIR"
echo "[deploy] target branch: $DEPLOY_BRANCH"
if [ -n "$DEPLOY_REPO_URL" ]; then
  echo "[deploy] target repo: $DEPLOY_REPO_URL"
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  if [ -z "$DEPLOY_REPO_URL" ]; then
    echo "[deploy] DEPLOY_REPO_URL is required to bootstrap a missing git repo" >&2
    exit 1
  fi

  echo "[deploy] bootstrapping git repo in $APP_DIR"
  git init
fi

CURRENT_ORIGIN_URL="$(git remote get-url origin 2>/dev/null || true)"
if [ -n "$DEPLOY_REPO_URL" ]; then
  if [ -z "$CURRENT_ORIGIN_URL" ]; then
    echo "[deploy] adding origin remote"
    git remote add origin "$DEPLOY_REPO_URL"
  elif [ "$CURRENT_ORIGIN_URL" != "$DEPLOY_REPO_URL" ]; then
    echo "[deploy] updating origin remote"
    git remote set-url origin "$DEPLOY_REPO_URL"
  fi
fi

git fetch origin "$DEPLOY_BRANCH"

git checkout -B "$DEPLOY_BRANCH"
git branch --set-upstream-to="origin/$DEPLOY_BRANCH" "$DEPLOY_BRANCH" >/dev/null 2>&1 || true
git reset --hard "origin/$DEPLOY_BRANCH"

ensure_docker_registry_ready

docker compose up -d --build

if [ -n "$HEALTHCHECK_URL" ]; then
  echo "[deploy] healthcheck: $HEALTHCHECK_URL"
  for _ in $(seq 1 30); do
    if curl -fsS "$HEALTHCHECK_URL" >/dev/null; then
      echo "[deploy] healthcheck passed"
      exit 0
    fi
    sleep 5
  done

  echo "[deploy] healthcheck failed" >&2
  exit 1
fi

echo "[deploy] finished without healthcheck"
