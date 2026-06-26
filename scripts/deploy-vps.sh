#!/usr/bin/env bash

set -euo pipefail

IMAGE_TAG="${1:-}"
if [ -z "$IMAGE_TAG" ]; then
  echo "Usage: scripts/deploy-vps.sh <image-tag>" >&2
  exit 1
fi

DEPLOY_HOST="${DEPLOY_HOST:?set DEPLOY_HOST to your server address}"
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

for attempt in \$(seq 1 30); do
  if docker exec deployik wget -q --spider http://localhost:8080/api/health; then
    exit 0
  fi

  sleep 2
done

echo "Deployik did not become healthy in time" >&2
docker logs --tail 200 deployik >&2 || true
exit 1
EOF
