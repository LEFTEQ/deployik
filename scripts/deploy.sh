#!/bin/bash

# =============================================================================
# Deployik - Deploy Script
# =============================================================================
# Usage:
#   ./scripts/deploy.sh              - Deploy latest image
#   ./scripts/deploy.sh abc123       - Deploy specific tag
# =============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

TAG=${1:-latest}
HOST="203.0.113.10"
USER="deploy"
APP_DIR="/opt/apps/deployik"
IMAGE="ghcr.io/lefteq/lovinka-deployik"

if [ "$1" = "--help" ] || [ "$1" = "-h" ]; then
    echo "Deployik Deploy Script"
    echo ""
    echo "Usage: ./scripts/deploy.sh [tag]"
    echo ""
    echo "Arguments:"
    echo "  tag    Docker image tag to deploy (default: latest)"
    exit 0
fi

echo -e "${BLUE}=== Deployik Deploy ===${NC}"
echo -e "Tag: ${GREEN}$TAG${NC}"
echo -e "Host: ${HOST}"
echo ""

echo -e "${YELLOW}Connecting to VPS...${NC}"

ssh ${USER}@${HOST} << EOF
    set -e
    cd ${APP_DIR}

    echo "=== Pulling image ==="
    docker pull ${IMAGE}:${TAG}

    echo "=== Restarting service ==="
    export IMAGE_TAG=${TAG}
    docker compose up -d --no-build app

    echo "=== Waiting for startup ==="
    sleep 5

    echo "=== Health check ==="
    if docker exec deployik wget -q --spider http://localhost:8080/api/health 2>/dev/null; then
        echo "Health check passed!"
    else
        echo "Health check failed!"
        exit 1
    fi

    echo "=== Status ==="
    docker compose ps
EOF

echo ""
echo -e "${GREEN}✓ Deploy successful!${NC}"
echo -e "Deployed: ${IMAGE}:${TAG}"
echo -e "URL: https://deployik.example.com"
