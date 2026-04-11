# Real-time Domain Verification Logs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stream real-time log events during domain verification so the user sees each step (DNS check, SSL cert, nginx config) as it happens, displayed in an inline expansion below the domain row.

**Architecture:** The existing WebSocket hub (`ws.Hub`) is extended with a new topic prefix `domain:{id}`. The verify endpoint becomes async — it kicks a goroutine that publishes structured log events via the hub, while a new WS endpoint (`/ws/domains/{did}/logs`) streams them to the frontend. The frontend adds a `useDomainVerification` hook and an inline log panel in `ProjectSettingsDomains.tsx`.

**Tech Stack:** Go (chi, gorilla/websocket, ws.Hub), React 19, TanStack Query, WebSocket API

**Spec:** `docs/superpowers/specs/2026-04-10-domain-verification-logs-design.md`

---

### Task 1: Define ProvisionLogger type and wire into ProvisionDomain

**Files:**
- Modify: `internal/domain/ssl.go:75-101`

This task adds an optional logger callback to the provisioning flow. The logger emits structured events for each step. When no logger is provided (e.g. from the deploy pipeline), provisioning works silently as before.

- [ ] **Step 1: Add the ProvisionLogger type and update ProvisionDomain signature**

Add the logger type above `ProvisionDomain` and update the method to accept it. The logger is a simple callback. A nil logger is safe — all call sites wrap it in a nil check.

```go
// Add after line 43 (after ProvisionConfig struct) in ssl.go:

// ProvisionLogger emits structured log events during domain provisioning.
// step: "dns"|"ssl"|"nginx"|"done", status: "running"|"success"|"error".
type ProvisionLogger func(step, status, content string)
```

Update `ProvisionDomain` signature at line 75:

```go
func (m *Manager) ProvisionDomain(cfg ProvisionConfig, requireDNS bool, logger ProvisionLogger) error {
```

Then wrap all internal steps with logger calls. Replace the body of `ProvisionDomain` (lines 75-101):

```go
func (m *Manager) ProvisionDomain(cfg ProvisionConfig, requireDNS bool, logger ProvisionLogger) error {
	emit := func(step, status, content string) {
		if logger != nil {
			logger(step, status, content)
		}
	}

	if requireDNS {
		for _, domainName := range cfg.domainsToVerify() {
			emit("dns", "running", fmt.Sprintf("Checking DNS for %s...", domainName))
			verified, err := m.VerifyDomainDNS(domainName)
			if err != nil {
				emit("dns", "error", fmt.Sprintf("DNS lookup failed for %s: %v", domainName, err))
				return err
			}
			if !verified {
				emit("dns", "error", fmt.Sprintf("%s does not point to %s", domainName, m.VPSHost))
				return ErrDNSNotVerified
			}
			emit("dns", "success", fmt.Sprintf("%s → %s", domainName, m.VPSHost))
		}
	}

	sslDomains := cfg.requestSSLDomains()
	emit("ssl", "running", fmt.Sprintf("Requesting SSL certificate for %s...", strings.Join(sslDomains, ", ")))
	if err := m.RequestSSLCert(sslDomains...); err != nil {
		emit("ssl", "error", fmt.Sprintf("SSL certificate request failed: %v", err))
		return err
	}
	emit("ssl", "success", "SSL certificate issued successfully")

	emit("nginx", "running", fmt.Sprintf("Writing nginx config for %s...", cfg.Domain))
	if _, err := m.WriteNginxConfig(cfg); err != nil {
		emit("nginx", "error", fmt.Sprintf("Nginx config write failed: %v", err))
		return err
	}

	emit("nginx", "running", "Testing and reloading nginx...")
	if err := m.ReloadNginx(); err != nil {
		emit("nginx", "error", fmt.Sprintf("Nginx reload failed: %v", err))
		return err
	}
	emit("nginx", "success", "Nginx reloaded successfully")

	return nil
}
```

- [ ] **Step 2: Fix all existing callers to pass nil logger**

