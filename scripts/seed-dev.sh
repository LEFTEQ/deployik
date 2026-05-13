#!/usr/bin/env bash
# Seed local dev database with test data for Playwright testing.
# Prerequisites: API server running with DEV_MODE=true (make dev-api)
#
# Usage: ./scripts/seed-dev.sh

set -euo pipefail

API="${API:-http://localhost:8080/api}"
COOKIE_JAR=$(mktemp)
trap 'rm -f "$COOKIE_JAR"' EXIT

echo "==> Authenticating as test-admin..."
curl -s -c "$COOKIE_JAR" -X POST "$API/auth/dev-login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"test-admin"}' | jq .

echo ""
echo "==> Creating test project: my-nextjs-app..."
PROJECT=$(curl -s -b "$COOKIE_JAR" -X POST "$API/projects" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "my-nextjs-app",
    "github_repo": "lovinka-deployik",
    "github_owner": "LEFTEQ",
    "branch": "main",
    "framework": "nextjs",
    "package_manager": "bun",
    "build_command": "bun run build",
    "install_command": "bun install",
    "node_version": "22"
  }' 2>/dev/null)

PROJECT_ID=$(echo "$PROJECT" | jq -r '.id // empty')
if [ -z "$PROJECT_ID" ]; then
  echo "Project may already exist, trying to fetch..."
  PROJECT_ID=$(curl -s -b "$COOKIE_JAR" "$API/projects" | jq -r '.[0].id // empty')
fi

if [ -z "$PROJECT_ID" ]; then
  echo "ERROR: Could not create or find a project."
  exit 1
fi

echo "   Project ID: $PROJECT_ID"

echo ""
echo "==> Creating test project: static-site..."
curl -s -b "$COOKIE_JAR" -X POST "$API/projects" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "static-site",
    "github_repo": "infra-repo",
    "github_owner": "LEFTEQ",
    "branch": "main",
    "framework": "vite",
    "package_manager": "npm",
    "build_command": "npm run build",
    "install_command": "npm ci",
    "node_version": "22"
  }' | jq .

echo ""
echo "==> Setting env vars on $PROJECT_ID..."
curl -s -b "$COOKIE_JAR" -X PUT "$API/projects/$PROJECT_ID/env" \
  -H 'Content-Type: application/json' \
  -d '{
    "environment": "shared",
    "variables": [
      {"key": "NEXT_PUBLIC_API_URL", "value": "https://api.example.com"},
      {"key": "DATABASE_URL", "value": "postgres://localhost:5432/mydb"}
    ]
  }' | jq .

echo ""
echo "==> Done! Dev data seeded."
echo ""
echo "Open http://localhost:5173 in your browser or use Playwright MCP to test."
echo ""
echo "Playwright auth snippet (run in browser_evaluate):"
echo '  fetch("/api/auth/dev-login", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({username:"test-admin"}), credentials:"include"})'
