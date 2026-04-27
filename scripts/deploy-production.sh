#!/usr/bin/env bash

set -euo pipefail

APP_DIR="${APP_DIR:-$(pwd)}"
DEPLOY_BRANCH="${DEPLOY_BRANCH:-production}"
DEPLOY_REPO_URL="${DEPLOY_REPO_URL:-}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-http://127.0.0.1:7000/image}"

cd "$APP_DIR"

echo "[deploy] app dir: $APP_DIR"
echo "[deploy] target branch: $DEPLOY_BRANCH"
if [ -n "$DEPLOY_REPO_URL" ]; then
  echo "[deploy] target repo: $DEPLOY_REPO_URL"
fi

if [ -n "$DEPLOY_REPO_URL" ]; then
  CURRENT_ORIGIN_URL="$(git remote get-url origin 2>/dev/null || true)"
  if [ "$CURRENT_ORIGIN_URL" != "$DEPLOY_REPO_URL" ]; then
    echo "[deploy] updating origin remote"
    git remote set-url origin "$DEPLOY_REPO_URL"
  fi
fi

git fetch origin "$DEPLOY_BRANCH"

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "$CURRENT_BRANCH" != "$DEPLOY_BRANCH" ]; then
  if git show-ref --verify --quiet "refs/heads/$DEPLOY_BRANCH"; then
    git checkout "$DEPLOY_BRANCH"
  else
    git checkout -b "$DEPLOY_BRANCH" --track "origin/$DEPLOY_BRANCH"
  fi
fi

git branch --set-upstream-to="origin/$DEPLOY_BRANCH" "$DEPLOY_BRANCH" >/dev/null 2>&1 || true
git pull --ff-only origin "$DEPLOY_BRANCH"

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