There are two callers of `ProvisionDomain`:

1. `internal/api/handlers/domains.go:233` — the Verify handler (will be changed in Task 3, pass `nil` for now):
```go
if err := h.Manager.ProvisionDomain(domain.ProvisionConfig{
    // ... existing fields ...
}, false, nil); err != nil {
```

2. `internal/build/pipeline.go` — the deploy pipeline. Search for `ProvisionDomain` in this file and add `nil` as the third argument:
```go
// Find the ProvisionDomain call and add nil:
err := mgr.ProvisionDomain(cfg, false, nil)
```

Search for any other callers:
```bash
grep -rn "ProvisionDomain" internal/
```

Also fix the `internal/domain/reconcile.go` caller if it exists (same pattern — add `nil`).

- [ ] **Step 3: Run tests**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./...
```

Expected: All existing tests pass. No behavior change — nil logger is a no-op.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/ssl.go internal/build/pipeline.go internal/api/handlers/domains.go internal/domain/reconcile.go
git commit -m "feat(domain): add ProvisionLogger callback to ProvisionDomain"
```

---

### Task 2: Create the domain logs WebSocket handler

**Files:**
- Create: `internal/ws/domain_logs.go`
- Modify: `internal/api/router.go:207-208`

Mirrors `internal/ws/logs.go` but subscribes to `domain:{id}` hub topics and authorizes via domain ownership instead of deployment.

- [ ] **Step 1: Create `internal/ws/domain_logs.go`**

```go
package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/LEFTEQ/lovinka-deployik/internal/api/middleware"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/authz"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// DomainLogsHandler handles WebSocket connections for domain verification log streaming.
func DomainLogsHandler(hub *Hub, database *db.DB, jwtSecret string, allowedOrigins []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := middleware.ExtractAccessToken(r)
		if tokenStr == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		claims, err := auth.ValidateAccessToken(jwtSecret, tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		domainID := r.PathValue("did")
		if domainID == "" {
			http.Error(w, "missing domain id", http.StatusBadRequest)
			return
		}

		// Authorize: find the domain, then check the user can access its project
		domain, err := database.GetDomainByID(domainID)
		if err != nil {
			http.Error(w, "failed to load domain", http.StatusInternalServerError)
			return
		}
		if domain == nil {
			http.Error(w, "domain not found", http.StatusNotFound)
			return
		}

		project, err := authz.LoadProject(database, claims, domain.ProjectID)
		if err != nil {
			http.Error(w, "failed to load project", http.StatusInternalServerError)
			return
		}
		if project == nil {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return middleware.OriginAllowed(r.Header.Get("Origin"), allowedOrigins)
			},
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		topic := "domain:" + domainID
		ch := hub.Subscribe(topic)
		defer hub.Unsubscribe(topic, ch)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		for {
			select {
			case line, ok := <-ch:
				if !ok {
					return
				}
				if err := conn.WriteJSON(line); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}
}
```

- [ ] **Step 2: Add `GetDomainByID` query to the database**

Check if `GetDomainByID` already exists. If not, add it to `internal/db/queries_domains.go`:

```go
// GetDomainByID returns a single domain by its ID.
func (d *DB) GetDomainByID(id string) (*Domain, error) {
	row := d.db.QueryRow("SELECT id, project_id, domain, environment, is_auto, dns_verified, ssl_status, ssl_expires_at, created_at FROM domains WHERE id = ?", id)
	var domain Domain
	err := row.Scan(&domain.ID, &domain.ProjectID, &domain.DomainName, &domain.Environment, &domain.IsAuto, &domain.DNSVerified, &domain.SSLStatus, &domain.SSLExpiresAt, &domain.CreatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &domain, nil
}
```

- [ ] **Step 3: Register the WS route in router.go**

Add after line 208 in `internal/api/router.go`:

