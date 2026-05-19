# Deployik — API actions

Action mode for the goals in [click-paths.md](click-paths.md). Each entry has the endpoint, payload shape, safety tier, and the exact `helpers/deployik` invocation.

## Helper script

Prefer Deployik MCP tools when they are available in the current session. They already hold the configured Deployik token and are safer than shelling out.

If MCP tools are unavailable, invoke the API via `./helpers/deployik` (relative to this skill's directory) — never paste tokens into raw curl. The wrapper:

- Reads `~/.config/deployik/config` (`DEPLOYIK_BASE_URL`, `DEPLOYIK_TOKEN`)
- Sends `Authorization: Bearer $DEPLOYIK_TOKEN`
- Prints `HTTP <status>` to stderr, body to stdout
- Exits non-zero on HTTP error
- Pretty-prints JSON if `jq` is installed

Usage:

```
deployik api <METHOD> <path> [json-body]
```

If `~/.config/deployik/config` is missing and no Deployik MCP tools are available, stop and direct the user to create a token (Sidebar → **Access tokens** → **Create token**) before retrying. If MCP tools are available, do not require this file.

## Safety tiers

| Verb / target | Rule |
|---------------|------|
| `GET *` | Execute silently — read-only |
| `POST/PUT/PATCH` non-production | Print payload, ask "Do this?" — wait for affirmative reply |
| `POST/PUT/PATCH` production env | Flag `**PRODUCTION**`, ask explicit yes/no |
| `DELETE *` and `POST .../regenerate` | Require typed string confirmation matching the target name (e.g. `yes delete example.com`) |

## Resolving project id

Most endpoints need `{id}` (a ULID). When the user names a project, run:

```
deployik api GET /api/projects
```

…and find the row whose `name` matches. Use its `id`.

---

## create-project

**Goal:** [click-paths.md#create-project](click-paths.md#create-project)

**Endpoint:** `POST /api/projects`
**Tier:** Mutation — confirm before executing.

**Body shape:**

```json
{
  "name": "my-app",
  "github_repo": "my-repo",
  "github_owner": "my-org",
  "branch": "main",
  "framework": "nextjs",
  "package_manager": "auto",
  "root_directory": "",
  "output_directory": ".next",
  "build_command": "bun run build",
  "install_command": "bun install",
  "node_version": "22",
  "group_id": "(optional — dashboard group id; defaults to your personal group)",
  "organization_id": "(deprecated alias for group_id)"
}
```

**Framework values:** `nextjs`, `vite`, `astro`, `static`, `node-api`.
For a user-provided Dockerfile, use `framework: "static"` and set
`root_directory` to the folder containing the Dockerfile. There is no special
`framework: "docker"` value; Dockerfile presence is what makes Deployik build
the Dockerfile as-is.

**Invocation:**

```
deployik api POST /api/projects '{"name":"my-app","github_repo":"my-repo","github_owner":"my-org","branch":"main","framework":"nextjs","package_manager":"auto","output_directory":".next","build_command":"bun run build","install_command":"bun install","node_version":"22"}'
```

**Behavior:** creates the project, auto-domain, GitHub webhook (best-effort), and triggers an initial preview deployment.

**Groups:** the Projects dashboard uses dashboard groups. Call `GET /api/groups`
to list available group ids. To create into a specific dashboard tab, pass that
group's id as `group_id` (the API still accepts the legacy field name
`organization_id` for backward compatibility).

---

## dockerfile-app

**Goal:** [click-paths.md#dockerfile-app](click-paths.md#dockerfile-app)

**Endpoint:** `POST /api/projects`, then optional `PATCH /api/projects/{id}` for runtime volume settings if they were not included at create time.
**Tier:** Mutation — confirm before executing.

**Deployik Dockerfile rule:** if a `Dockerfile` exists at repo root, Deployik uses it as-is with repo root as context. If `root_directory` is set and that folder contains a `Dockerfile`, Deployik uses that Dockerfile with `root_directory` as context. Do not invent `framework: "docker"`; use `framework: "static"` as the neutral preset.

**Body shape for a Go / Dockerfile app in a subdirectory:**

```json
{
  "name": "fleet",
  "github_repo": "QTTa",
  "github_owner": "LEFTEQ",
  "branch": "main",
  "framework": "static",
  "package_manager": "auto",
  "root_directory": "apps/fleet",
  "output_directory": "",
  "build_command": "",
  "install_command": "",
  "node_version": "22",
  "port": 8080,
  "data_volume_enabled": true,
  "data_mount_path": "/data"
}
```

**Invocation:**

```
deployik api POST /api/projects '{"name":"fleet","github_repo":"QTTa","github_owner":"LEFTEQ","branch":"main","framework":"static","package_manager":"auto","root_directory":"apps/fleet","output_directory":"","build_command":"","install_command":"","node_version":"22","port":8080,"data_volume_enabled":true,"data_mount_path":"/data"}'
```

**When the repo inspector misses the app:** Dockerfile-only or Go-only folders may not appear in the JS-oriented repo inspector because they have no `package.json`. That is not evidence Deployik cannot run them. If the user knows the folder, create the project directly with `root_directory` and `framework: "static"`.

**After creation:** Deployik triggers an initial preview deployment automatically. Use `GET /api/projects/{id}/deployments` or the MCP `list_deployments` / `deploy_project(wait:true)` flow to watch it.

**Common failure:** Docker cannot copy files outside its build context. If the Dockerfile says `COPY go.sum .` or `COPY internal ./internal`, those paths must exist inside `root_directory` when `root_directory` is used.

---

## custom-domain

**Goal:** [click-paths.md#custom-domain](click-paths.md#custom-domain)

**Endpoints:**
- `POST /api/projects/{id}/domains` — adds the row
- `POST /api/projects/{id}/domains/{did}/verify` — runs DNS check + cert provisioning (returns 202; final result streams over WebSocket but the synchronous response confirms acceptance)
- `PATCH /api/projects/{id}/domains/{did}` `{is_primary: true}` — optional, mark as primary

**Tier:** Mutation — confirm before each step.

**Body shape (POST):**

```json
{ "domain": "example.com", "environment": "production" }
```

**Invocation:**

```
deployik api POST /api/projects/{id}/domains '{"domain":"example.com","environment":"production"}'
deployik api POST /api/projects/{id}/domains/{did}/verify
```

**Critical reminder for the user:** the DNS A-record at the registrar must point to the VPS IP **before** Verify, otherwise it fails. Tell them the click-paths recipe explains where to add it.

---

## env-vars

**Goal:** [click-paths.md#env-vars](click-paths.md#env-vars)

**Endpoints:**
- `POST /api/projects/{id}/env` — single upsert (env var)
- `POST /api/projects/{id}/secrets` — single upsert (secret)
- `DELETE /api/projects/{id}/env/{key}?environment=preview|production|shared` — destructive
- `PUT /api/projects/{id}/env` — bulk replace (don't use unless the user says "replace all")

**Tier:** Mutation — confirm. **Production environment writes get extra production-flag confirmation.**

**Body shape (single upsert):**

```json
{ "key": "STRIPE_PUBLIC", "value": "pk_test_xxx", "environment": "preview" }
```

**Environment values:** `shared` (applies to both), `preview`, or `production`. If the user says "preview", use `preview`; if "production" or "live", use `production`. If unsure, ask.

**Invocations:**

```
deployik api POST /api/projects/{id}/env '{"key":"STRIPE_PUBLIC","value":"pk_test_xxx","environment":"preview"}'
deployik api POST /api/projects/{id}/secrets '{"key":"DATABASE_URL","value":"postgres://...","environment":"production"}'
deployik api DELETE /api/projects/{id}/env/STRIPE_PUBLIC?environment=preview
```

**Constraint:** secrets refuse `NEXT_PUBLIC_*` keys (they need to be in the variables store to bake into the build). Surface that error verbatim if the API returns it.

**Heads-up:** changing a variable does NOT redeploy. After applying, ask the user if they want you to trigger a redeploy via the [#rollback](#rollback) endpoint with the latest commit sha.

---

## auto-deploy

**Goal:** [click-paths.md#auto-deploy](click-paths.md#auto-deploy)

**Endpoint:** `PUT /api/projects/{id}/auto-build`
**Tier:** Mutation — confirm.

**Body shape:**

```json
{
  "enabled": true,
  "production_branch": "main",
  "preview_branches": "*"
}
```

**Invocation:**

```
deployik api PUT /api/projects/{id}/auto-build '{"enabled":true,"production_branch":"main","preview_branches":"*"}'
```

**Side effect:** Deployik creates a webhook on the GitHub repo. If the user hasn't granted the `admin:repo_hook` scope to the OAuth app, this errors — ask them to sign out and back in in the dashboard, then retry.

To disable: `deployik api DELETE /api/projects/{id}/auto-build` (deletes config + webhook). Treat as destructive — typed confirmation.

---

## password-protection

**Goal:** [click-paths.md#password-protection](click-paths.md#password-protection)

**Endpoints:**
- `PUT /api/projects/{id}/protection` — enable/disable per environment
- `POST /api/projects/{id}/protection/regenerate` — new password (destructive — require typed confirmation, the old password stops working at this point)

**Tier:** PUT = mutation/confirm. Regenerate = destructive/typed-confirm.

**Body shapes:**

```json
{ "environment": "preview",     "enabled": true }
{ "environment": "production",  "enabled": false }
```

**Invocations:**

```
deployik api PUT  /api/projects/{id}/protection '{"environment":"preview","enabled":true}'
deployik api POST /api/projects/{id}/protection/regenerate '{"environment":"preview"}'
```

**Response on enable / regenerate:** the JSON includes the plaintext password under `password`. Surface it to the user with a warning: this is the only time the API will return it.

---

## contact-form-email

**Goal:** [click-paths.md#contact-form-email](click-paths.md#contact-form-email)

**Endpoints:**
- `GET /api/projects/{id}/email` — fetch current settings + the **AI install prompt** (auto-generated server-side from the project's framework, package manager, root directory, and reCAPTCHA site key)
- `PUT /api/projects/{id}/email` — save SMTP host/port/security/user, From address + name, contact recipients, reCAPTCHA site key, score threshold; the SMTP password and reCAPTCHA secret are persisted as encrypted secrets
- `POST /api/projects/{id}/email/test-smtp` — send a test email through the saved SMTP credentials

**Tier:** PUT = mutation/confirm. test-smtp = mutation/confirm (it actually sends an email — usually fine, but flag it explicitly because the user might be configuring against a live mailbox). GET = read silent.

**Body shape (PUT):**

```json
{
  "provider": "webglobe",
  "smtp_host": "smtp.webglobe.cz",
  "smtp_port": 587,
  "smtp_security": "starttls",
  "smtp_user": "noreply@example.com",
  "smtp_password": "<paste, only sent on save>",
  "email_from": "noreply@example.com",
  "email_from_name": "My Site",
  "contact_email_to": "owner@example.com",
  "recaptcha_site_key": "6Lc...public",
  "recaptcha_secret": "6Lc...secret",
  "recaptcha_mode": "v3",
  "recaptcha_score_threshold": 0.5
}
```

**Provider values:** `webglobe` (defaults derived from the project's primary production domain) or `smtp` (custom — user supplies all fields). Use `webglobe` unless the user says otherwise.

**Security values:** `starttls` (port 587), `tls` (port 465), `none` (only for explicit testing — never recommend it).

**Invocations:**

```
deployik api GET  /api/projects/{id}/email
deployik api PUT  /api/projects/{id}/email '{"provider":"webglobe","smtp_host":"smtp.webglobe.cz","smtp_port":587,"smtp_security":"starttls","smtp_user":"noreply@example.com","smtp_password":"<...>","email_from":"noreply@example.com","email_from_name":"My Site","contact_email_to":"owner@example.com","recaptcha_site_key":"<site>","recaptcha_secret":"<secret>","recaptcha_mode":"v3","recaptcha_score_threshold":0.5}'
deployik api POST /api/projects/{id}/email/test-smtp
```

**Response from GET:** includes the field `ai_install_prompt` (or similar — confirm the exact key by reading the response). This is a fully-formed prompt the user can paste into any AI coding assistant to install the contact-form code into their site. **When the user asks for the AI prompt, surface this string verbatim** — do not paraphrase or shorten it.

**Workflow when guiding action mode for this recipe:**
1. Ask the user for the four SMTP values from the Webglobe portal and the two reCAPTCHA keys from Google. Offer to walk them through Parts 1 and 2 of the click-paths recipe if they don't have them yet.
2. Print the PUT payload (with **password and secret masked** as `***` in the preview) and ask "Do this?"
3. On confirm, send the PUT.
4. Ask if they want to verify by sending a test email; if yes, run POST `/email/test-smtp`.
5. If the test succeeds, fetch GET `/email` and surface the AI install prompt for Part 4 of the recipe.
6. If the test fails, surface the error from `payload.settings.last_test_error` verbatim and offer to walk through troubleshooting.

**Sensitive values:** `smtp_password` and `recaptcha_secret` are the only truly sensitive fields. Treat them like passwords. If the user pastes them in chat, complete the action then suggest rotating them.

---

## attach-postgres

**Goal:** [click-paths.md#attach-postgres](click-paths.md#attach-postgres)

**Endpoints:**
- `GET /api/projects/{id}/services` — list services attached to the project
- `POST /api/projects/{id}/services` — attach a service (v1: postgres)
- `DELETE /api/projects/{id}/services/{env}` — **destructive** — detach + drop volume
- `GET /api/projects/{id}/services/{env}/credentials` — reveal password + DSN + SSH tunnel command (audited)
- `POST /api/projects/{id}/services/{env}/regenerate-password` — rotate password (container keeps old password until next deploy/restart)
- `POST /api/projects/{id}/services/{env}/restart` — recreate container, preserve data
- `POST /api/projects/{id}/services/{env}/reset` — **destructive** — wipe volume, requires `{"confirm":"<project>-<env>"}` body

**Tier:**
- `GET` list — silent.
- `POST` attach — mutation, confirm.
- `POST` restart, regenerate-password — mutation, confirm. **Production gets the production flag.**
- `DELETE` detach and `POST` reset — typed-string confirm matching `<project>-<env>` even in preview. Do **not** run these as a shortcut for "start fresh" — there is no undo.

**Body shapes:**

```json
// Attach
{ "environment": "preview", "type": "postgres" }

// Reset (server requires the typed string)
{ "confirm": "<project_name>-<environment>" }
```

**Invocations:**

```
deployik api GET  /api/projects/{id}/services
deployik api POST /api/projects/{id}/services '{"environment":"preview","type":"postgres"}'
deployik api GET  /api/projects/{id}/services/preview/credentials
deployik api POST /api/projects/{id}/services/preview/restart
deployik api POST /api/projects/{id}/services/preview/regenerate-password
deployik api POST /api/projects/{id}/services/production/reset '{"confirm":"my-app-production"}'
deployik api DELETE /api/projects/{id}/services/preview
```

**After attach — don't forget the deploy:** attach is just a row in the database. The container only starts on the next deployment for that environment (the pipeline's `EnsureServices` hook). Tell the user to trigger one: `deployik api POST /api/projects/{id}/deployments '{"environment":"preview"}'`.

**Project rename is blocked while attached.** `PATCH /api/projects/{id}` with a new `name` will 409 if any service row exists for the project. Detach first.

---

## rollback

**Goal:** [click-paths.md#rollback](click-paths.md#rollback)

**Endpoints:**
- `GET /api/projects/{id}/deployments?environment=production&status=live` — find the previous good deployment
- `POST /api/projects/{id}/deployments` — trigger a redeploy of a specific commit

**Tier:** Mutation — confirm. **Production redeploys flagged as production.**

**Body shape (POST):**

```json
{ "environment": "production", "branch": "main", "commit_sha": "abc1234" }
```

If the user wants to roll back to "the previous good one", list deployments first and pick the most recent `live` row before the regression. Show its commit sha and message in the confirmation prompt.

**Invocations:**

```
deployik api GET /api/projects/{id}/deployments?environment=production&status=live&limit=5
deployik api POST /api/projects/{id}/deployments '{"environment":"production","branch":"main","commit_sha":"abc1234"}'
```

---

## Errors

When the API returns a non-2xx status:

- Print the HTTP status and the JSON body (the helper does this automatically — let the user see it).
- **Don't retry silently.** Most errors are user-meaningful (e.g. `403` from auto-build = missing OAuth scope, `409` from domain add = duplicate, `400` from secret = `NEXT_PUBLIC_` not allowed).
- Suggest the next step based on the error message rather than guessing or hand-rolling a fix.
