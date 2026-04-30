# Deployik How-To Skill — Design

## Summary

A project-scoped Claude skill that turns Claude into a live, hands-on assistant for the Deployik dashboard. The skill serves a non-technical primary user (the dev's girlfriend) who asks goal-phrased questions like "I want to make Google email work" or "I want my own domain."

The skill operates in two modes from a single source of truth:

1. **Guide mode** — Walks the user through clicks (route → sidebar breadcrumb → numbered steps) for the SPA at `web/src/pages/*`.
2. **Action mode** — Performs the same task on the user's behalf via the Deployik HTTP API, authenticated with a Personal Access Token, with confirmation gates for mutations and a hard block on destructive actions.

Action mode requires a small new backend capability — long-lived API tokens — which is captured here as a prerequisite work item.

## Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| Primary consumer | Claude as live reference | Triggers on user goals, not on a docs site |
| Audience | Non-technical (the user's girlfriend) | Drives plain language, no jargon, "click here" anchors |
| Scope of features | All user-facing SPA features (no infra/self-hosting) | Matches her surface area; infra docs are a different audience |
| v1 goal set | 6 must-haves (project create, custom domain, env/secrets, auto-deploy, password protection, rollback) | MoSCoW must-have cut; defer email/MX, analytics install, release tags, workspace switching to v2 |
| File layout | `SKILL.md` (router) + `click-paths.md` + `api-actions.md` + optional `helpers/` | Single skill; split files because end-to-end content will exceed the inline budget |
| Index style | Hybrid: goal-phrased index → feature-grouped recipes | Two ways in: search by goal (her); browse by feature (advanced) |
| Click-recipe format | Route + sidebar breadcrumb + numbered steps + gotcha | Unambiguous deep-link, mirrors the visual sidebar, covers form fields |
| External scope | Full end-to-end (registrars, Google admin, GitHub) | She came to make X work; dead-ending at the Deployik boundary defeats the point |
| Language | English skill; English replies (no bilingual) | Simpler maintenance; auth page Czech is a separate UI surface |
| Skill location | `lovinka-deployik/.claude/skills/deployik-howto/` (project-scoped, in git) | Versioned with the SPA it documents — UI changes and doc updates land in the same PR |
| Operating mode | Hybrid: guide + API action from one skill | Single source of truth; Claude routes by request shape ("show me how" vs "do it for me") |
| Auth strategy | New backend feature: Personal Access Tokens (PATs) | Existing JWT is browser-bound (1h/7d cookies); long-lived bearer tokens fit Claude's flow |
| Safety policy | Read = no confirm; mutate = confirm payload; destructive = typed confirmation; production flagged | Conservative default per CLAUDE.md "executing actions with care" |

## Chosen Approach

### Two work items

This design produces two separable deliverables. The skill **depends on** the PAT feature for action mode, but guide mode works without it.

#### Work item A — Backend: Personal Access Tokens

A small additive backend feature that introduces user-scoped long-lived API tokens.

- New migration `015_api_tokens.sql`: `api_tokens(id ULID, user_id FK, name, token_hash, last_used_at, expires_at NULL, revoked_at NULL, created_at)`.
- New `internal/db/queries_api_tokens.go`: create, list-for-user, get-by-hash, revoke, touch-last-used.
- Token format: `dpk_<base64url(32 random bytes)>`. Stored as SHA-256 hash; raw token shown to user **once** at creation.
- Middleware update (`internal/api/middleware/auth.go`): if `Authorization: Bearer dpk_...`, hash the value, look up `api_tokens`, verify not revoked + not expired, populate claims with the owning user. Existing JWT path unchanged.
- New handler `internal/api/handlers/tokens.go`:
  - `GET /api/me/tokens` — list (id, name, last_used_at, created_at; never the value).
  - `POST /api/me/tokens` — create `{name, expires_at?}`, returns `{id, name, token}` with the raw value once.
  - `DELETE /api/me/tokens/{id}` — revoke (sets `revoked_at`).
- Audit each create/revoke via `audit/recorder.go`.
- New SPA settings page `web/src/pages/UserTokens.tsx` (or extend an existing user-settings page) with create modal that copies the token to clipboard once and warns it's not retrievable.
- Rate-limit `/api/me/tokens` POST (auth-tier limiter is fine).

This work item ships independently of the skill. It also unblocks any future tooling (CLI, CI integrations).

#### Work item B — Skill: `deployik-howto`

Lives at `lovinka-deployik/.claude/skills/deployik-howto/` with this structure:

```
deployik-howto/
  SKILL.md           # frontmatter + when-to-use + decision: guide vs action vs both
  click-paths.md     # goal-indexed table → feature recipes (route + sidebar + steps)
  api-actions.md     # endpoint catalog per goal + curl examples + safety rules
  helpers/
    deployik         # bash wrapper: `deployik api <METHOD> <path> [json]`
                     # reads ~/.config/deployik/{base_url, token}, sends Bearer, pretty-prints
```

`SKILL.md` is intentionally thin — frontmatter, when-to-use triggers, the guide-vs-action decision tree, and links into the two reference files. This keeps the always-loaded portion small per writing-skills' token-efficiency rule. The two reference files are loaded only when needed.

##### Frontmatter

```yaml
---
name: deployik-howto
description: Use when a user asks how to do something in the Deployik dashboard — connecting a GitHub repo, custom domains, environment variables or secrets, auto-deploy on push, password protection, rollbacks — or asks Claude to perform one of those actions for them. Triggers include "how do I…", "where do I click…", "I want to make X work", and "just do X for me" phrased against Deployik.
---
```

Description is triggers-only per writing-skills CSO guidance — no workflow summary.

##### Decision flow inside SKILL.md

```
User asks about Deployik
       │
       ▼
Goal phrased as "do it for me" / imperative?
       │
   ┌───┴───┐
  yes      no
   │       │
   ▼       ▼
api-actions.md   click-paths.md
   │       │
   ▼       ▼
Mutation? ──yes──► Show payload + confirm        Walk her through:
   │                                             - Route URL
   no                                            - Sidebar breadcrumb
   │                                             - Numbered steps
   ▼                                             - Gotchas
Destructive? ──yes──► Require typed confirm
   │
   no
   │
   ▼
Execute via helpers/deployik api ...
```

##### v1 task index (in click-paths.md)

| Goal (plain language) | Feature recipe |
|-----------------------|----------------|
| I want to put my GitHub repo online | `#create-project` |
| I want to use my own domain (example.com) | `#custom-domain` (incl. registrar A-record steps) |
| I want to set environment variables / API keys | `#env-vars` |
| I want it to redeploy when I push to GitHub | `#auto-deploy` |
| I want a password before people see the site | `#password-protection` |
| I want to roll back to a previous version | `#rollback` |

Each recipe section has:

```markdown
### <feature-id>

**Goal:** <plain language restatement>
**Route:** `/projects/{id}/...`
**Sidebar:** Project → Settings → ...

**In Deployik:**
1. ...
2. ...

**Outside Deployik (if applicable):**
- At your registrar (Cloudflare, etc.): ...

**Gotcha:** ...
**API equivalent:** see `api-actions.md#<feature-id>`
```

##### v1 API action catalog (in api-actions.md)

Each entry pairs the recipe id with HTTP method/path/body and a curl skeleton using the helper:

| Recipe id | API call | Tier |
|-----------|----------|------|
| `create-project` | `POST /api/projects` | Confirm |
| `custom-domain` | `POST /api/projects/{id}/domains` then `POST /api/projects/{id}/domains/{did}/verify` | Confirm |
| `env-vars` | `POST /api/projects/{id}/env` (single upsert) or `PUT` (bulk replace) | Confirm; **production = extra prompt** |
| `auto-deploy` | `PUT /api/projects/{id}/auto-build` | Confirm |
| `password-protection` | `PUT /api/projects/{id}/protection` (enable/disable); `POST /api/projects/{id}/protection/regenerate` | Confirm |
| `rollback` | `POST /api/projects/{id}/deployments` with previous `commit_sha` | Confirm |

Plus read-only endpoints used as supporting context: `GET /api/projects`, `GET /api/projects/{id}/deployments`, `GET /api/projects/{id}/domains`, etc. — no confirmation needed.

##### Helper script

`helpers/deployik` is a thin bash wrapper:

```bash
#!/usr/bin/env bash
# Reads ~/.config/deployik/config (KEY=value), sends Bearer auth, pretty-prints JSON.
# Usage: deployik api GET /api/projects
#        deployik api POST /api/projects/abc/env '{"key":"FOO","value":"bar","environment":"preview"}'
```

Config file format:

```
DEPLOYIK_BASE_URL=https://deploy.example.com
DEPLOYIK_TOKEN=dpk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

The skill instructs Claude to use this helper exclusively — never raw curl with embedded tokens — so tokens stay out of shell history.

##### Safety policy concretized

| Verb / target | Policy |
|---------------|--------|
| `GET *` | Execute silently |
| `POST/PUT/PATCH` to non-production | Print intended `<METHOD> <path>` and JSON body; ask "do this?" |
| `POST/PUT/PATCH` to production env | Print payload, flag PRODUCTION, ask explicit yes/no |
| `DELETE *` and `POST .../regenerate` | Require typed string confirmation matching the target name (e.g. `yes delete example.com`) |
| Any error | Surface raw response + status code; do not retry silently |

### Alternatives considered (and rejected)

- **Two separate skills** (`deployik-howto` + `deployik-cli`): cleaner separation but doubles activation triggers and risks one loading without the other when both are useful. Rejected.
- **Action-only skill, no guide**: faster for power users but defeats the non-technical audience — if API auth fails, she's stuck with no fallback. Rejected.
- **Refresh-cookie auth instead of PAT**: avoids backend work, but cookie rotation conflicts with the user's browser session and the parsing is fragile. Rejected.
- **Dev-mode-only auth**: useful for the dev's testing but doesn't serve the actual user against the live VPS. Rejected.
- **Single SKILL.md with everything inline**: full end-to-end content (registrar UIs, Google admin steps, multiple recipes) will exceed the always-loaded budget. Splitting into router + on-demand reference files matches writing-skills' token-efficiency guidance.

## Architecture

### Skill file tree

```
lovinka-deployik/.claude/skills/deployik-howto/
├── SKILL.md           # ~150 words: frontmatter + when-to-use + router
├── click-paths.md     # Goal index + 6 feature recipes
├── api-actions.md     # 6 API recipes + safety rules + helper usage
└── helpers/
    └── deployik       # bash wrapper
```

### Auth path (action mode)

```
User: "Add env var STRIPE_KEY=sk_test_... to preview of project foo"
   │
   ▼
Claude reads SKILL.md → routes to api-actions.md → finds env-vars recipe
   │
   ▼
Claude resolves project id via `deployik api GET /api/projects` (read, no confirm)
   │
   ▼
Claude shows: "I'll POST /api/projects/01.../env with
              {key:STRIPE_KEY, value:sk_test_..., environment:preview}. Confirm?"
   │
   ▼ (user says yes)
helpers/deployik api POST /api/projects/01.../env '{...}'
   │
   ▼
Reads ~/.config/deployik/config → curl with Bearer → prints response
```

### Backend changes (work item A)

```
internal/db/migrations/015_api_tokens.sql      ← new
internal/db/queries_api_tokens.go              ← new
internal/db/models.go                          ← add ApiToken struct
internal/api/handlers/tokens.go                ← new (CRUD)
internal/api/middleware/auth.go                ← extend Bearer parsing
internal/api/router.go                         ← /api/me/tokens routes
web/src/pages/UserTokens.tsx                   ← new (or extend existing settings)
web/src/lib/api.ts                             ← createToken, listTokens, revokeToken
```

No changes to existing JWT or cookie flow. PATs are an additive auth method.

### Tradeoff snapshot

| Concern | Mitigation |
|---------|------------|
| Skill rot — UI changes break click recipes | Skill lives in the same repo as the SPA; PRs that change `web/src/pages/*` should update `click-paths.md`. Pre-commit linter could flag if SPA files changed without skill change (deferred). |
| End-to-end content (registrars, Google admin) rots fast | Skill explicitly notes "external UIs change; if a step doesn't match, tell Claude what you see". Skill scopes to Cloudflare + 2 others, not all registrars. |
| Token leakage via shell history | Helper script reads from config file only; skill is explicit "never inline tokens in curl". |
| Confirmation fatigue | Tiered: read silent, mutate confirm, destructive typed-confirm. Production flagged louder than preview. |
| PAT compromise blast radius | Tokens are user-scoped, not admin-scoped. Future: add per-project or per-scope tokens (deferred). |

## Open Questions

1. **Production env-var changes — confirmation strength.** Should production-scoped writes require typed confirmation (matching destructive tier) instead of just an extra prompt? Lean: yes, given encrypted at rest doesn't help if the wrong value goes live.
2. **Token expiry default.** PATs default to no expiry, or 90 days? GitHub-style 30/60/90/never picker? Lean: optional `expires_at`, default never, UI offers 30/90/365/never.
3. **Token scopes.** v1 = full user-scoped. Should we model project-scoped tokens (read-only, single-project) for safer defaults? Defer to v2.
4. **Where the token-management UI lives.** New top-level "Account → Tokens" sidebar item, or under an existing settings page? Defer until UserTokens.tsx is wired.
5. **Helper script distribution.** Skill ships the bash wrapper inside `helpers/`. Does Claude run it from there, or does the user `chmod +x` and `cp` it to `~/.local/bin`? Lean: skill instructs Claude to invoke it via the in-skill path; no install step.
6. **Email feature** (recently added per CLAUDE.md update — `handlers/email.go`, Webglobe SMTP, contact form prompts). Not in v1; add as v2 goal under "I want emails to send from my site."
7. **DNS provider coverage.** v1 covers "any registrar" generically + Cloudflare specifically (most common). Do we add Namecheap and a registrar table, or keep it generic and prompt the user to name their registrar? Lean: generic + Cloudflare specific, expand on demand.
8. **Activation overlap.** The skill triggers on UI goal phrasing. Make sure the description does NOT activate when the dev is editing Deployik source code (e.g. "fix the bug in domains.go"). Phrasing should anchor to the dashboard, not the codebase.

## Next Step

If approved, the natural follow-up is `/superpowers:writing-plans` to produce two implementation plans:

- **Plan A** — Backend API tokens (migration, queries, middleware, handlers, SPA management UI). Self-contained, mergeable independently.
- **Plan B** — Skill `deployik-howto` (`SKILL.md`, `click-paths.md`, `api-actions.md`, `helpers/deployik`) with RED-GREEN-REFACTOR per writing-skills (baseline scenarios → write skill → close loopholes).

Plan B blocks on Plan A only for action-mode testing — guide mode can ship independently.