```go
r.With(wsLimiter.Middleware("ws_domain_logs")).Get("/ws/domains/{did}/logs", ws.DomainLogsHandler(cfg.WSHub, cfg.DB, cfg.JWTSecret, cfg.AllowedOrigins))
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./...
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/ws/domain_logs.go internal/db/queries_domains.go internal/api/router.go
git commit -m "feat(ws): add domain verification log WebSocket endpoint"
```

---

### Task 3: Make the Verify handler async with hub publishing

**Files:**
- Modify: `internal/api/handlers/domains.go:18-22` (struct), `163-285` (Verify method)

The Verify handler becomes async: it validates inputs, starts a goroutine that runs the provisioning and publishes log events to the hub, and returns immediately. A `sync.Map` prevents concurrent verifications per project.

- [ ] **Step 1: Update the DomainHandler struct**

Add hub and in-flight tracking to the struct. In `internal/api/handlers/domains.go`, update the struct (around line 18):

```go
type DomainHandler struct {
	DB      *db.DB
	Manager *domain.Manager
	Hub     *ws.Hub
	Audit   *audit.Recorder
	// verifying tracks in-flight domain verifications per project to prevent concurrent runs.
	verifying sync.Map // map[projectID]domainID
}
```

Add the required imports at the top of the file:

```go
import (
	"sync"
	"time"

	// ... existing imports ...
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)
```

- [ ] **Step 2: Rewrite the Verify method**

Replace the entire `Verify` method (lines 163-285) with the async version:

```go
func (h *DomainHandler) Verify(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "did")
	projectID := chi.URLParam(r, "id")

	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	// Find the domain
	domains, _ := h.DB.ListDomains(projectID)
	var target *db.Domain
	for _, d := range domains {
		if d.ID == domainID {
			target = &d
			break
		}
	}

	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}

	// Prevent concurrent verifications per project
	if _, loaded := h.verifying.LoadOrStore(projectID, domainID); loaded {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "A domain verification is already in progress for this project",
		})
		return
	}

	claims := auth.GetClaims(r.Context())

	// Return immediately — provisioning runs in background
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "verifying",
		"domain_id": domainID,
	})

	// Run provisioning in background goroutine
	go func() {
		defer h.verifying.Delete(projectID)

		start := time.Now()
		lineNum := 0
		topic := "domain:" + domainID

		emit := func(step, status, content string) {
			lineNum++
			h.Hub.Publish(ws.LogLine{
				DeploymentID: topic,
				LineNumber:   lineNum,
				Content:      content,
				Stream:       step + ":" + status,
			})
		}

		// DNS verification
		plan := domain.ResolveVariantPlan(target.DomainName, target.Environment)
		var missing []string

		for _, hostname := range plan.AllDomains() {
			emit("dns", "running", fmt.Sprintf("Checking DNS for %s...", hostname))
			verified, err := h.Manager.VerifyDomainDNS(hostname)
			if err != nil {
				log.Printf("DNS verification error for %s: %v", hostname, err)
				emit("dns", "error", fmt.Sprintf("DNS lookup failed for %s: %v", hostname, err))
				missing = append(missing, hostname)
				continue
			}
			if !verified {
				emit("dns", "error", fmt.Sprintf("%s does not point to %s", hostname, h.Manager.VPSHost))
				missing = append(missing, hostname)
			} else {
				emit("dns", "success", fmt.Sprintf("%s → %s", hostname, h.Manager.VPSHost))
			}
		}

		if len(missing) > 0 {
			h.DB.UpdateDomainDNS(domainID, false)
			h.DB.UpdateDomainSSL(domainID, "pending", target.SSLExpiresAt)
			msg := fmt.Sprintf("Point %s to %s before verifying SSL", strings.Join(missing, ", "), h.Manager.VPSHost)
			emit("done", "error", msg)
			h.Audit.Record(audit.Entry{
				UserID: claims.UserID, Action: "domain.verify", ResourceType: "domain",
				ResourceID: domainID, ProjectID: projectID,
				Metadata: map[string]any{"domain": target.DomainName, "dns_verified": false, "ssl_status": "pending"},
			})
			return
		}

		h.DB.UpdateDomainDNS(domainID, true)

		// SSL + Nginx provisioning via ProvisionDomain with logger
		provisionLogger := func(step, status, content string) {
			emit(step, status, content)
		}

		if err := h.Manager.ProvisionDomain(domain.ProvisionConfig{
			ProjectID:     project.ID,
			ProjectName:   project.Name,
			Domain:        plan.CanonicalDomain,
			RedirectDomain: plan.RedirectDomain,
			SSLDomains:    plan.AllDomains(),
			Environment:   target.Environment,
			ContainerName: "deployik-" + project.Name + "-" + target.Environment,
		}, false, provisionLogger); err != nil {
			log.Printf("SSL cert request failed for %s: %v", target.DomainName, err)
			h.DB.UpdateDomainSSL(domainID, "error", target.SSLExpiresAt)
			durationMs := time.Since(start).Milliseconds()
			emit("done", "error", fmt.Sprintf("DNS verified but SSL/nginx provisioning failed (took %dms)", durationMs))
			h.Audit.Record(audit.Entry{
				UserID: claims.UserID, Action: "domain.verify", ResourceType: "domain",
				ResourceID: domainID, ProjectID: projectID,
				Metadata: map[string]any{"domain": target.DomainName, "dns_verified": true, "ssl_status": "error"},
			})
			return
		}

		h.DB.UpdateDomainSSL(domainID, "active", target.SSLExpiresAt)
		durationMs := time.Since(start).Milliseconds()
		emit("done", "success", fmt.Sprintf("Domain verified and live (took %dms)", durationMs))
		h.Audit.Record(audit.Entry{
			UserID: claims.UserID, Action: "domain.verify", ResourceType: "domain",
			ResourceID: domainID, ProjectID: projectID,
			Metadata: map[string]any{"domain": target.DomainName, "dns_verified": true, "ssl_status": "active"},
		})
	}()
}
```

