#!/usr/bin/env bash
set -euo pipefail

PORT="${1:-7000}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$REPO_ROOT/backend"

cd "$BACKEND_DIR"
export SERVER_HOST="0.0.0.0"
export SERVER_PORT="$PORT"
echo "Starting backend on http://127.0.0.1:$PORT"
go run .
