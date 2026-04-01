#!/usr/bin/env bash

set -euo pipefail

IMAGE_TAG="${1:-}"
if [ -z "$IMAGE_TAG" ]; then
  echo "Usage: scripts/deploy-vps.sh <image-tag>" >&2
  exit 1
fi

DEPLOY_HOST="${DEPLOY_HOST:-203.0.113.10}"
DEPLOY_USER="${DEPLOY_USER:-deploy}"
REMOTE_APP_DIR="${REMOTE_APP_DIR:-/opt/apps/deployik}"

ssh "${DEPLOY_USER}@${DEPLOY_HOST}" "bash -se" <<EOF
set -euo pipefail

if [ -n "${GHCR_USERNAME:-}" ] && [ -n "${GHCR_TOKEN:-}" ]; then
  printf '%s' '${GHCR_TOKEN:-}' | docker login ghcr.io -u '${GHCR_USERNAME:-}' --password-stdin >/dev/null
fi

cd "${REMOTE_APP_DIR}"
export IMAGE_TAG="${IMAGE_TAG}"

docker compose pull app
docker compose up -d --no-build app
docker exec deployik wget -q --spider http://localhost:8080/api/health
EOF