- [ ] **Step 3: Update DomainHandler initialization in router.go**

In `internal/api/router.go`, find the `DomainHandler` initialization (around line 175) and add the Hub field:

```go
domainHandler := &handlers.DomainHandler{DB: cfg.DB, Manager: cfg.DomainManager, Hub: cfg.WSHub, Audit: auditRecorder}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./...
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/domains.go internal/api/router.go
git commit -m "feat(domain): async verify with real-time log publishing via WebSocket hub"
```

---

### Task 4: Add frontend types and `useDomainVerification` hook

**Files:**
- Modify: `web/src/types/api.ts:87-97`
- Create: `web/src/hooks/useDomainVerification.ts`
- Modify: `web/src/lib/api.ts:233-240`

- [ ] **Step 1: Add types to `web/src/types/api.ts`**

Add after the `Domain` interface (after line 97):

```typescript
export interface DomainLogEvent {
  deployment_id: string; // actually "domain:{id}" topic key
  line_number: number;
  content: string;
  stream: string; // "{step}:{status}" e.g. "dns:success", "ssl:running", "done:error"
}

export interface VerifyDomainResponse {
  status: "verifying";
  domain_id: string;
}
```

- [ ] **Step 2: Update `verifyDomain` return type in `web/src/lib/api.ts`**

Replace the `verifyDomain` method (lines 233-240):

```typescript
async verifyDomain(
  projectId: string,
  domainId: string,
): Promise<VerifyDomainResponse | { error: string }> {
  return this.request(`/projects/${projectId}/domains/${domainId}/verify`, {
    method: "POST",
  });
}
```

Add the import at the top of `api.ts`:

```typescript
import type { VerifyDomainResponse } from "@/types/api";
```

- [ ] **Step 3: Create `web/src/hooks/useDomainVerification.ts`**

