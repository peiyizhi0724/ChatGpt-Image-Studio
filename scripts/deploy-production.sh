#!/usr/bin/env bash

set -euo pipefail

APP_DIR="${APP_DIR:-$(pwd)}"
DEPLOY_BRANCH="${DEPLOY_BRANCH:-production}"
DEPLOY_REPO_URL="${DEPLOY_REPO_URL:-}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-http://127.0.0.1:7000/image}"

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
