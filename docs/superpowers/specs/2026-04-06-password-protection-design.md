# Password Protection for Deployed Sites

**Date:** 2026-04-06
**Scope:** Full-stack -- nginx config changes, backend API endpoints, database migration, frontend UI

## Context

Deployik-deployed sites are publicly accessible by default. Users need the ability to restrict access with a password -- showing a styled "unavailable" page to visitors, with a hidden login at the bottom for authorized access. This is useful for pre-launch staging, client previews, and development environments.

## Architecture

```
Visitor → nginx
  ├── Has valid deployik_site_auth cookie? → proxy to container ✓
  └── No cookie → serve static auth page
       └── User enters password → POST /api/site-auth/verify
           ├── Correct → Set signed cookie, redirect back
           └── Wrong → Show error
```

- **nginx `auth_request`** checks a Deployik API endpoint on every request
- **Deployik API** validates a signed cookie (HMAC-SHA256 with project+env+expiry)
- **Static auth page** is served by nginx directly (not the container)
- **Per-environment**: preview and production can be independently protected

## Database

**New migration (`010_password_protection.sql`):**
```sql
ALTER TABLE projects ADD COLUMN preview_password TEXT;
ALTER TABLE projects ADD COLUMN production_password TEXT;
```

Both columns are nullable. NULL = public (not protected). When set, the value is AES-256-GCM encrypted (using the existing `crypto.Encryptor`).

## API Endpoints

### Protected (Deployik admin, JWT required)

**`GET /api/projects/{id}/protection`** -- Get protection status
- Response: `{ "preview_enabled": true, "production_enabled": false }`
- Never returns passwords

**`PUT /api/projects/{id}/protection`** -- Enable/disable protection
- Body: `{ "environment": "preview"|"production", "enabled": true }`
- When enabling: generates a random 16-char password, encrypts and stores it, returns plaintext once
- Response: `{ "environment": "preview", "enabled": true, "password": "xK9m2pQ..." }`
- When disabling: clears the password column, regenerates nginx config without auth_request
- Response: `{ "environment": "preview", "enabled": false }`

**`POST /api/projects/{id}/protection/regenerate`** -- Regenerate password
- Body: `{ "environment": "preview"|"production" }`
- Generates new password, re-encrypts, returns plaintext once
- Response: `{ "environment": "preview", "password": "newPass..." }`

### Public (no JWT)

**`POST /api/site-auth/verify`** -- Validate password and issue cookie
- Body: `{ "project_id": "...", "environment": "preview"|"production", "password": "..." }`
- On success: sets `deployik_site_auth` cookie (HttpOnly, Secure, 24h TTL, path=/)
- Cookie value: HMAC-SHA256 signed token containing `project_id:environment:expiry`
- Response: 200 `{ "ok": true }`
- On failure: 401 `{ "error": "invalid password" }`

**`GET /api/site-auth/check`** -- Validate cookie (called by nginx auth_request)
- Reads `deployik_site_auth` cookie
- Reads `X-Deployik-Project` and `X-Deployik-Environment` headers (set by nginx)
- Validates HMAC signature and expiry
- Returns 200 (pass through) or 401 (block)

## Auth Page

Static HTML file at `/opt/nginx-proxy/auth-pages/auth.html`. Served by nginx when auth_request returns 401.

Design:
- Dark background matching Deployik's theme
- Centered message: "Stránka není dostupná" / "Omlouváme se, ale tato stránka je dočasně nedostupná. Zkuste to prosím později."
- At the bottom: subtle divider, small muted "Přístup" button
- Clicking reveals a password input + submit button
- Form POSTs to `/_deployik/verify` (proxied to Deployik API)
- On success: redirects back to the original URL
- On error: shows inline error message

The HTML is self-contained (inline CSS, inline JS, no external dependencies). Generated once by Deployik and written to the auth-pages directory.

## Nginx Config Changes

When password protection is enabled for an environment, the nginx config template (`internal/domain/nginx.go`) adds:

```nginx
set $deployik_project_id "PROJECT_ID";
set $deployik_environment "preview";

location = /_deployik/auth-check {
    internal;
    proxy_pass http://deployik-app:8080/api/site-auth/check;
    proxy_set_header X-Deployik-Project $deployik_project_id;
    proxy_set_header X-Deployik-Environment $deployik_environment;
    proxy_set_header Cookie $http_cookie;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
}

error_page 401 = @auth_page;

location @auth_page {
    root /opt/nginx-proxy/auth-pages;
    try_files /auth.html =503;
}

location = /_deployik/verify {
    proxy_pass http://deployik-app:8080/api/site-auth/verify;
    proxy_set_header X-Deployik-Project $deployik_project_id;
    proxy_set_header X-Deployik-Environment $deployik_environment;
}

location / {
    auth_request /_deployik/auth-check;
    # ... existing proxy_pass to container ...
}
```

When protection is disabled, these blocks are omitted (standard config).

## Deployik UI

New section in project Settings (Build page), between Auto-Build and Danger Zone:

```
Password Protection
───────────────────────────────────────────
Control access to your deployed environments.

Preview     [● Enabled]    ******** [Copy] [Regenerate]
Production  [○ Disabled]   Not protected
```

- Toggle switch enables/disables per environment
- When first enabled: password is shown in a one-time alert dialog with copy button
- "Regenerate" generates a new password (with confirmation dialog: "This will invalidate the current password")
- Password is never shown again after the initial display or regeneration

## Files

### Create
| File | Purpose |
|------|---------|
| `internal/db/migrations/010_password_protection.sql` | Add preview_password, production_password columns |
| `internal/api/handlers/protection.go` | Protection CRUD + site-auth verify/check handlers |
| `internal/domain/auth_page.go` | Generate static auth HTML page |

### Modify
| File | Changes |
|------|---------|
| `internal/db/models.go` | Add PreviewPassword, ProductionPassword to Project |
| `internal/db/queries_projects.go` | Add Get/Set password methods |
| `internal/api/router.go` | Add protection and site-auth routes |
| `internal/domain/nginx.go` | Add auth_request blocks when protection enabled |
| `internal/domain/reconcile.go` | Pass protection status to nginx config generation |
| `internal/build/pipeline.go` | Pass protection status during domain provisioning |
| `web/src/pages/ProjectSettings.tsx` | Add Password Protection section |
| `web/src/types/api.ts` | Add ProtectionStatus type |
| `web/src/lib/api.ts` | Add protection API methods |

## Verification

- Enable protection on preview → visit preview domain → see "unavailable" page → enter password → see site
- Disable protection → site is publicly accessible again
- Regenerate password → old password stops working, new one works
- Cookie expiry → after 24h, user must re-enter password
- Protection status shows correctly in Deployik UI