```typescript
import { useEffect, useRef, useState, useCallback } from "react";
import { api } from "@/lib/api";
import type { DomainLogEvent } from "@/types/api";

export type VerificationState =
  | "idle"
  | "connecting"
  | "verifying"
  | "success"
  | "error";

interface DomainVerificationResult {
  logs: DomainLogEvent[];
  state: VerificationState;
  summary: string | null;
  durationMs: number | null;
  clearLogs: () => void;
}

export function parseStream(stream: string): { step: string; status: string } {
  const [step = "", status = ""] = stream.split(":");
  return { step, status };
}

export function useDomainVerification(
  domainId: string | null,
): DomainVerificationResult {
  const [logs, setLogs] = useState<DomainLogEvent[]>([]);
  const [state, setState] = useState<VerificationState>("idle");
  const [summary, setSummary] = useState<string | null>(null);
  const [durationMs, setDurationMs] = useState<number | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  const connect = useCallback(() => {
    if (!domainId) return;

    setState("connecting");
    const url = api.getWebSocketUrl(`/domains/${domainId}/logs`);
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => setState("verifying");
    ws.onclose = () => {};
    ws.onerror = () => setState("error");

    ws.onmessage = (event) => {
      const line: DomainLogEvent = JSON.parse(event.data);
      const { step, status } = parseStream(line.stream);

      if (step === "done") {
        // Extract duration from content like "Domain verified and live (took 14200ms)"
        const durationMatch = line.content.match(/took (\d+)ms/);
        const ms = durationMatch ? parseInt(durationMatch[1], 10) : null;
        setDurationMs(ms);

        if (status === "success") {
          const seconds = ms ? (ms / 1000).toFixed(0) : "?";
          setState("success");
          setSummary(`Verified in ${seconds}s`);
        } else {
          setState("error");
          setSummary(line.content);
        }

        // Close WS after receiving done
        ws.close();
      }

      setLogs((prev) => {
        if (prev.some((l) => l.line_number === line.line_number)) return prev;
        const next = [...prev, line];
        return next.length > 200 ? next.slice(-200) : next;
      });
    };

    return ws;
  }, [domainId]);

  useEffect(() => {
    if (!domainId) {
      return;
    }

    const ws = connect();
    return () => {
      ws?.close();
    };
  }, [connect, domainId]);

  const clearLogs = useCallback(() => {
    setLogs([]);
    setState("idle");
    setSummary(null);
    setDurationMs(null);
  }, []);

  return { logs, state, summary, durationMs, clearLogs };
}
```

- [ ] **Step 4: Run typecheck**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik/web && bunx tsc --noEmit
```

Expected: No type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/types/api.ts web/src/hooks/useDomainVerification.ts web/src/lib/api.ts
git commit -m "feat(web): add DomainLogEvent types and useDomainVerification hook"
```

---

### Task 5: Update `ProjectSettingsDomains.tsx` with inline verification log panel

**Files:**
- Modify: `web/src/pages/ProjectSettingsDomains.tsx`

This is the main UI task — adding the inline log panel with the three states (verifying, success summary, error summary).

- [ ] **Step 1: Add state and hook wiring**

Add imports and state to `ProjectSettingsDomains.tsx`. At the top, add:

```typescript
import { useState, useEffect, useRef } from "react";
import { ChevronDown, ChevronUp } from "lucide-react";
import { useDomainVerification, type VerificationState } from "@/hooks/useDomainVerification";
```

