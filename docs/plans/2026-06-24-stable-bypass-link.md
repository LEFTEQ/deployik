# Stable Per-Project Bypass Link — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a stable, per-project, rotate-to-revoke password-bypass link (`?_dpkbypass=<token>`) so automation (the eve prospector screenshot bot) or a human can skip a project's password gate — built on the existing `_dpkauth` HMAC machinery, with no expiry.

**Architecture:** A per-project `bypass_nonce` is folded into a signed message `staticbypass:<projectID>:<env>:<nonce>` (HMAC-SHA256 with the existing `JWTSecret`). The `Check` gate handler honors a `_dpkbypass` query param by doing **one DB read of the nonce only when that param is present** (ordinary cookie traffic pays nothing). Rotating the nonce invalidates every prior link. Exposed via a new `GET /protection/bypass-link` + `POST /protection/bypass/rotate` API, surfaced in `get_protection`/`set_protection`, and a new MCP `get_bypass_link` / `rotate_bypass_link` tool.

**Tech Stack:** Go (chi router, SQLite, `crypto/hmac`), embedded SQL migrations, TypeScript MCP server (`@modelcontextprotocol/sdk`, zod).

**Repo:** `lovinka-deployik`.

**Reference design:** `../eve-ai-layer/docs/plans/2026-06-24-deployik-bypass-link-and-prospector-shot-design.md` (Piece 1). Note: the prospector screenshot fix (Piece 2) is a **separate, independent** plan in the eve-ai-layer repo and does NOT depend on this feature.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/db/migrations/031_project_bypass_nonce.sql` | schema | Create — add `bypass_nonce` column |
| `internal/db/queries_projects.go` | project queries | Add `Get/Set/ClearProjectBypassNonce` |
| `internal/auth/siteauth.go` | site-auth token crypto | Add static-bypass mint/verify/extract; refactor a shared query-param extractor |
| `internal/api/handlers/protection.go` | gate + protection mgmt | Honor `_dpkbypass` in `Check`; add `bypassURL`/`ensureBypassNonce`/`BypassLink`/`RotateBypass`; surface link in `Get`/`Update`; clear nonce on disable |
| `internal/api/router.go` | routes | Register the two new endpoints |
| `internal/auth/siteauth_test.go` | crypto tests | Add static-bypass tests |
| `internal/api/handlers/protection_test.go` | handler tests | Add nonce-query, Check-bypass, BypassLink, Rotate tests |
| `mcp/src/client/types.ts` | MCP API types | Add bypass fields + `BypassLinkResponse` |
| `mcp/src/tools/protection.ts` | MCP tools | Add `get_bypass_link` + `rotate_bypass_link`; surface link in `set_protection` |
| `mcp/src/lib/format.ts` | MCP rendering | Show bypass URLs in `renderProtection` |

---

## Task 1: Migration + DB nonce queries

**Files:**
- Create: `internal/db/migrations/031_project_bypass_nonce.sql`
- Modify: `internal/db/queries_projects.go`
- Test: `internal/api/handlers/protection_test.go` (uses the migrated test DB from `newProtectionHandler`)

- [ ] **Step 1: Write the failing test**

Append to `internal/api/handlers/protection_test.go`:

```go
func TestBypassNonce_QueryRoundTrip(t *testing.T) {
	h, project := newProtectionHandler(t)

	got, err := h.DB.GetProjectBypassNonce(project.ID)
	if err != nil || got != "" {
		t.Fatalf("fresh project nonce = %q err=%v, want empty/no-error", got, err)
	}
	if err := h.DB.SetProjectBypassNonce(project.ID, "n1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _ = h.DB.GetProjectBypassNonce(project.ID); got != "n1" {
		t.Fatalf("nonce = %q, want n1", got)
	}
	if err := h.DB.ClearProjectBypassNonce(project.ID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ = h.DB.GetProjectBypassNonce(project.ID); got != "" {
		t.Fatalf("nonce after clear = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/handlers/ -run TestBypassNonce_QueryRoundTrip`
Expected: FAIL — `h.DB.GetProjectBypassNonce` undefined (and, once compiling, "no such column: bypass_nonce" until the migration exists).

- [ ] **Step 3: Create the migration**

Create `internal/db/migrations/031_project_bypass_nonce.sql`:

```sql
-- Per-project nonce for the STABLE password-bypass link (?_dpkbypass=...).
-- NULL = no link issued yet (lazily created on first read). Rotating the value
-- revokes every previously-minted bypass link for the project.
ALTER TABLE projects ADD COLUMN bypass_nonce TEXT;
```

(The embedded migration runner `db.Migrate()` in `internal/db/migrations.go` applies un-applied `.sql` files in lexical order, tracked in `_migrations` — `031_*` runs automatically on next DB open, including in tests.)

- [ ] **Step 4: Add the queries**

Append to `internal/db/queries_projects.go` (mirror the existing `GetProjectPassword`/`SetProjectPassword`/`ClearProjectPassword` shape, but the nonce is project-level — no environment column):

```go
// GetProjectBypassNonce returns the project's stable bypass-link nonce ("" when
// none has been issued). Empty means the project cannot be bypassed.
func (db *DB) GetProjectBypassNonce(projectID string) (string, error) {
	var val sql.NullString
	err := db.QueryRow(`SELECT bypass_nonce FROM projects WHERE id = ?`, projectID).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get project bypass nonce: %w", err)
	}
	if !val.Valid {
		return "", nil
	}
	return val.String, nil
}

// SetProjectBypassNonce stores (or rotates) the project's bypass-link nonce.
func (db *DB) SetProjectBypassNonce(projectID, nonce string) error {
	_, err := db.Exec(
		`UPDATE projects SET bypass_nonce = ?, updated_at = datetime('now') WHERE id = ?`,
		nonce, projectID,
	)
	if err != nil {
		return fmt.Errorf("set project bypass nonce: %w", err)
	}
	return nil
}

// ClearProjectBypassNonce revokes the project's bypass link (used when password
// protection is disabled).
func (db *DB) ClearProjectBypassNonce(projectID string) error {
	_, err := db.Exec(
		`UPDATE projects SET bypass_nonce = NULL, updated_at = datetime('now') WHERE id = ?`,
		projectID,
	)
	if err != nil {
		return fmt.Errorf("clear project bypass nonce: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/api/handlers/ -run TestBypassNonce_QueryRoundTrip`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/031_project_bypass_nonce.sql internal/db/queries_projects.go internal/api/handlers/protection_test.go
git commit -m "feat(protection): bypass_nonce column + Get/Set/Clear queries"
```

---

## Task 2: Static-bypass token crypto

**Files:**
- Modify: `internal/auth/siteauth.go`
- Test: `internal/auth/siteauth_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/auth/siteauth_test.go`:

```go
func TestStaticBypass_RoundTrip(t *testing.T) {
	tok := MintStaticBypassToken(testSecret, "proj-1", "preview", "nonce-abc")
	if !VerifyStaticBypass(testSecret, tok, "proj-1", "preview", "nonce-abc") {
		t.Fatal("freshly-minted static bypass token failed verification")
	}
}

func TestStaticBypass_RotatedNonceRevokes(t *testing.T) {
	tok := MintStaticBypassToken(testSecret, "proj-1", "preview", "old-nonce")
	if VerifyStaticBypass(testSecret, tok, "proj-1", "preview", "new-nonce") {
		t.Fatal("token minted against old nonce must not verify against a rotated nonce")
	}
}

func TestStaticBypass_EmptyNonceRejected(t *testing.T) {
	if VerifyStaticBypass(testSecret, "anything", "proj-1", "preview", "") {
		t.Fatal("empty nonce must never verify (no link issued)")
	}
}

func TestStaticBypass_WrongProjectOrEnv(t *testing.T) {
	tok := MintStaticBypassToken(testSecret, "proj-A", "preview", "n")
	if VerifyStaticBypass(testSecret, tok, "proj-B", "preview", "n") {
		t.Fatal("wrong project must not verify")
	}
	if VerifyStaticBypass(testSecret, tok, "proj-A", "production", "n") {
		t.Fatal("wrong environment must not verify")
	}
}

func TestExtractStaticBypassToken(t *testing.T) {
	if got := ExtractStaticBypassToken("/path?a=1&_dpkbypass=deadbeef&b=2"); got != "deadbeef" {
		t.Fatalf("got %q, want deadbeef", got)
	}
	if got := ExtractStaticBypassToken("/path?_dpkauth=x.y"); got != "" {
		t.Fatalf("got %q, want empty (different param)", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/auth/ -run TestStaticBypass`
Expected: FAIL — `MintStaticBypassToken` / `VerifyStaticBypass` / `ExtractStaticBypassToken` undefined.

- [ ] **Step 3: Implement the crypto**

In `internal/auth/siteauth.go`, add the param constant near `SiteAuthBypassParam`:

```go
// SiteAuthStaticBypassParam is the query parameter carrying the STABLE
// (non-expiring, rotate-to-revoke) bypass token. Unlike _dpkauth it is bound to
// the project's bypass_nonce, so revocation is a DB nonce rotation rather than
// an expiry.
const SiteAuthStaticBypassParam = "_dpkbypass"
```

Refactor `ExtractBypassToken` to share a query extractor, and add the static
functions (append after `ExtractBypassToken`):

```go
func extractQueryParam(requestURI, param string) string {
	q := strings.IndexByte(requestURI, '?')
	if q < 0 {
		return ""
	}
	values, err := url.ParseQuery(requestURI[q+1:])
	if err != nil {
		return ""
	}
	return values.Get(param)
}

// MintStaticBypassToken returns a stable signed token authorising the site-auth
// gate for the given project + environment, bound to the project's bypass nonce.
// No expiry: rotating the nonce is what revokes it. Domain separation
// ("staticbypass:" prefix) keeps it non-interchangeable with _dpkauth + cookies.
func MintStaticBypassToken(secret, projectID, environment, nonce string) string {
	msg := fmt.Sprintf("staticbypass:%s:%s:%s", projectID, environment, nonce)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyStaticBypass returns true only when token is the valid HMAC for the
// given project + environment + nonce. An empty nonce or token always returns
// false (a project with no issued link cannot be bypassed).
func VerifyStaticBypass(secret, token, expectedProject, expectedEnv, nonce string) bool {
	if nonce == "" || token == "" {
		return false
	}
	expected := MintStaticBypassToken(secret, expectedProject, expectedEnv, nonce)
	return hmac.Equal([]byte(token), []byte(expected))
}

// ExtractStaticBypassToken pulls the _dpkbypass token out of a request URI.
func ExtractStaticBypassToken(requestURI string) string {
	return extractQueryParam(requestURI, SiteAuthStaticBypassParam)
}
```

Then simplify the existing `ExtractBypassToken` body to reuse the shared helper:

```go
// ExtractBypassToken pulls the short-lived _dpkauth token out of a request URI.
func ExtractBypassToken(requestURI string) string {
	return extractQueryParam(requestURI, SiteAuthBypassParam)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/auth/`
Expected: PASS — new static-bypass tests + all existing `_dpkauth` tests (the `ExtractBypassToken` refactor must keep them green).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/siteauth.go internal/auth/siteauth_test.go
git commit -m "feat(auth): stable static-bypass token (mint/verify/extract) bound to a project nonce"
```

---

## Task 3: `Check` gate honors `_dpkbypass`

**Files:**
- Modify: `internal/api/handlers/protection.go` (`Check`)
- Test: `internal/api/handlers/protection_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/api/handlers/protection_test.go`:

```go
func TestCheckHandler_AcceptsValidStaticBypass(t *testing.T) {
	h, project := newProtectionHandler(t)
	if err := h.DB.SetProjectBypassNonce(project.ID, "nonce-xyz"); err != nil {
		t.Fatalf("SetProjectBypassNonce: %v", err)
	}
	token := auth.MintStaticBypassToken(testBypassSecret, project.ID, "preview", "nonce-xyz")

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", project.ID)
	req.Header.Set("X-Deployik-Environment", "preview")
	req.Header.Set("X-Original-URI", "/?_dpkbypass="+token)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}
}

func TestCheckHandler_RejectsStaticBypassAfterRotation(t *testing.T) {
	h, project := newProtectionHandler(t)
	_ = h.DB.SetProjectBypassNonce(project.ID, "old")
	token := auth.MintStaticBypassToken(testBypassSecret, project.ID, "preview", "old")
	_ = h.DB.SetProjectBypassNonce(project.ID, "new") // rotate

	req := httptest.NewRequest(http.MethodGet, "/api/site-auth/check", nil)
	req.Header.Set("X-Deployik-Project", project.ID)
	req.Header.Set("X-Deployik-Environment", "preview")
	req.Header.Set("X-Original-URI", "/?_dpkbypass="+token)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 after rotation", rr.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/handlers/ -run TestCheckHandler_.*StaticBypass`
Expected: FAIL — `Check` returns 401 (it doesn't honor `_dpkbypass` yet).

- [ ] **Step 3: Implement the `Check` change**

In `internal/api/handlers/protection.go`, inside `Check`, add a second `_dpkbypass`
block immediately AFTER the existing `_dpkauth` `originalURI` block and BEFORE the
cookie lookup:

```go
	if originalURI := r.Header.Get("X-Original-URI"); originalURI != "" {
		if token := auth.ExtractStaticBypassToken(originalURI); token != "" {
			// One DB read, only on explicit bypass-link requests (rare — ordinary
			// visitors use the cookie path below and never reach this read).
			if nonce, err := h.DB.GetProjectBypassNonce(expectedProject); err == nil && nonce != "" {
				if auth.VerifyStaticBypass(h.JWTSecret, token, expectedProject, expectedEnv, nonce) {
					w.WriteHeader(http.StatusOK)
					return
				}
			}
		}
	}
```

(The existing `_dpkauth`-only `Check` tests construct the handler as
`&ProtectionHandler{JWTSecret: testBypassSecret}` with a nil `DB`. They stay green
because their `X-Original-URI` carries `_dpkauth`, so `ExtractStaticBypassToken`
returns `""` and `h.DB` is never touched.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/handlers/ -run TestCheckHandler`
Expected: PASS — new static-bypass tests + all existing `_dpkauth` Check tests.

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers/protection.go internal/api/handlers/protection_test.go
git commit -m "feat(protection): Check honors the stable _dpkbypass token (1 DB read, bypass-only)"
```

---

## Task 4: Bypass-link endpoints + auto-issue + disable-revoke

**Files:**
- Modify: `internal/api/handlers/protection.go`
- Test: `internal/api/handlers/protection_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/api/handlers/protection_test.go`:

```go
func TestBypassLink_ReturnsTokenForProtectedEnv(t *testing.T) {
	h, project := newProtectionHandler(t)
	enable := protectionRequest(t, http.MethodPut, "/projects/"+project.ID+"/protection", project.ID, map[string]any{
		"environment": "preview", "enabled": true,
	})
	h.Update(httptest.NewRecorder(), enable)

	req := protectionRequest(t, http.MethodGet, "/projects/"+project.ID+"/protection/bypass-link?environment=preview", project.ID, nil)
	rec := httptest.NewRecorder()
	h.BypassLink(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
		Param string `json:"param"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Param != auth.SiteAuthStaticBypassParam {
		t.Fatalf("param = %q, want %q", resp.Param, auth.SiteAuthStaticBypassParam)
	}
	nonce, _ := h.DB.GetProjectBypassNonce(project.ID)
	if !auth.VerifyStaticBypass(testBypassSecret, resp.Token, project.ID, "preview", nonce) {
		t.Fatal("returned token does not verify against the stored nonce")
	}
}

func TestBypassLink_ConflictWhenNotProtected(t *testing.T) {
	h, project := newProtectionHandler(t)
	req := protectionRequest(t, http.MethodGet, "/projects/"+project.ID+"/protection/bypass-link?environment=preview", project.ID, nil)
	rec := httptest.NewRecorder()
	h.BypassLink(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for unprotected env", rec.Code)
	}
}

func TestRotateBypass_RevokesOldToken(t *testing.T) {
	h, project := newProtectionHandler(t)
	enable := protectionRequest(t, http.MethodPut, "/projects/"+project.ID+"/protection", project.ID, map[string]any{
		"environment": "preview", "enabled": true,
	})
	h.Update(httptest.NewRecorder(), enable)
	nonceBefore, _ := h.DB.GetProjectBypassNonce(project.ID)
	oldToken := auth.MintStaticBypassToken(testBypassSecret, project.ID, "preview", nonceBefore)

	rot := protectionRequest(t, http.MethodPost, "/projects/"+project.ID+"/protection/bypass/rotate?environment=preview", project.ID, nil)
	rec := httptest.NewRecorder()
	h.RotateBypass(rec, rot)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	nonceAfter, _ := h.DB.GetProjectBypassNonce(project.ID)
	if nonceAfter == nonceBefore || nonceAfter == "" {
		t.Fatalf("rotate must change the nonce (before=%q after=%q)", nonceBefore, nonceAfter)
	}
	if auth.VerifyStaticBypass(testBypassSecret, oldToken, project.ID, "preview", nonceAfter) {
		t.Fatal("old token must not verify after rotation")
	}
}

func TestUpdate_DisableClearsBypassNonce(t *testing.T) {
	h, project := newProtectionHandler(t)
	h.Update(httptest.NewRecorder(), protectionRequest(t, http.MethodPut, "/x", project.ID, map[string]any{
		"environment": "preview", "enabled": true,
	}))
	// force a nonce to exist
	if _, err := h.ensureBypassNonce(project.ID); err != nil {
		t.Fatalf("ensureBypassNonce: %v", err)
	}
	h.Update(httptest.NewRecorder(), protectionRequest(t, http.MethodPut, "/x", project.ID, map[string]any{
		"environment": "preview", "enabled": false,
	}))
	if got, _ := h.DB.GetProjectBypassNonce(project.ID); got != "" {
		t.Fatalf("nonce after disable = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/handlers/ -run 'TestBypassLink|TestRotateBypass|TestUpdate_DisableClears'`
Expected: FAIL — `h.BypassLink` / `h.RotateBypass` / `h.ensureBypassNonce` undefined; disable doesn't clear the nonce.

- [ ] **Step 3: Implement the helpers + endpoints + Update/Get changes**

In `internal/api/handlers/protection.go`, add the helpers (place near the other
`ProtectionHandler` methods, e.g. after `RevealPassword`):

```go
// ensureBypassNonce returns the project's bypass nonce, lazily creating one
// (16-char base64url) on first use.
func (h *ProtectionHandler) ensureBypassNonce(projectID string) (string, error) {
	nonce, err := h.DB.GetProjectBypassNonce(projectID)
	if err != nil {
		return "", err
	}
	if nonce != "" {
		return nonce, nil
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate bypass nonce: %w", err)
	}
	nonce = base64.RawURLEncoding.EncodeToString(raw)
	if err := h.DB.SetProjectBypassNonce(projectID, nonce); err != nil {
		return "", err
	}
	return nonce, nil
}

// bypassURL builds the stable bypass link for one protected environment:
// "https://<primary-domain>/?_dpkbypass=<token>". Returns ("","",nil) when the
// env isn't protected; returns ("", token, nil) when protected but no SSL-active
// domain exists yet (the token is valid the moment a domain appears).
func (h *ProtectionHandler) bypassURL(project *db.Project, environment string) (linkURL string, token string, err error) {
	if !project.IsEnvironmentProtected(environment) {
		return "", "", nil
	}
	nonce, err := h.ensureBypassNonce(project.ID)
	if err != nil {
		return "", "", err
	}
	token = auth.MintStaticBypassToken(h.JWTSecret, project.ID, environment, nonce)

	previewInstanceID := ""
	if environment == "preview" {
		if inst, ierr := h.DB.GetDefaultPreviewInstance(project.ID); ierr == nil && inst != nil {
			previewInstanceID = inst.ID
		}
	}
	primary, derr := h.DB.GetPrimaryDomainForTarget(project.ID, environment, previewInstanceID)
	if derr != nil || primary == nil {
		return "", token, nil // token usable once a domain exists
	}
	return fmt.Sprintf("https://%s/?%s=%s", primary.DomainName, auth.SiteAuthStaticBypassParam, token), token, nil
}

// BypassLink handles GET /api/projects/{id}/protection/bypass-link?environment=
func (h *ProtectionHandler) BypassLink(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}
	environment := r.URL.Query().Get("environment")
	if environment != "preview" && environment != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}
	if !project.IsEnvironmentProtected(environment) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "environment is not password-protected"})
		return
	}
	url, token, err := h.bypassURL(project, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"environment": environment,
		"param":       auth.SiteAuthStaticBypassParam,
		"token":       token,
		"url":         url, // "" until an SSL-active domain exists
	})
}

// RotateBypass handles POST /api/projects/{id}/protection/bypass/rotate?environment=
func (h *ProtectionHandler) RotateBypass(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, claims, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}
	environment := r.URL.Query().Get("environment")
	if environment != "preview" && environment != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if err := h.DB.SetProjectBypassNonce(project.ID, base64.RawURLEncoding.EncodeToString(raw)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "protection.bypass_rotate",
		ResourceType: "project",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": environment},
	})
	url, token, err := h.bypassURL(project, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"environment": environment,
		"param":       auth.SiteAuthStaticBypassParam,
		"token":       token,
		"url":         url,
	})
}
```

Update `Get` to surface the links (replace its `writeJSON` map):

```go
	previewURL, _, _ := h.bypassURL(project, "preview")
	productionURL, _, _ := h.bypassURL(project, "production")
	writeJSON(w, http.StatusOK, map[string]any{
		"preview_enabled":       project.PreviewPassword != "",
		"production_enabled":    project.ProductionPassword != "",
		"preview_bypass_url":    previewURL,    // "" when unprotected / no domain yet
		"production_bypass_url": productionURL,
	})
```

In `Update`, the **enabled** branch: surface the link in the response (reload the
project so `IsEnvironmentProtected` reflects the write). Replace that branch's
final `writeJSON` with:

```go
		// Enrich the success response with the bypass link (best-effort: a build
		// failure here just yields an empty url — protection itself succeeded).
		bypassLink := ""
		if updated, gerr := h.DB.GetProject(project.ID); gerr == nil && updated != nil {
			bypassLink, _, _ = h.bypassURL(updated, req.Environment)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"environment": req.Environment,
			"enabled":     true,
			"password":    plaintext,
			"bypass_url":  bypassLink,
		})
```

In `Update`, the **disabled** branch: revoke the bypass link by clearing the
nonce, right after the `ClearProjectPassword` success and before
`regenerateNginxForEnvironment`:

```go
		if err := h.DB.ClearProjectBypassNonce(project.ID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to clear bypass link"})
			return
		}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/handlers/ -run 'TestBypassLink|TestRotateBypass|TestUpdate_DisableClears'`
Expected: PASS.

- [ ] **Step 5: Run the whole handlers package to catch regressions**

Run: `go test ./internal/api/handlers/`
Expected: PASS — including the pre-existing `TestUpdate_*` / `TestCheckHandler_*` tests (the `Get` response shape changed from `map[string]bool` to `map[string]any`; verify no existing test asserts the old strict shape — if one does, update it to read `preview_enabled` from a `map[string]any`/struct).

- [ ] **Step 6: Commit**

```bash
git add internal/api/handlers/protection.go internal/api/handlers/protection_test.go
git commit -m "feat(protection): bypass-link + rotate endpoints; surface link in get/set; revoke on disable"
```

---

## Task 5: Register the routes

**Files:**
- Modify: `internal/api/router.go`

- [ ] **Step 1: Add the routes**

In `internal/api/router.go`, in the `// Password protection` block (just after the
`protection/regenerate` line), add:

```go
			r.Get("/projects/{id}/protection/bypass-link", protectionHandler.BypassLink)
			r.With(mutationLimiter.Middleware("protection_bypass_rotate")).Post("/projects/{id}/protection/bypass/rotate", protectionHandler.RotateBypass)
```

- [ ] **Step 2: Build to verify wiring**

Run: `go build ./...`
Expected: SUCCESS (no unused/undefined symbols).

- [ ] **Step 3: Commit**

```bash
git add internal/api/router.go
git commit -m "feat(api): register protection bypass-link + rotate routes"
```

---

## Task 6: MCP — types, tools, rendering

**Files:**
- Modify: `mcp/src/client/types.ts`
- Modify: `mcp/src/tools/protection.ts`
- Modify: `mcp/src/lib/format.ts`

- [ ] **Step 1: Extend the API types**

In `mcp/src/client/types.ts`, update the two protection interfaces and add a new one:

```ts
export interface ProtectionStatus {
  preview_enabled: boolean;
  production_enabled: boolean;
  preview_bypass_url?: string;
  production_bypass_url?: string;
}

export interface ProtectionUpdateResponse {
  environment: string;
  enabled: boolean;
  password?: string;
  bypass_url?: string;
}

export interface BypassLinkResponse {
  environment: string;
  param: string;
  token: string;
  url: string; // "" until an SSL-active domain exists
}
```

- [ ] **Step 2: Show the bypass URLs in `renderProtection`**

In `mcp/src/lib/format.ts`, replace `renderProtection` with:

```ts
export function renderProtection(status: ProtectionStatus): string {
  const lines = [
    `  Preview:    ${status.preview_enabled ? "enabled" : "disabled"}`,
    `  Production: ${status.production_enabled ? "enabled" : "disabled"}`,
  ];
  if (status.preview_bypass_url) lines.push(`  Preview bypass:    ${status.preview_bypass_url}`);
  if (status.production_bypass_url) lines.push(`  Production bypass: ${status.production_bypass_url}`);
  return lines.join("\n");
}
```

- [ ] **Step 3: Add the MCP tools + surface the link in `set_protection`**

In `mcp/src/tools/protection.ts`:

Update the type import to include `BypassLinkResponse`:

```ts
import type { ProtectionStatus, ProtectionUpdateResponse, BypassLinkResponse } from "../client/types.js";
```

In the `set_protection` handler, append the bypass line to the result text (replace the `return` of that handler):

```ts
      const passwordLine = res.password ? `\nPassword (shown once): ${res.password}` : "";
      const bypassLine = res.bypass_url ? `\nBypass link: ${res.bypass_url}` : "";
      return {
        text: `Protection ${res.enabled ? "enabled" : "disabled"} on ${args.environment} of ${project.name}.${passwordLine}${bypassLine}`,
        data: res,
      };
```

Add two new tools inside `registerProtectionTools` (after `regenerate_protection_password`):

```ts
  registerTool(server, ctx, {
    name: "get_bypass_link",
    description:
      "Get a STABLE password-bypass link for a protected environment. Append it to the site URL " +
      "(it already includes ?_dpkbypass=...) to skip the password gate — for screenshots or a quick " +
      "preview. Treat it as a credential; rotate_bypass_link revokes it.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
    },
    annotations: { readOnlyHint: true },
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const res = await ctx.client.request<BypassLinkResponse>(
        `/projects/${project.id}/protection/bypass-link?environment=${args.environment}`,
      );
      const line = res.url
        ? `Bypass link for ${args.environment} of ${project.name}:\n${res.url}`
        : `Bypass token for ${args.environment} of ${project.name} (no SSL-active domain yet):\n?${res.param}=${res.token}`;
      return { text: line, data: res };
    },
  });

  registerTool(server, ctx, {
    name: "rotate_bypass_link",
    description:
      "Rotate (revoke) the bypass link for an environment — every previously-shared link stops working.",
    inputSchema: {
      project_id: z.string().optional(),
      project: z.string().optional(),
      environment: ENV,
      confirm: z.boolean().optional(),
      confirm_name: z.string().optional(),
    },
    annotations: { destructiveHint: true },
    audit: true,
    handler: async (args) => {
      const { project } = await resolveProject(ctx, args);
      const safety = checkSafety(
        {
          toolName: "rotate_bypass_link",
          tier: "destructive",
          expectedName: project.name,
          impact: { project: project.name, environment: args.environment, note: "All existing bypass links stop working." },
        },
        { confirm: args.confirm, confirm_name: args.confirm_name },
      );
      if (!safety.proceed) return { text: renderDryRun(safety.dryRun) };
      const res = await ctx.client.request<BypassLinkResponse>(
        `/projects/${project.id}/protection/bypass/rotate?environment=${args.environment}`,
        { method: "POST" },
      );
      return {
        text: `Rotated bypass link for ${args.environment} of ${project.name}:\n${res.url || `?${res.param}=${res.token}`}`,
        data: res,
      };
    },
  });
```

(`registerTool`, `resolveProject`, `checkSafety`, `renderDryRun`, `ENV` are already imported/defined in this file.)

- [ ] **Step 4: Build + test the MCP package**

Run (from `mcp/`): `npm run build && npm test`
(If the repo uses a different script, check `mcp/package.json` — typically `tsc` build + a test runner.)
Expected: SUCCESS — TypeScript compiles (the new types/tools are consistent) and existing MCP tests pass.

- [ ] **Step 5: Commit**

```bash
git add mcp/src/client/types.ts mcp/src/tools/protection.ts mcp/src/lib/format.ts
git commit -m "feat(mcp): get_bypass_link + rotate_bypass_link tools; surface bypass link in protection"
```

---

## Task 7: Full gate + manual verification

- [ ] **Step 1: Full Go test + vet + build**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS across the repo.

- [ ] **Step 2: Manual end-to-end (local or staging)**

On a running Deployik with a protected preview project:
1. `curl -H "Authorization: Bearer $DEPLOYIK_TOKEN" "$BASE/api/projects/<id>/protection/bypass-link?environment=preview"` → JSON with `url` + `token`.
2. Open the `url` in a browser (or `curl -i`) → the **real site**, not the gate (HTTP 200, not the auth.html 401).
3. `curl -X POST -H "Authorization: Bearer $DEPLOYIK_TOKEN" "$BASE/api/projects/<id>/protection/bypass/rotate?environment=preview"` → new token.
4. Re-open the OLD url → gate returns (401); the new url → real site. Confirms rotation revokes.

- [ ] **Step 3: Deploy note**

This adds a DB migration (`031`) and is GitOps-deployed like the rest of the platform. No new secret/env var is required (signing reuses `JWTSecret`). The migration is additive and backward-compatible (`bypass_nonce` defaults NULL; existing protected sites simply have no link until one is first requested).

---

## Self-Review Notes

- **Spec coverage (Piece 1):** stable per-project link ✅ (Tasks 2,4), `_dpkbypass` honored at gate ✅ (Task 3), 1-DB-read-only-on-bypass ✅ (Task 3), rotate-to-revoke ✅ (Tasks 2,4), API + MCP + auto-issue-on-protect ✅ (Tasks 4,6 — auto-issue is lazy via `ensureBypassNonce`, exercised on the first `Get`/`set_protection`/`bypass-link` call), revoke-on-disable ✅ (Task 4), surfaced in get/set_protection ✅ (Tasks 4,6).
- **Type consistency:** `bypass_nonce` column ↔ `Get/Set/ClearProjectBypassNonce` ↔ `ensureBypassNonce` ↔ `MintStaticBypassToken(secret, projectID, environment, nonce)` ↔ `VerifyStaticBypass(secret, token, project, env, nonce)` — argument order matches across crypto, `Check`, `bypassURL`, and all tests. JSON keys `token`/`param`/`url`/`bypass_url`/`preview_bypass_url`/`production_bypass_url` match between Go handlers and the MCP TS types. ✅
- **No silent swallows:** disable-revoke surfaces its error (500). The only ignored errors are best-effort link *enrichment* on already-successful responses (`Get`, `Update`-enable) where a failure correctly degrades to an empty URL — commented as such. The product endpoint (`BypassLink`) surfaces errors as 500. ✅
- **Placeholder scan:** every code step shows complete code; commands have expected output. ✅
