# Stage 1: Build frontend
FROM oven/bun:1 AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ .
RUN bun run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/web/dist cmd/server/web_dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /deployik ./cmd/server/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /deployik-backup ./cmd/backup/

# Stage 3: Minimal runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates git docker-cli
WORKDIR /app
COPY --from=go-builder /deployik /app/deployik
COPY --from=go-builder /deployik-backup /app/deployik-backup
COPY templates/ /app/templates/

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=10s \
  CMD wget -q --spider http://localhost:8080/api/health || exit 1

ENTRYPOINT ["/app/deployik"]
