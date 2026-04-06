.PHONY: dev-api dev-web dev-seed build-web build run docker-build clean

# Development
dev-api:
	DEV_MODE=true go run ./cmd/server/

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

# Clean
clean:
	rm -rf bin/ cmd/server/web_dist web/dist web/node_modules
