#!/usr/bin/env bash
#
# Deployik self-host bootstrap
#
# Prepares a fresh Docker host to run Deployik:
#   - checks Docker + Docker Compose are available
#   - creates the shared `proxy` Docker network (if missing)
#   - creates host directories for the reverse proxy, backups, and builds
#   - creates a `.env` from `.env.example` and generates strong secrets
#
# This script is IDEMPOTENT and SAFE: it never overwrites an existing `.env`
# and never destroys data. It does NOT start Deployik — it prints the command
# for you to run once you've filled in your GitHub OAuth credentials.

set -euo pipefail

# --- Run from the repository root, regardless of where we were invoked ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

# Configurable host paths (override via env if your host differs).
NGINX_PROXY_DIR="${NGINX_PROXY_DIR:-/opt/nginx-proxy}"
BACKUPS_DIR="${BACKUPS_DIR:-/opt/backups/deployik}"
BUILDS_DIR="/tmp/deployik-builds"

echo "==> Deployik bootstrap (repo: ${REPO_ROOT})"

# --- a. Check Docker + Docker Compose exist -------------------------------
echo "==> Checking for Docker and Docker Compose..."
if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: 'docker' was not found in PATH. Install Docker first." >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "ERROR: 'docker compose' was not found. Install the Docker Compose plugin." >&2
  exit 1
fi
echo "    Docker and Docker Compose are available."

# --- b. Create the shared `proxy` Docker network (if missing) -------------
echo "==> Ensuring the shared 'proxy' Docker network exists..."
if docker network inspect proxy >/dev/null 2>&1; then
  echo "    Network 'proxy' already exists."
else
  docker network create proxy 2>/dev/null || true
  echo "    Created network 'proxy'."
fi

# --- c. Create host directories (if missing) ------------------------------
echo "==> Creating host directories (if missing)..."
mkdir -p \
  "${NGINX_PROXY_DIR}/conf.d" \
  "${NGINX_PROXY_DIR}/html" \
  "${NGINX_PROXY_DIR}/certs" \
  "${BACKUPS_DIR}" \
  "${BUILDS_DIR}"
echo "    nginx-proxy: ${NGINX_PROXY_DIR}/{conf.d,html,certs}"
echo "    backups:     ${BACKUPS_DIR}"
echo "    builds:      ${BUILDS_DIR}"

# --- d/e. Create .env and generate secrets (only if .env is absent) -------
if [ -f .env ]; then
  # e. .env already exists -> do not touch it.
  echo "==> '.env' already exists — leaving it untouched."
else
  echo "==> Creating '.env' from '.env.example'..."
  if [ ! -f .env.example ]; then
    echo "ERROR: '.env.example' not found in ${REPO_ROOT}. Cannot create '.env'." >&2
    exit 1
  fi
  cp .env.example .env

  if ! command -v openssl >/dev/null 2>&1; then
    echo "ERROR: 'openssl' is required to generate secrets but was not found." >&2
    echo "       '.env' was created from the example; set JWT_SECRET and ENCRYPTION_KEY manually." >&2
    exit 1
  fi

  JWT_SECRET="$(openssl rand -hex 32)"
  ENCRYPTION_KEY="$(openssl rand -hex 32)"

  # Replace the empty `KEY=` lines using a temp file (portable across BSD/GNU sed).
  TMP_ENV="$(mktemp)"
  sed \
    -e "s|^JWT_SECRET=.*$|JWT_SECRET=${JWT_SECRET}|" \
    -e "s|^ENCRYPTION_KEY=.*$|ENCRYPTION_KEY=${ENCRYPTION_KEY}|" \
    .env >"${TMP_ENV}"
  mv "${TMP_ENV}" .env

  echo "    Generated JWT_SECRET and ENCRYPTION_KEY in '.env'."
  echo ""
  echo "    !! You must still set these in '.env' before the first run:"
  echo "         GITHUB_CLIENT_ID"
  echo "         GITHUB_CLIENT_SECRET"
  echo "         BASE_DOMAIN          (your wildcard-DNS base domain)"
fi

# --- f. Final next steps --------------------------------------------------
echo ""
echo "==> Bootstrap complete. Next steps:"
echo "    1. Edit '.env' and set GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, and BASE_DOMAIN."
echo "    2. Start Deployik:"
echo ""
echo "         docker compose -f docker/docker-compose.yml up -d"
echo ""
