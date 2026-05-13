.PHONY: dev-api dev-web dev-seed build-web build run docker-build clean mcp-install mcp-build mcp-dev mcp-inspect mcp-typecheck mcp-publish

# Development
# Sources .env when present (so GITHUB_CLIENT_ID/SECRET etc. are picked up) and
# always forces DEV_MODE=true regardless of what's in .env — otherwise a missing
# DEV_MODE line would silently drop the dev-login endpoint and mock GitHub data.
dev-api:
	@bash -c 'if [ -f .env ]; then set -a && . ./.env && set +a; fi; DEV_MODE=true go run ./cmd/server/'

dev-web:
	cd web && bun run dev

dev-seed:
	./scripts/seed-dev.sh

# Build frontend
build-web:
	cd web && bun run build

# Build Go binary with embedded SPA
build: build-web
	rm -rf cmd/server/web_dist
	cp -r web/dist cmd/server/web_dist
	go build -o bin/deployik ./cmd/server/

# Run production binary
run: build
	./bin/deployik

# Docker
docker-build:
	docker build -f docker/Dockerfile -t deployik:latest .

docker-run:
	docker run --rm -p 8080:8080 \
		-e JWT_SECRET=dev-secret \
		-e ENCRYPTION_KEY=dev-encryption-key-32chars!! \
		-e DEV_MODE=true \
		deployik:latest

# MCP server (mcp/ package — @lovinka/deployik-mcp)
mcp-install:
	cd mcp && bun install

mcp-build:
	cd mcp && bun run build

mcp-dev:
	cd mcp && bun run dev

mcp-inspect:
	cd mcp && bun run inspect

mcp-typecheck:
	cd mcp && bun run typecheck

mcp-publish:
	cd mcp && bun run build && npm publish --access public

# Clean
clean:
	rm -rf bin/ cmd/server/web_dist web/dist web/node_modules mcp/dist mcp/node_modules
