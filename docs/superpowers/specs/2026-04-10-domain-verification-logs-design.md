# Real-time Domain Verification Logs

**Date:** 2026-04-10
**Status:** Approved

## Problem

When a user clicks "Verify" on a custom domain, the UI shows a spinner and nothing else for 10-30 seconds. There is no visibility into what the backend is doing (DNS lookup, SSL cert request, nginx config, reload). On failure, the user gets a single toast message with no context about which step failed or why.

## Solution

Add a real-time log stream to domain verification, displayed as an inline expansion below the domain row. Uses the existing WebSocket hub infrastructure to stream structured events from each provisioning step.

## Decisions

| Question | Choice | Rationale |
|----------|--------|-----------|
| Feedback type | Full log stream | User manages VPS directly, wants maximum visibility |
| Log placement | Inline row expansion | Stays in context, no navigation away from settings |
| Streaming mechanism | WebSocket via existing hub | Consistent with build log streaming, reuses `ws.Hub` |
| Hub changes | None -- use `domain:{id}` topic key | Hub already supports arbitrary string topics |
| After completion | Auto-minimize to summary line | "Verified in 14s -- 4 steps completed", expandable back to full logs |

## Backend Changes

### 1. Async Verify Endpoint

`POST /api/projects/{id}/domains/{did}/verify` becomes async:

- Validates project/domain ownership (unchanged)
- Kicks off provisioning in a background goroutine
- Returns immediately: `{"status": "verifying", "domain_id": "<id>"}`
- The goroutine publishes log events to `ws.Hub` under topic `domain:{domainID}`
- On completion, updates domain DNS/SSL status in DB (unchanged)
- Audit logging happens at the end of the goroutine (unchanged)

Concurrent verification guard: only one domain can verify at a time per project. If a verification is already running, return `409 Conflict` with `{"error": "A domain verification is already in progress for this project"}`. The frontend disables all Verify buttons for the project while one is running.

### 2. Log Event Structure

Each event published to the hub is a JSON object:

```json
{
  "step": "dns" | "ssl" | "nginx" | "done",
  "status": "running" | "success" | "error",
  "content": "Human-readable log line",
  "line_number": 1
}
```

The `done` event includes additional fields:

```json
{
  "step": "done",
  "status": "success" | "error",
  "content": "Domain verified and live",
  "line_number": 8,
  "duration_ms": 14200,
  "dns_verified": true,
  "ssl_status": "active"
}
```

### 3. Provisioning Step Events

**DNS Check** (per variant domain from `ResolveVariantPlan`):
```
{step: "dns", status: "running",  content: "Checking DNS for acme.io...",       line: 1}
{step: "dns", status: "success",  content: "acme.io → 203.0.113.10",          line: 2}
{step: "dns", status: "running",  content: "Checking DNS for www.acme.io...",   line: 3}
{step: "dns", status: "success",  content: "www.acme.io → 203.0.113.10",      line: 4}
```

On DNS failure, the error event fires and `done` follows immediately (no SSL/nginx attempted):
```
{step: "dns", status: "error",    content: "www.acme.io → cname.vercel-dns.com (expected 203.0.113.10)", line: 3}
{step: "done", status: "error",   content: "DNS verification failed. Fix DNS records and retry.", ...}
```

**SSL Certificate:**
```
{step: "ssl", status: "running",  content: "Running certbot for acme.io, www.acme.io...", line: 5}
{step: "ssl", status: "running",  content: "Requesting certificate from Let's Encrypt...",          line: 6}
{step: "ssl", status: "success",  content: "SSL certificate issued successfully",                   line: 7}
```

**Nginx Config:**
```
{step: "nginx", status: "running", content: "Writing nginx config for acme.io...",  line: 8}
{step: "nginx", status: "running", content: "Testing nginx configuration...",             line: 9}
{step: "nginx", status: "success", content: "Nginx reloaded successfully",                line: 10}
```

**Done:**
```
{step: "done", status: "success", content: "Domain verified and live", line: 11, duration_ms: 14200, dns_verified: true, ssl_status: "active"}
```

### 4. Logger Callback Pattern

`ProvisionDomain` and `VerifyDNS` accept an optional event emitter callback:

```go
type ProvisionLogger func(step, status, content string)
```

The verify handler creates a logger that wraps `hub.Publish(topic, event)` with auto-incrementing line numbers. Functions that don't receive a logger (e.g. called from deployment pipeline reconciliation) continue to work silently.

### 5. WebSocket Endpoint

New route: `GET /ws/domains/{did}/logs`