Remove `useState` from the existing react import (it's now in the new one). Keep the existing imports.

Inside the component, add after the existing state declarations (after line 38):

```typescript
const [verifyingDomainId, setVerifyingDomainId] = useState<string | null>(null);
const [expandedLogDomainId, setExpandedLogDomainId] = useState<string | null>(null);
const [minimized, setMinimized] = useState(false);
const { logs, state: verifyState, summary, clearLogs } = useDomainVerification(verifyingDomainId);
```

- [ ] **Step 2: Update the verify mutation**

Replace the `verifyMutation` (lines 66-73) to trigger the async flow:

```typescript
const verifyMutation = useMutation({
  mutationFn: (domainId: string) => api.verifyDomain(id, domainId),
  onSuccess: (_result, domainId) => {
    setVerifyingDomainId(domainId);
    setExpandedLogDomainId(domainId);
    setMinimized(false);
    clearLogs();
  },
  onError: (err) => toast.error(err.message),
});
```

- [ ] **Step 3: Add auto-minimize effect and query invalidation on completion**

Add after the verify mutation:

```typescript
// Auto-minimize after verification completes
useEffect(() => {
  if (verifyState === "success" || verifyState === "error") {
    queryClient.invalidateQueries({ queryKey: ["domains", id] });
    const timer = setTimeout(() => setMinimized(true), 2000);
    return () => clearTimeout(timer);
  }
}, [verifyState, queryClient, id]);
```

- [ ] **Step 4: Create the inline log panel component (inside the file)**

Add a `VerificationLogPanel` component before the `return` statement in `ProjectSettingsDomains`:

```typescript
function VerificationLogPanel({
  logs,
  state,
  summary,
  minimized,
  onToggleMinimize,
}: {
  logs: DomainLogEvent[];
  state: VerificationState;
  summary: string | null;
  minimized: boolean;
  onToggleMinimize: () => void;
}) {
  const containerRef = useRef<HTMLDivElement>(null);

  // Auto-scroll
  useEffect(() => {
    if (containerRef.current && !minimized) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [logs, minimized]);

  const isComplete = state === "success" || state === "error";

  if (minimized && isComplete && summary) {
    return (
      <button
        type="button"
        onClick={onToggleMinimize}
        className={cn(
          "flex w-full items-center justify-between px-4 py-2 text-xs transition-colors",
          state === "success"
            ? "bg-emerald-500/5 text-emerald-400 hover:bg-emerald-500/10"
            : "bg-red-500/5 text-red-400 hover:bg-red-500/10",
        )}
      >
        <span className="flex items-center gap-1.5">
          {state === "success" ? (
            <CheckCircle2 className="h-3 w-3" />
          ) : (
            <X className="h-3 w-3" />
          )}
          {summary}
        </span>
        <ChevronDown className="h-3 w-3 text-muted-foreground" />
      </button>
    );
  }

  return (
    <div>
      {isComplete && (
        <button
          type="button"
          onClick={onToggleMinimize}
          className={cn(
            "flex w-full items-center justify-between px-4 py-1.5 text-xs transition-colors",
            state === "success"
              ? "bg-emerald-500/5 text-emerald-400 hover:bg-emerald-500/10"
              : "bg-red-500/5 text-red-400 hover:bg-red-500/10",
          )}
        >
          <span className="flex items-center gap-1.5">
            {state === "success" ? (
              <CheckCircle2 className="h-3 w-3" />
            ) : (
              <X className="h-3 w-3" />
            )}
            {summary}
          </span>
          <ChevronUp className="h-3 w-3 text-muted-foreground" />
        </button>
      )}
      <div
        ref={containerRef}
        className="max-h-[200px] overflow-y-auto bg-zinc-950 px-4 py-3 font-mono text-xs leading-6"
      >
        {logs.length === 0 ? (
          <span className="text-zinc-500">Connecting...</span>
        ) : (
          logs.map((line) => {
            const { status } = parseStream(line.stream);
            return (
              <div
                key={line.line_number}
                className={cn(
                  "whitespace-pre-wrap break-all",
                  status === "success" && "text-emerald-400",
                  status === "error" && "text-red-400",
                  status === "running" && "text-zinc-400",
                )}
              >
                <span className="mr-2 select-none text-zinc-700">
                  {line.line_number}
                </span>
                {line.content}
              </div>
            );
          })
        )}
        {(state === "verifying" || state === "connecting") && (
          <span className="inline-block h-3 w-1.5 animate-pulse bg-zinc-400" />
        )}
      </div>
    </div>
  );
}
```

Add the imports for types and the `parseStream` utility:

```typescript
import type { DomainLogEvent } from "@/types/api";
import { type VerificationState, parseStream } from "@/hooks/useDomainVerification";
```

- [ ] **Step 5: Wire the log panel into `renderDomainRow`**

Update the `renderDomainRow` function to include the log panel. Replace the function (lines 84-160):

```typescript
function renderDomainRow(domain: Domain) {
  const ready = isDomainReady(domain);
  const isVerifying = verifyingDomainId === domain.id && (verifyState === "verifying" || verifyState === "connecting");
  const showLogPanel = expandedLogDomainId === domain.id && verifyState !== "idle";
  const allVerifyDisabled = verifyMutation.isPending || verifyingDomainId !== null;

  return (
    <div key={domain.id}>
      <div className="flex flex-col gap-4 px-4 py-3 md:flex-row md:items-center md:justify-between">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm font-medium">{domain.domain}</p>
            <Badge
              variant="outline"
              className={ENVIRONMENT_META[domain.environment].badgeClass}
            >
              {ENVIRONMENT_META[domain.environment].label}
            </Badge>
            <Badge variant={domain.is_auto ? "secondary" : "outline"}>
              {domain.is_auto ? "Auto" : "Custom"}
            </Badge>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1 rounded-full border border-white/8 px-2 py-1">
              <Link2 className="h-3 w-3" />
              DNS {domain.dns_verified ? "verified" : "pending"}
            </span>
            <span
              className={cn(
                "inline-flex items-center gap-1 rounded-full border px-2 py-1",
                domain.ssl_status === "active" &&
                  "border-emerald-400/25 text-emerald-200",
                domain.ssl_status === "pending" &&
                  "border-amber-400/25 text-amber-100",
                domain.ssl_status === "error" &&
                  "border-rose-400/25 text-rose-200",
              )}
            >
              <CheckCircle2 className="h-3 w-3" />
              SSL {domain.ssl_status}
            </span>
          </div>
        </div>

        <div className="flex flex-wrap gap-2">
          {ready ? (
            <Button asChild size="sm" variant="outline">
              <a
                href={`https://${domain.domain}`}
                target="_blank"
                rel="noopener noreferrer"
              >
                <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                Open
              </a>
            </Button>
          ) : null}
          {!domain.is_auto ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => verifyMutation.mutate(domain.id)}
              disabled={allVerifyDisabled}
            >
              {isVerifying ? (
                <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              ) : (
                <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
              )}
              {isVerifying ? "Verifying..." : "Verify"}
            </Button>
          ) : null}
        </div>
      </div>

      {showLogPanel && (
        <VerificationLogPanel
          logs={logs}
          state={verifyState}
          summary={summary}
          minimized={minimized}
          onToggleMinimize={() => setMinimized((prev) => !prev)}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 6: Run typecheck and dev build**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik/web && bunx tsc --noEmit
```

Expected: No type errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/ProjectSettingsDomains.tsx
git commit -m "feat(web): inline domain verification log panel with real-time streaming"
```

---

### Task 6: Integration test — full flow verification

**Files:** No new files — manual testing

- [ ] **Step 1: Run full backend tests**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik && go test ./...
```

- [ ] **Step 2: Run frontend typecheck and build**

```bash
cd /Users/your-github-username/Documents/Work/lovinka-deployik/web && bunx tsc --noEmit && bun run build
```

- [ ] **Step 3: Run dev servers and test manually**

```bash
# Terminal 1:
make dev-api

# Terminal 2:
make dev-web
```

Test the flow:
1. Navigate to a project's domain settings
2. Add a custom domain (or use an existing one)
3. Click "Verify"
4. Verify the log panel expands inline with streaming log lines
5. Verify log lines are color-coded (green=success, yellow=running, red=error)
6. Verify the panel auto-minimizes to a summary after completion
7. Verify clicking the summary re-expands the full log
8. Verify domain badges (DNS/SSL) update after verification completes
9. Verify the Verify button is disabled on all domains while one is verifying

- [ ] **Step 4: Commit any fixes**

If any adjustments were needed during testing, commit them:

```bash
git add -p
git commit -m "fix(domain): polish verification log panel from integration testing"
```