Implementation mirrors `ws/logs.go` (the deployment logs handler):
- Cookie or Bearer auth
- Origin allowlist check
- Subscribes to hub topic `domain:{domainID}`
- Forwards events to the WebSocket connection
- Disconnects when client closes or `done` event is received

Add to `api/router.go` in the protected group alongside the existing deployment WS route.

### 6. Concurrency

One verification at a time per project. Track in-flight verifications with a `sync.Map[projectID]domainID` in the handler struct. Clear on completion. Return 409 if a verification is already running for the project.

## Frontend Changes

### 1. New Hook: `useDomainVerification.ts`

Similar pattern to `useBuildLogs.ts`:

```typescript
interface DomainLogEvent {
  step: "dns" | "ssl" | "nginx" | "done";
  status: "running" | "success" | "error";
  content: string;
  line_number: number;
  duration_ms?: number;
  dns_verified?: boolean;
  ssl_status?: string;
}

type VerificationState = "idle" | "connecting" | "verifying" | "success" | "error";

function useDomainVerification(domainId: string | null): {
  logs: DomainLogEvent[];
  state: VerificationState;
  summary: string | null;       // e.g. "Verified in 14s -- 4 steps completed"
  durationMs: number | null;
}
```

- Connects WS when `domainId` is non-null
- Accumulates log events, deduplicates by `line_number`
- On `done` event: sets final state, builds summary string, disconnects WS
- Caps at 200 lines (domain verification won't produce more, but safety bound)

### 2. Updated `ProjectSettingsDomains.tsx`

**State additions:**
- `verifyingDomainId: string | null` -- which domain is currently being verified
- `expandedLogDomainId: string | null` -- which domain's log panel is expanded (persists after completion for the summary view)

**Domain row changes:**
- When `verifyingDomainId === domain.id`: replace "Verify" button with "Verifying..." spinner label
- Below the domain row, render the inline log panel (animated expand via CSS transition or Radix Collapsible)

**Inline log panel layout:**
- Dark background (`bg-[#0a0a0a]`), monospace font, matching `BuildLog.tsx` styling
- Line numbers in muted color (non-selectable)
- Color coding: green for success, yellow/amber for running, red for error
- Pulsing dot indicator at bottom while `state === "verifying"`
- Auto-scroll to bottom as new lines arrive

**After completion:**
- Auto-minimize after ~2s delay to a summary bar:
  - Success: green tinted background, "Verified in 14s -- 4 steps completed", click to expand
  - Error: red tinted background, "DNS verification failed -- fix DNS records and retry", click to expand
- Clicking the summary bar toggles back to full log view
- Invalidate `["domains", projectId]` query to refresh DNS/SSL badges on the domain row

**Trigger flow:**
1. User clicks "Verify"
2. Call `POST .../verify` mutation
3. On `200 {status: "verifying"}`, set `verifyingDomainId` to the domain ID (this activates the WS hook)
4. Logs stream in, panel expands
5. On `done` event, auto-minimize after 2s, refresh domain list

### 3. TypeScript Types

Add to `types/api.ts`:

```typescript
interface DomainLogEvent {
  step: "dns" | "ssl" | "nginx" | "done";
  status: "running" | "success" | "error";
  content: string;
  line_number: number;
  duration_ms?: number;
  dns_verified?: boolean;
  ssl_status?: string;
}

interface VerifyDomainResponse {
  status: "verifying";
  domain_id: string;
}
```

## Files Changed

| File | Change |
|------|--------|
| `internal/api/handlers/domains.go` | Async verify, concurrency guard, logger callback wiring |
| `internal/domain/ssl.go` | `ProvisionDomain` accepts `ProvisionLogger` callback, emits ssl/nginx events |
| `internal/domain/dns.go` | `VerifyDNS` variant that accepts logger, emits per-domain check results |
| `internal/ws/domain_logs.go` | New file: `DomainLogsHandler` (mirrors deployment logs handler) |
| `internal/api/router.go` | Add `GET /ws/domains/{did}/logs` route |
| `web/src/hooks/useDomainVerification.ts` | New hook: WS connection, log accumulation, state machine |
| `web/src/pages/ProjectSettingsDomains.tsx` | Inline log panel, verify trigger flow, auto-minimize |
| `web/src/types/api.ts` | `DomainLogEvent`, `VerifyDomainResponse` interfaces |

## Not Changing

- No new database tables or migrations
- No changes to `ws/hub.go` (already supports arbitrary topics)
- No changes to the domain model struct
- Build pipeline provisioning (`build/pipeline.go`) continues calling `ProvisionDomain` without a logger (silent operation)
- Existing `useBuildLogs.ts` stays untouched (no shared abstraction -- the hooks are similar but domain verification is simpler and won't grow)
