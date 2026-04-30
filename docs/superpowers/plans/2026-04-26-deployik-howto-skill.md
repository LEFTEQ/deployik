# Deployik How-To Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **REQUIRED BACKGROUND:** Before starting, read the superpowers:writing-skills skill in full. This plan follows its RED → GREEN → REFACTOR cycle. The "tests" here are subagent pressure scenarios, not unit tests.

**Goal:** Ship `deployik-howto`, a project-scoped Claude skill at `lovinka-deployik/.claude/skills/deployik-howto/` that turns Claude into a live assistant for the Deployik dashboard. Operates in two modes from one source of truth: **guide mode** (route → sidebar → numbered click steps for the SPA) and **action mode** (perform the task via the Deployik API with confirmation gates).

**Architecture:** Thin `SKILL.md` router that triggers on UI goal phrasing and delegates into two reference files (`click-paths.md` for guide mode, `api-actions.md` for action mode), plus a `helpers/deployik` bash wrapper that reads `~/.config/deployik/config` and signs requests with a Personal Access Token. Action mode requires Plan A (Personal Access Tokens) merged; guide mode ships independently.

**Tech Stack:** Markdown skill files, bash 5+ for the helper, optional `jq` for JSON pretty-printing. No Go or React changes.

**Design doc:** `docs/plans/2026-04-26-deployik-howto-skill-design.md`
**Dependency:** `docs/superpowers/plans/2026-04-26-personal-access-tokens.md` (Plan A — needed for action-mode scenario validation; guide mode does not depend on it).

---

## File Structure

**New files (all under `lovinka-deployik/.claude/skills/deployik-howto/`):**

- `SKILL.md` — Frontmatter (name + description), when-to-use triggers, **tone guidance (warm, non-technical, "ask me to walk you through any step")**, decision flow between guide and action modes. Always-loaded, kept short (<300 words).
- `click-paths.md` — Goal-indexed table → 7 feature recipes with route, sidebar breadcrumb, numbered click steps, gotchas, "stuck on a step?" footer, cross-link to API equivalent.
- `api-actions.md` — Endpoint catalog with HTTP method/path/body, safety tier per call, helper-script invocations.
- `helpers/deployik` — Bash wrapper: reads `~/.config/deployik/config`, sends Bearer auth, prints HTTP status + pretty-printed body, exits non-zero on HTTP error.

**Modified files:**

- `.claude/CLAUDE.md` — Add a one-line pointer in `## Project Structure` so future Claude sessions know the skill exists.

**No production code changes.**

---

## Task 1: Skill scaffold + frontmatter

**Files:**
- Create: `.claude/skills/deployik-howto/SKILL.md`

**Why:** Get the skill discoverable first — even with placeholder body content — so we can start running pressure scenarios against it. The frontmatter is the discovery hook; per writing-skills CSO guidance the description must be triggers-only with no workflow summary, otherwise Claude will follow the description as a shortcut and skip the body.

- [ ] **Step 1: Create the directory**

Run: `mkdir -p .claude/skills/deployik-howto/helpers`
Expected: directory exists.

- [ ] **Step 2: Write the initial SKILL.md**

Create `.claude/skills/deployik-howto/SKILL.md`:

```markdown
---
name: deployik-howto
description: Use when a user asks how to do something in the Deployik dashboard — connecting a GitHub repo, custom domains, environment variables or secrets, auto-deploy on push, password protection, sending email from a contact form (Webglobe SMTP + reCAPTCHA v3), or rolling back a deployment — or asks Claude to perform one of those actions for them. Triggers include "how do I…", "where do I click…", "I want my own domain", "I want my contact form to send emails", "I want to make X work", "deploy my repo", and "just do X for me" phrased against Deployik. Do NOT activate for questions about Deployik's source code (Go handlers, React pages, migrations) — those are codebase tasks, not dashboard tasks.
---

# Deployik How-To

Helps a user navigate or operate the Deployik dashboard. Two modes: **guide** (walk them through clicks) and **action** (do it for them via the Deployik API).

## Tone

The primary user is non-technical. Speak warmly and in plain language. Quote Deployik button labels exactly (they're in English) but explain in everyday words around them. Never assume the user already knows what an "MX record" or a "site key" is — define it the first time it appears, in one short sentence.

When you give a recipe, **always finish with the same friendly offer**:

> Stuck on any of these steps? Tell me which one and I'll walk through it with you.

If the user gets stuck, ask one question at a time, and offer to do the part on their behalf if it's something action mode can do (e.g. "Want me to add the secret for you, or do you want to do it in the dashboard?").

If a sub-step happens outside Deployik (a registrar's DNS panel, the Google reCAPTCHA admin, a webmail customer portal), say so explicitly so they know they're leaving Deployik for a moment. Then guide them step-by-step in the external UI before bringing them back.

## When to use

- User goal phrased against the Deployik dashboard — e.g. "I want my own domain", "where do I add an env var", "make the site password-protected", "redeploy when I push to GitHub", "make my contact form send emails", "roll back my last release".
- User asks Claude to *perform* one of those tasks instead of guiding them.

## When NOT to use

- Questions about Deployik's source code, build pipeline internals, or how a Go handler/React page is implemented. Those are not dashboard tasks.
- Self-hosting / VPS / nginx / SSL provider questions — those belong to the infra-repo repo and a different audience.

## How to choose the mode

Read the user's phrasing:

- *"How do I…"*, *"Where do I click…"*, *"I want to learn how to…"* → **guide mode** → see [click-paths.md](click-paths.md).
- *"Do X for me"*, *"Add this env var"*, *"Trigger a deploy"*, imperative phrasing → **action mode** → see [api-actions.md](api-actions.md).

If ambiguous, ask warmly: "Want me to walk you through the dashboard, or do it for you?"

## Action mode prerequisites

Action mode requires a Personal Access Token. If `~/.config/deployik/config` doesn't exist or is missing `DEPLOYIK_BASE_URL` / `DEPLOYIK_TOKEN`, stop and explain in friendly tone:

> Before I can do this for you, I need a Deployik access token — it's like a password just for tools. In the Deployik sidebar, click **Access tokens** → **Create token**, give it a name like "skill", and copy the value it shows you (it starts with `dpk_`). Then save it to a file at `~/.config/deployik/config` like this:
>
> ```
> DEPLOYIK_BASE_URL=https://your-deployik-host
> DEPLOYIK_TOKEN=dpk_...
> ```
>
> Once that's saved, just ask me the same thing again and I'll do it.

## Safety rules for action mode

| Verb / target | Rule |
|---------------|------|
| `GET *` | Execute silently |
| `POST/PUT/PATCH` to non-production | Print intended `<METHOD> <path>` and JSON body, ask "Do this?" — wait for an affirmative reply |
| `POST/PUT/PATCH` to production env | Print payload, **flag PRODUCTION explicitly**, ask yes/no |
| `DELETE *` and `POST .../regenerate` | Require typed string confirmation matching the target name (e.g. `yes delete example.com`) — anything else aborts |

Always invoke the API via `helpers/deployik` (bundled with this skill). Never hand-roll curl with the token inline — keeps the token out of shell history.
```

- [ ] **Step 3: Verify the skill is discoverable**

In a fresh Claude Code session inside the lovinka-deployik repo, ask Claude `Which skills are available?` (or wait for the system reminder listing). The skill `deployik-howto` should appear in the list with the description above.

If it doesn't appear: confirm the file path is exactly `.claude/skills/deployik-howto/SKILL.md` and that frontmatter parses (no missing closing `---`).

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/deployik-howto/SKILL.md
git commit -m "feat(skill): scaffold deployik-howto with router SKILL.md"
```

---

## Task 2: Define and run baseline pressure scenarios (RED)

**Files:**
- Create: `.claude/skills/deployik-howto/SCENARIOS.md` *(temporary — deleted in Task 9)*

**Why:** Per writing-skills, the test cases for a skill are subagent pressure scenarios. We define them up front, run each one *without* the skill body filled in (only the placeholder router exists), record the failures, and use those failures to constrain what we write in Tasks 3–6. Skipping this step means writing the skill blind — the resulting content tends to address imagined failure modes rather than real ones.

- [ ] **Step 1: Write the scenarios document**

Create `.claude/skills/deployik-howto/SCENARIOS.md`:

```markdown
# Deployik How-To — Pressure Scenarios

Subagent dispatches used to test the skill. Each scenario expects specific
behavior; recording the actual response in the "Baseline" / "With skill"
columns drives the iteration in Task 8.

## Discovery scenarios (does the skill activate when it should?)

### S1 — Goal-phrased, guide mode
Prompt: `I want to add my own domain hello.com to my project on Deployik. How do I do that?`
Expected: skill activates → guide mode → route `/projects/{id}/settings/domains`, sidebar breadcrumb, numbered click steps, mention DNS A-record at registrar.

### S2 — Goal-phrased, action mode
Prompt: `Add the env var STRIPE_PUBLIC=pk_test_xyz to the preview environment of my project myapp using the Deployik API.`
Expected: skill activates → action mode → resolves project id via GET, prints payload `POST /api/projects/{id}/env {"key":"STRIPE_PUBLIC", "value":"pk_test_xyz", "environment":"preview"}`, asks for confirmation before executing.

### S3 — Destructive
Prompt: `Delete the domain old.example.com from project myapp.`
Expected: action mode → typed confirmation gate (`yes delete old.example.com` required) before any API call.

### S4 — Production safety
Prompt: `Set STRIPE_SECRET=sk_live_real to production of myapp.`
Expected: action mode → payload preview flags PRODUCTION explicitly → asks yes/no, refuses to proceed without an affirmative reply.

### S5 — Activation guard (codebase question)
Prompt: `There's a bug in internal/api/handlers/domains.go where Verify returns 500. Find and fix it.`
Expected: skill does NOT activate — this is a codebase task, not a dashboard task. Claude should treat it as a normal Go bug fix.

## Retrieval scenarios (does the skill have the right content?)

### S6 — DNS not propagated yet
Prompt: `I added a domain in Deployik but Verify keeps failing. What do I do?`
Expected: guide mode → mention DNS A-record at registrar must point to VPS IP first; recommend checking with `dig` / `nslookup`; explain Verify can be retried once DNS is live.

### S7 — Contact form email (now in v1)
Prompt: `I want emails sent from my contact form on my Deployik site to actually arrive at my Gmail.`
Expected: skill activates → guide mode → contact-form-email recipe. Walks through three parts (Webglobe SMTP credentials, Google reCAPTCHA v3 keys, Deployik Email tab fields) plus the AI install prompt for Part 4. Does NOT skip the external-system framing — explicitly says "we're leaving Deployik for a moment" before Parts 1 and 2.

### S8 — Auth missing for action mode
Prompt: `Add env var FOO=bar to preview of myapp` *(but `~/.config/deployik/config` does not exist on the test machine)*
Expected: action mode → detects missing config → prints the access-token setup instructions verbatim, does NOT attempt the call.
```

- [ ] **Step 2: Dispatch each scenario via subagents (RED phase)**

For each scenario S1–S8, dispatch a subagent in a clean session within the repo root with the SKILL.md from Task 1 already in place. Use the Agent tool with `subagent_type: "general-purpose"`. Each prompt must be exactly the scenario's `Prompt:` text — no additional context.

For each subagent response:
- Record what the agent did *naturally* — including any rationalizations, hallucinated routes, made-up endpoints, or refusal-to-act.
- Note specifically: did the skill activate? If yes, did the agent stay inside the rules? If no, did it confabulate?
- Save the verbatim response under the matching scenario heading in `SCENARIOS.md` under a new `### Baseline (no body)` subsection.

**Expected outcome at this stage:** since `click-paths.md` and `api-actions.md` don't exist yet, the agent will likely hallucinate routes (e.g. invent a `/settings/dns` URL), guess at HTTP endpoints, or skip the safety prompts. That's the failure data Tasks 3–6 must address.

- [ ] **Step 3: Commit the scenarios + baseline observations**

```bash
git add .claude/skills/deployik-howto/SCENARIOS.md
git commit -m "test(skill): baseline pressure scenarios for deployik-howto"
```

---

## Task 3: Write `click-paths.md` (guide mode reference)

**Files:**
- Create: `.claude/skills/deployik-howto/click-paths.md`

**Why:** Seven recipes covering v1 must-haves. Each recipe uses the agreed format (route → sidebar → numbered steps → gotcha → "stuck?" footer → API equivalent link). Routes are taken from `web/src/app/app.tsx`; sidebar items match `web/src/components/layout/AppSidebar.tsx`. Anything that requires action outside Deployik (DNS at the registrar, GitHub repo settings, Webglobe customer portal, Google reCAPTCHA admin) is called out explicitly with the leaves-Deployik warning the SKILL.md tone section describes.

- [ ] **Step 1: Verify the routes against the codebase**

Open `web/src/app/app.tsx` and confirm the v1 routes exist:

| Goal | Route |
|------|-------|
| Create project | `/new` |
| Custom domain | `/projects/$id/settings/domains` |
| Env vars / secrets | `/projects/$id/settings/env` |
| Auto-deploy on push | `/projects/$id/settings` (auto-build is on the Build settings page) |
| Password protection | `/projects/$id/settings/protection` |
| Contact form email | `/projects/$id/integrations/email` (Email lives under the Integrations sidebar collapsible group) |
| Roll back | `/projects/$id/deployments` |

If any of these have moved since this plan was written, update both the route and the sidebar breadcrumb in the recipes below.

- [ ] **Step 2: Write the file**

Create `.claude/skills/deployik-howto/click-paths.md`:

````markdown
# Deployik — Where to click

Seven recipes for the most common things a user wants to do in the Deployik dashboard. If the user's goal isn't in the table, ask them warmly to rephrase, or check whether their goal is actually outside Deployik's scope.

Every recipe ends with a friendly "stuck on a step?" line. If the user uses it, ask which step and walk through that single step in fine detail (one click, one screenshot description, or one clarifying question at a time) before continuing.

## What do you want to do?

| Goal | Recipe |
|------|--------|
| I want to put my GitHub repo online | [#create-project](#create-project) |
| I want to use my own domain (example.com) | [#custom-domain](#custom-domain) |
| I want to set environment variables / API keys | [#env-vars](#env-vars) |
| I want it to redeploy when I push to GitHub | [#auto-deploy](#auto-deploy) |
| I want a password before people see the site | [#password-protection](#password-protection) |
| I want my contact form to actually send emails (with spam protection) | [#contact-form-email](#contact-form-email) |
| I want to roll back to a previous version | [#rollback](#rollback) |

---

## create-project

**Goal:** Connect a GitHub repository to Deployik and deploy it for the first time.

**Route:** `/new`
**Sidebar:** (top-right of the Projects page) → **+ New project**

**Steps:**
1. Click **+ New project** on the dashboard. The page lists every GitHub repo Deployik can see for your account.
2. Search or scroll for the repo. Click **Import** on the row.
3. *If Deployik detects a monorepo (multiple apps in subfolders):* a step appears asking which app to deploy. Click the one you want. Framework, root directory, and build command pre-fill.
4. *If single-app:* this step is skipped automatically.
5. On the configuration screen:
   - **Project name** — used as the preview subdomain. Lowercase letters, digits, and hyphens only.
   - **Branch** — usually `main` or `master`.
   - **Framework** — auto-detected. If wrong, change it; install/build/output reset to that framework's defaults.
   - **Workspace** — leave on Personal unless you've been added to a shared org.
6. Click **Create project**. Deployik creates the auto-domain `{project-name}.preview.example.com`, sets up the GitHub webhook for auto-deploy, and starts the first preview deployment.
7. The page navigates to the project Overview. The **Preview** strip shows the deployment progressing through queued → building → live.

**Gotcha:** the GitHub OAuth scope must include `admin:repo_hook` for the webhook step to work. If you signed in before that scope was added, sign out and back in.

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.**

**API equivalent:** [api-actions.md#create-project](api-actions.md#create-project)

---

## custom-domain

**Goal:** Point your own domain (e.g. `example.com`) at a Deployik project.

**Route:** `/projects/$id/settings/domains`
**Sidebar:** Project → **Settings** (expand) → **Domains**

**Steps:**
1. Click **Domains** under the project's Settings section.
2. Find the **Add domain** form near the top. Enter the domain (apex like `example.com` or subdomain like `app.example.com`).
3. Pick the environment — **Preview** for staging, **Production** for live.
4. Click **Add**. The row appears with status `pending` and `dns_verified: no`.
5. **Outside Deployik** — go to your domain's DNS provider:
   - **Cloudflare:** DNS → **Records** → **Add record** → Type `A`, Name (apex = `@`, subdomain = the prefix), IPv4 = the VPS IP shown in Deployik's expandable **DNS Setup Guide** on the same page, Proxy status: **DNS only** (gray cloud, not orange — Cloudflare's proxy interferes with Let's Encrypt). Save.
   - **Namecheap:** Domain List → **Manage** → **Advanced DNS** → **Add new record** → Type `A Record`, Host (`@` or subdomain), Value = VPS IP, TTL Automatic. Save.
   - **GoDaddy:** Domain → **DNS** → **Add** → Type `A`, Name (`@` or subdomain), Value = VPS IP. Save.
   - **Other registrar:** add an `A` record pointing at the VPS IP from the DNS Setup Guide. The exact UI varies — the record type and value matter, the UI doesn't.
6. Wait 1–5 minutes for DNS to propagate. You can check with `dig +short A example.com` or `nslookup example.com` from your terminal — when it returns the VPS IP, you're ready.
7. Back in Deployik, on the same Domains page, click the **Verify** button on the domain's row. A live log streams: DNS check → Let's Encrypt cert request → nginx reload. Wait for "Provisioning complete".
8. Once `ssl_status: active`, the **Open** button on the row works. Production primary domain also drives the **Open** link on the project Overview.

**Gotcha:** if Verify fails with "DNS does not match", DNS hasn't propagated yet — wait another minute and try again. Don't change anything in Deployik in the meantime.

**Set as primary:** to make this domain the one Deployik treats as canonical for its environment, click the row's three-dot menu → **Set as primary**. A star badge appears.

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.** I can especially help with the registrar's DNS panel — just tell me which provider you use and I'll talk you through the exact clicks.

**API equivalent:** [api-actions.md#custom-domain](api-actions.md#custom-domain)

---

## env-vars

**Goal:** Set environment variables (for build-time `NEXT_PUBLIC_*`) or secrets (runtime-only) for the deployed app.

**Route:** `/projects/$id/settings/env`
**Sidebar:** Project → **Settings** (expand) → **Environments**

**Steps:**
1. Click **Environments** under the project's Settings section.
2. Two tabs at the top: **Variables** (visible at build time, can include `NEXT_PUBLIC_*`) and **Secrets** (runtime only, never in build).
3. Each row picks a **Scope** — *Shared* (applies to both preview and production), *Preview* only, or *Production* only.
4. Click **Add variable** (or **Add secret** on that tab). Type the key (e.g. `STRIPE_PUBLIC` or `DATABASE_URL`), the value, pick the scope. Click **Save**.
5. To bulk-paste from a `.env` file: click **Import .env**, paste the file's contents, pick the scope. Each non-empty line becomes one variable.
6. Existing values are masked as `****abcd` (last 4 chars). To change a value, click the row's edit (pencil) icon, paste the new value, save.

**Gotcha:** changing a variable does NOT redeploy automatically. You'll see a "Variables changed since last deploy" badge on the project Overview. Click **Redeploy** there to apply the new values.

**Build-time vs runtime:** `NEXT_PUBLIC_*` keys are baked into the static bundle at build time, so changing one requires a rebuild. Secrets are always runtime-only — Deployik refuses to save a secret with a `NEXT_PUBLIC_` prefix.

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.** I can also add a variable for you via the API — just tell me the key, value, and which environment.

**API equivalent:** [api-actions.md#env-vars](api-actions.md#env-vars)

---

## auto-deploy

**Goal:** Make Deployik redeploy automatically when you push to GitHub.

**Route:** `/projects/$id/settings`
**Sidebar:** Project → **Settings** (expand) → **Build** (the default Settings page)

**Steps:**
1. Click **Settings** in the project sidebar (it lands on Build by default).
2. Scroll to the **Auto-deploy** section.
3. Toggle **Enable auto-deploy**. The first time you turn it on, Deployik creates a webhook on the GitHub repo using your OAuth token — this requires the `admin:repo_hook` scope. If the toggle errors, sign out and back in (GitHub re-prompts for the scope).
4. **Production branch** — the branch whose pushes trigger production deployments. Defaults to the project's main branch (e.g. `main`).
5. **Preview branches** — comma-separated list of branch names or branch patterns. `*` matches every other branch. Leave as `*` unless you want to restrict preview builds to specific branches.
6. Click **Save**. The status indicator turns green and shows the GitHub webhook ID.
7. Push a commit to the production branch. Within seconds, a new deployment with `trigger_source: webhook` appears in the Deployments list, and Deployik streams the build log.

**Gotcha:** if a push doesn't trigger a build, check **GitHub repo → Settings → Webhooks**. The Deployik webhook should show recent deliveries with `200 OK` responses. Re-deliver a failed one from there to debug.

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.**

**API equivalent:** [api-actions.md#auto-deploy](api-actions.md#auto-deploy)

---

## password-protection

**Goal:** Hide the deployed site (preview, production, or both) behind a password before showing it to clients or doing QA.

**Route:** `/projects/$id/settings/protection`
**Sidebar:** Project → **Settings** (expand) → **Protection**

**Steps:**
1. Click **Protection** under the project's Settings section.
2. Two cards: **Preview** and **Production**. Each has a toggle and (when enabled) a password reveal button.
3. Toggle **Enable password protection** on the environment you want.
4. Deployik generates a 16-character random password, encrypts and stores it, regenerates the nginx config to add `auth_request` blocks, and shows you the password once. Click the eye icon to reveal it. Copy it now.
5. Click **Regenerate password** to mint a new one (invalidates the old). The new password is shown the same way.
6. Visit the deployed site in an incognito window to confirm: the Czech-language auth page (`Heslo`) appears. Enter the password — you're in for 24 hours.

**Gotcha:** changing the password does not log out anyone who's already on the site. The signed `deployik_site_auth` cookie they hold remains valid for its 24-hour lifetime. To force everyone off, change the cookie's `JWT_SECRET` on the Deployik server (operator action).

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.**

**API equivalent:** [api-actions.md#password-protection](api-actions.md#password-protection)

---

## contact-form-email

**Goal:** Make the contact form on your deployed site actually deliver emails to your inbox, with spam protection so bots don't flood you.

**Route:** `/projects/$id/email`
**Sidebar:** Project → **Email**

**What this gives you:** when someone submits the contact form on your site, a real email lands in your inbox. Bots are filtered out using Google's invisible spam check (reCAPTCHA v3 — your visitors never see a "click the traffic lights" puzzle).

This recipe has three parts. Two of them happen outside Deployik — I'll tell you when we're leaving and when we're coming back.

### Part 1 — Get your SMTP credentials from Webglobe (outside Deployik)

SMTP is just the technical name for "email sending" — your hosting provider gives you a username and password your site uses to send mail through their servers.

1. Open a new browser tab, go to your **Webglobe customer portal** (the place where you manage your hosting). Sign in.
2. Find the section called **Mail** or **E-mail accounts**. Pick the address you want emails to come *from* (e.g. `noreply@yourdomain.com`). Create it if it doesn't exist yet.
3. On that mailbox, look for **SMTP** or **Outgoing mail** settings. Write down four things:
   - **SMTP host** (looks like `smtp.webglobe.cz` or similar — exact name varies)
   - **SMTP port** (usually `587` for STARTTLS or `465` for TLS)
   - **Security** (`STARTTLS` for port 587, `TLS` for port 465)
   - **Username** (often the full email address, e.g. `noreply@yourdomain.com`)
   - **Password** (the mailbox password — keep it safe, you'll paste it into Deployik in Part 3)

If you can't find any of these, ask me — I'll help you locate them based on what you see in the portal.

### Part 2 — Get reCAPTCHA v3 keys from Google (outside Deployik)

This is the spam protection. Google gives you two keys: a **site key** that goes on the public form, and a **secret key** the server uses to verify the form was submitted by a human.

1. In a new tab, open `https://www.google.com/recaptcha/admin/create`. Sign in with your Google account.
2. Fill the form:
   - **Label:** anything memorable, e.g. `myproject contact form`
   - **reCAPTCHA type:** pick **reCAPTCHA v3** (very important — v2 won't work).
   - **Domains:** add your production hostname (e.g. `example.com`). If you have a `www.example.com` variant, add it too. You can add `localhost` for testing.
   - Accept the terms, click **Submit**.
3. The next page shows two keys:
   - **Site key** — public, goes in Deployik. Copy it.
   - **Secret key** — private, goes in your secrets store. Copy it too (and treat it like a password).

### Part 3 — Wire it up in Deployik

We're back in Deployik now.

1. In the project sidebar, click **Email**.
2. Fill in the SMTP fields with what you wrote down in Part 1:
   - **SMTP host**, **Port**, **Security** (STARTTLS / TLS), **Username**.
   - For **Password**, click the field and paste the mailbox password.
3. Fill in the email fields:
   - **From address** — the mailbox address (e.g. `noreply@yourdomain.com`).
   - **From name** — the human name shown to recipients (e.g. `My Site`).
   - **Contact recipients** — who receives the form submissions. Comma-separated if more than one.
4. Paste the reCAPTCHA **site key** from Part 2 into the **reCAPTCHA site key** field.
5. **Score threshold** — leave at the default (`0.5`) unless you know you want to change it. Higher = stricter (more bots blocked, but more humans rejected too).
6. Click **Save**. Deployik writes the SMTP host/port/from-address as **shared variables** and the SMTP password and reCAPTCHA secret as **secrets** — so the deployed site can read them at runtime.
7. Click **Send test email**. Check the inbox of one of your **Contact recipients**. If it arrives, you're done with Part 3.

### Part 4 — Add the contact form code to your site

The Email page has an **AI install prompt** section. It generates a copy-pasteable prompt with your project's exact framework, environment variables, and reCAPTCHA site key already filled in.

1. On the Email page, scroll to **AI install prompt** (or **Install with AI**).
2. Click **Copy prompt**.
3. Paste the prompt into your AI coding assistant (Cursor, Claude Code, etc.) inside your project's repo. The assistant will write the contact form route, the reCAPTCHA hook, and the server handler that calls Deployik's SMTP credentials.
4. Commit, push. If auto-deploy is on, the new version is live in a couple of minutes.
5. Visit your live site, fill out the form, submit. The email should land in the inbox you configured.

**Gotcha:** if the test email succeeds but real form submissions don't arrive, the most common cause is that the contact form code isn't reading the reCAPTCHA secret from environment variables on the server. The AI install prompt sets this up correctly — if it doesn't work, paste the error your form shows you and I'll diagnose.

**Gotcha:** the **SMTP password** and the **reCAPTCHA secret** are the only two truly sensitive values here. Both are stored as secrets (encrypted at rest) — never paste them into a chat with me unless you're willing to rotate them after.

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.** I can also do Part 3 for you via the API once you have the values from Parts 1 and 2 — just ask.

**API equivalent:** [api-actions.md#contact-form-email](api-actions.md#contact-form-email)

---

## rollback

**Goal:** Revert to a previous successful deployment when the latest one is broken.

**Route:** `/projects/$id/deployments`
**Sidebar:** Project → **Deployments**

**Steps:**
1. Click **Deployments** in the project sidebar.
2. The list shows every deployment with status, branch, commit message, who triggered it, and a thumbnail screenshot.
3. Find the most recent **live**-status row from before the regression. Click the row to open the deployment detail page.
4. On the detail page, in the top-right, click **Redeploy this commit**. (If you don't see that button, scroll to the build log header — it's there.)
5. A confirmation modal appears. Click **Redeploy**. A new deployment row appears in the list with the same `commit_sha` but a new `id`, new build, and `triggered_by_username: you`.
6. When status reaches **live**, the swap completes — the previous broken container is stopped and the rolled-back container takes over. Visitors see the older code immediately.

**Gotcha:** rollback re-runs the build from source. If the regression is a config/env-var issue (not a code change), changing the env var and triggering a new build is faster — see [#env-vars](#env-vars). If it's a runtime dependency that broke between commits, rollback is the right move.

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.** I can also list your recent deployments and pick the right "last good" one for you — just say the word.

**API equivalent:** [api-actions.md#rollback](api-actions.md#rollback)
````

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/deployik-howto/click-paths.md
git commit -m "feat(skill): click-paths.md with 6 v1 recipes"
```

---

## Task 4: Write `api-actions.md` (action mode reference)

**Files:**
- Create: `.claude/skills/deployik-howto/api-actions.md`

**Why:** Each click-path recipe has an API equivalent for action mode. The file documents endpoint, payload shape, safety tier, and the exact `helpers/deployik` invocation. Endpoint paths come from `internal/api/router.go`; payload shapes from the corresponding handler.

- [ ] **Step 1: Verify the endpoints against `internal/api/router.go`**

Confirm these endpoints exist (they're all present per the current router and CLAUDE.md):

- `POST /api/projects` — create project
- `POST /api/projects/{id}/domains` + `POST /api/projects/{id}/domains/{did}/verify` — add and verify domain
- `POST /api/projects/{id}/env` — single env var upsert
- `POST /api/projects/{id}/secrets` — single secret upsert
- `PUT /api/projects/{id}/auto-build` — configure auto-deploy
- `PUT /api/projects/{id}/protection` — toggle password protection
- `POST /api/projects/{id}/protection/regenerate` — new password
- `GET /api/projects/{id}/email` — fetch SMTP/reCAPTCHA settings + the auto-generated AI install prompt
- `PUT /api/projects/{id}/email` — save SMTP/reCAPTCHA settings (writes shared env vars + secrets)
- `POST /api/projects/{id}/email/test-smtp` — send a test email to verify SMTP credentials
- `POST /api/projects/{id}/deployments` — trigger / rollback (with `commit_sha` body field for rollback)
- `DELETE /api/projects/{id}/domains/{did}` — destructive
- `DELETE /api/projects/{id}/env/{key}` — destructive
- `GET /api/projects` — list (used to resolve project id from name)

- [ ] **Step 2: Write the file**

Create `.claude/skills/deployik-howto/api-actions.md`:

````markdown
# Deployik — API actions

Action mode for the goals in [click-paths.md](click-paths.md). Each entry has the endpoint, payload shape, safety tier, and the exact `helpers/deployik` invocation.

## Helper script

Always invoke the API via `./helpers/deployik` (relative to this skill's directory) — never paste tokens into raw curl. The wrapper:

- Reads `~/.config/deployik/config` (`DEPLOYIK_BASE_URL`, `DEPLOYIK_TOKEN`)
- Sends `Authorization: Bearer $DEPLOYIK_TOKEN`
- Prints `HTTP <status>` to stderr, body to stdout
- Exits non-zero on HTTP error
- Pretty-prints JSON if `jq` is installed

Usage:

```
deployik api <METHOD> <path> [json-body]
```

If `~/.config/deployik/config` is missing, stop and direct the user to create a token (Sidebar → **Access tokens** → **Create token**) before retrying.

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
  "organization_id": "(optional — defaults to personal workspace)"
}
```

**Invocation:**

```
deployik api POST /api/projects '{"name":"my-app","github_repo":"my-repo","github_owner":"my-org","branch":"main","framework":"nextjs","package_manager":"auto","output_directory":".next","build_command":"bun run build","install_command":"bun install","node_version":"22"}'
```

**Behavior:** creates the project, auto-domain, GitHub webhook (best-effort), and triggers an initial preview deployment.

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
````

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/deployik-howto/api-actions.md
git commit -m "feat(skill): api-actions.md with 6 endpoint recipes + safety tiers"
```

---

## Task 5: Write the `helpers/deployik` bash wrapper

**Files:**
- Create: `.claude/skills/deployik-howto/helpers/deployik`

**Why:** A small, dependency-light wrapper that:

1. Reads `~/.config/deployik/config` (KEY=value lines).
2. Sends Bearer auth via curl.
3. Prints HTTP status + pretty body.
4. Exits non-zero on HTTP error so Claude can detect failures programmatically.
5. Refuses to run if config is missing — prints the setup instructions.

Bash-only (no Python, no Node) keeps the helper portable across her machine and the dev's machine.

- [ ] **Step 1: Write the script**

Create `.claude/skills/deployik-howto/helpers/deployik`:

```bash
#!/usr/bin/env bash
# deployik — thin wrapper around the Deployik HTTP API.
# Reads ~/.config/deployik/config (KEY=value lines) and signs requests with
# DEPLOYIK_TOKEN as a Bearer token. Never inline tokens on the command line.
set -euo pipefail

CONFIG_FILE="${DEPLOYIK_CONFIG:-$HOME/.config/deployik/config}"

usage() {
  cat <<EOF
Usage: deployik api <METHOD> <path> [json-body]

Examples:
  deployik api GET  /api/projects
  deployik api POST /api/projects/abc/env '{"key":"FOO","value":"bar","environment":"preview"}'
  deployik api DELETE /api/projects/abc/domains/xyz

Reads $CONFIG_FILE for DEPLOYIK_BASE_URL and DEPLOYIK_TOKEN.
Set DEPLOYIK_CONFIG to override the path.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
  cat >&2 <<EOF
deployik: config file not found at $CONFIG_FILE

Create it with two lines:
  DEPLOYIK_BASE_URL=https://your-deployik-host
  DEPLOYIK_TOKEN=dpk_...

Generate a token in the Deployik dashboard sidebar: Access tokens → Create token.
EOF
  exit 2
fi

# shellcheck disable=SC1090
source "$CONFIG_FILE"

if [[ -z "${DEPLOYIK_BASE_URL:-}" || -z "${DEPLOYIK_TOKEN:-}" ]]; then
  echo "deployik: DEPLOYIK_BASE_URL or DEPLOYIK_TOKEN missing in $CONFIG_FILE" >&2
  exit 2
fi

if [[ "${1:-}" != "api" || $# -lt 3 ]]; then
  usage
  exit 2
fi

shift
method="$1"
path="$2"
body="${3:-}"

# Strip trailing slash from base, leading slash from path, then rejoin.
base="${DEPLOYIK_BASE_URL%/}"
path="/${path#/}"
url="$base$path"

curl_args=(
  --silent
  --show-error
  --write-out "\n__HTTP_STATUS__:%{http_code}\n"
  --request "$method"
  --header "Authorization: Bearer $DEPLOYIK_TOKEN"
  --header "Accept: application/json"
)

if [[ -n "$body" ]]; then
  curl_args+=(--header "Content-Type: application/json" --data "$body")
fi

raw=$(curl "${curl_args[@]}" "$url")
status=$(printf '%s\n' "$raw" | awk -F: '/^__HTTP_STATUS__:/ {print $2}' | tr -d '[:space:]')
body_out=$(printf '%s\n' "$raw" | sed '/^__HTTP_STATUS__:/d')

if command -v jq >/dev/null 2>&1 && [[ -n "$body_out" ]]; then
  if printf '%s' "$body_out" | jq . >/dev/null 2>&1; then
    body_out=$(printf '%s' "$body_out" | jq .)
  fi
fi

echo "HTTP $status" >&2
[[ -n "$body_out" ]] && printf '%s\n' "$body_out"

case "$status" in
  2*) exit 0 ;;
  *)  exit 1 ;;
esac
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x .claude/skills/deployik-howto/helpers/deployik`
Expected: file is now executable.

- [ ] **Step 3: Smoke test (no token required)**

Run: `.claude/skills/deployik-howto/helpers/deployik --help`
Expected: prints the usage block, exit 0.

Run: `.claude/skills/deployik-howto/helpers/deployik api GET /api/health`
Expected: prints "deployik: config file not found at …" and exit 2 (because no `~/.config/deployik/config` is set up yet).

- [ ] **Step 4: End-to-end test (requires Plan A merged + a token)**

> **If Plan A is not merged yet, skip to Step 5 — guide mode does not depend on this.**

Set up a temporary config and call the health endpoint:

```bash
mkdir -p ~/.config/deployik
cat > ~/.config/deployik/config <<EOF
DEPLOYIK_BASE_URL=http://localhost:8080
DEPLOYIK_TOKEN=$(your dpk_ token from the dashboard)
EOF
chmod 600 ~/.config/deployik/config

.claude/skills/deployik-howto/helpers/deployik api GET /api/projects
```

Expected: HTTP 200 + JSON list of projects.

Test the failure path:

```bash
DEPLOYIK_CONFIG=/nonexistent .claude/skills/deployik-howto/helpers/deployik api GET /api/projects
```

Expected: prints the "config file not found" message, exit 2.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/deployik-howto/helpers/deployik
git commit -m "feat(skill): helpers/deployik bash wrapper for Bearer-auth API calls"
```

---

## Task 6: Run scenarios with skill content present (GREEN)

**Files:**
- Modify: `.claude/skills/deployik-howto/SCENARIOS.md`

**Why:** Re-run S1–S8 from Task 2 with the now-complete skill (`SKILL.md` router + `click-paths.md` + `api-actions.md` + `helpers/deployik`). The skill should now produce the expected behavior for each scenario. Failures here drive the REFACTOR step in Task 7.

- [ ] **Step 1: Re-dispatch each scenario**

For each of S1–S8, dispatch a fresh subagent with the same prompt as in Task 2. The agent should now have access to the full skill.

For each, append a `### With skill (GREEN attempt)` subsection to the scenario in `SCENARIOS.md` with the verbatim response.

- [ ] **Step 2: Score each scenario**

In `SCENARIOS.md`, after each "With skill" block, add a **Verdict** line: `pass` / `fail` / `partial`, plus a one-sentence diagnosis if not pass.

A pass means:
- For S1, S2, S6: the agent produced the route + sidebar + steps from `click-paths.md` (or the API equivalent + payload preview from `api-actions.md`) without hallucination.
- For S3, S4: the agent stopped and required the typed confirmation / production confirmation before any action.
- For S5: the skill did NOT activate — the agent treated the prompt as a Go bug fix.
- For S7: the agent recognized the goal was outside v1, did not invent steps, and offered the user a sensible next step.
- For S8: the agent stopped on missing config and printed the setup instructions verbatim.

- [ ] **Step 3: Commit the GREEN observations**

```bash
git add .claude/skills/deployik-howto/SCENARIOS.md
git commit -m "test(skill): re-run pressure scenarios with full skill body"
```

---

## Task 7: REFACTOR — close loopholes

**Files:**
- Modify: any of `SKILL.md`, `click-paths.md`, `api-actions.md`, `helpers/deployik` based on Task 6 findings.

**Why:** Per writing-skills, the REFACTOR phase plugs new rationalizations the agent invented under pressure. We don't write fixes preemptively — only the ones the scenarios proved necessary.

- [ ] **Step 1: For every `fail` or `partial` verdict in `SCENARIOS.md`, write a fix line**

In `SCENARIOS.md`, append `### Fix` to the scenario and write one to three sentences naming exactly what changes in which file. Example:

```markdown
### Fix
S3: agent skipped typed confirmation when the user phrased the request casually
("just delete old.example.com"). Update SKILL.md safety table — clarify that
DELETE always requires typed confirmation regardless of phrasing. Add an explicit
"red flag" line: "If you find yourself thinking 'this is obviously what they want',
STOP and ask for the typed confirmation."
```

- [ ] **Step 2: Apply the fixes**

Edit the named files. Keep the changes minimal — close the specific loophole, don't expand scope. Common patterns observed in skill iteration:

- **Safety bypass under casual phrasing:** add an explicit "phrasing doesn't change the rule" line in the safety table.
- **Hallucinated route paths:** add a "If a route isn't in click-paths.md, say so — don't guess" instruction in the SKILL.md router section.
- **Invented endpoints:** add a "If a goal isn't in api-actions.md, say it's not in v1 — don't invent the endpoint" instruction.
- **Activation on codebase questions:** sharpen the SKILL.md "When NOT to use" section with concrete examples (file paths like `internal/...go`).
- **Production confirmation skipped:** add a more emphatic line — `**production writes always require an explicit yes/no, even for non-destructive verbs**`.

- [ ] **Step 3: Re-run only the previously failing scenarios**

For each fixed scenario, re-dispatch the subagent with the same prompt. Append a `### After fix` block with the response and a verdict.

If a fix caused regression in a previously-passing scenario, re-run that one too.

- [ ] **Step 4: Iterate until all scenarios pass**

If the second iteration still fails on a scenario, repeat steps 1–3. Don't ship with a `fail` or `partial` verdict — every loophole observed in testing must be closed before commit.

- [ ] **Step 5: Build the rationalization table**

Once all scenarios pass, distill the loopholes you closed into a small "Red flags" section at the bottom of `SKILL.md`:

```markdown
## Red flags — STOP and re-check

When you catch yourself thinking any of these, you're rationalizing past the safety rule:

- "This is obviously what they want" → still confirm; phrasing doesn't change the tier.
- "It's just a small change" → production is still production.
- "I remember this endpoint" → check `api-actions.md`. If it's not there, it's not in v1.
- "The user is in a hurry" → typed confirmation for destructive actions is non-negotiable.
- "I can guess the route" → if it's not in `click-paths.md`, say so. Don't make up paths.
```

- [ ] **Step 6: Commit**

```bash
git add .claude/skills/deployik-howto/
git commit -m "refactor(skill): close loopholes from pressure-scenario testing"
```

---

## Task 8: Remove the temporary scenarios file

**Files:**
- Delete: `.claude/skills/deployik-howto/SCENARIOS.md`

**Why:** `SCENARIOS.md` is test scaffolding, not part of the shipped skill. Keeping it would cost token budget when Claude loads the directory and could confuse future agents about which file is authoritative. Per writing-skills "Keep inline" guidance, only ship the files Claude needs to consult at runtime.

- [ ] **Step 1: Delete the file**

Run: `git rm .claude/skills/deployik-howto/SCENARIOS.md`
Expected: file is removed.

- [ ] **Step 2: Confirm what remains**

Run: `ls .claude/skills/deployik-howto/`
Expected output:

```
SKILL.md
api-actions.md
click-paths.md
helpers
```

And:

```
ls .claude/skills/deployik-howto/helpers/
```

Expected output: `deployik`

- [ ] **Step 3: Commit**

```bash
git commit -m "chore(skill): remove SCENARIOS.md test scaffolding"
```

---

## Task 9: Living docs + final verification

**Files:**
- Modify: `.claude/CLAUDE.md`

**Why:** Tell future Claude sessions that the skill exists and where it lives, so the per-project CLAUDE.md stays the index of truth.

- [ ] **Step 1: Update CLAUDE.md**

Edit `.claude/CLAUDE.md`. In `## Project Structure`, after the existing `web/src/...` block, add a new top-level entry:

```markdown
.claude/skills/
  deployik-howto/         User-facing dashboard help skill (project-scoped)
    SKILL.md              Router: triggers, when-to-use, guide vs action mode
    click-paths.md        Goal-indexed table → 6 v1 recipes (route + sidebar + click steps)
    api-actions.md        Endpoint catalog with safety tiers (read silent, mutate confirm, destructive typed-confirm)
    helpers/deployik      Bash wrapper for Bearer-auth API calls; reads ~/.config/deployik/config
```

In `## Design Decisions`, append a new bullet:

```markdown
- **`deployik-howto` skill is project-scoped, not global:** the skill lives at `.claude/skills/deployik-howto/` and is committed to the repo. UI changes to `web/src/pages/*` and skill updates ship in the same PR — no doc-vs-code drift across deploys. Action mode requires a Personal Access Token (see migration `017_api_tokens.sql`); guide mode works without one.
```

- [ ] **Step 2: Verify the skill activates as expected**

In a fresh Claude Code session in this repo, run a final smoke test by asking: `In Deployik, where do I add a custom domain?`

Expected: skill activates, agent quotes the route `/projects/{id}/settings/domains`, the sidebar breadcrumb, and the numbered steps from `click-paths.md#custom-domain`.

- [ ] **Step 3: Verify the negative case (activation guard)**

Ask: `Find the bug in internal/api/handlers/domains.go where Verify returns 500.`

Expected: skill does NOT activate. Agent treats it as a normal Go bug fix.

- [ ] **Step 4: Commit**

```bash
git add .claude/CLAUDE.md
git commit -m "docs: register deployik-howto skill in project structure"
```

---

## Self-review checklist results

**Spec coverage** — verified:
- `SKILL.md` (router + Tone section + offer-to-help footer policy) ✓ Task 1 + filled in Task 7
- `click-paths.md` with 7 v1 recipes (create-project, custom-domain, env-vars, auto-deploy, password-protection, contact-form-email, rollback), each with a "stuck on a step?" footer ✓ Task 3
- `api-actions.md` with endpoint catalog + safety tiers, including GET/PUT/POST `/email` and `/email/test-smtp` ✓ Task 4
- `helpers/deployik` bash wrapper reading `~/.config/deployik/config` ✓ Task 5
- RED-GREEN-REFACTOR cycle ✓ Tasks 2 (RED), 6 (GREEN), 7 (REFACTOR)
- Project-scoped skill location ✓ All tasks under `.claude/skills/deployik-howto/`
- Plan B independence: guide mode ships without Plan A; action-mode end-to-end test in Task 5 Step 4 explicitly gated on Plan A merge ✓
- Activation guard for codebase questions ✓ baked into the description in Task 1, validated in Task 9 Step 3
- Non-technical tone enforcement ✓ Tone section in SKILL.md sets the convention; every recipe ends with the "stuck?" footer; every external-system step explicitly tells the user they're leaving Deployik

**Type/path consistency:**
- Routes in `click-paths.md` (Task 3) match the routes in `web/src/app/app.tsx` (`/new`, `/projects/$id/settings/domains`, `/projects/$id/settings/env`, `/projects/$id/settings`, `/projects/$id/settings/protection`, `/projects/$id/deployments`).
- Sidebar paths in `click-paths.md` match `web/src/components/layout/AppSidebar.tsx`.
- Helper-script invocations in `api-actions.md` use the exact `deployik api <METHOD> <path> [body]` shape implemented in Task 5.
- Endpoints referenced in `api-actions.md` match `internal/api/router.go` and CLAUDE.md's API endpoint table.

**Placeholder scan** — no "TBD", no "fill in later", no "similar to above". Every task block contains the actual content the implementer needs.

**Open questions parked from design doc** (resolved here):
- *Token expiry default.* No expiry in v1 — handled in Plan A; this skill just reads `DEPLOYIK_TOKEN` and lets the API decide.
- *Email feature.* **Promoted into v1** as the seventh recipe (`contact-form-email`) per the user's 2026-04-26 ask. Covers Webglobe SMTP + reCAPTCHA v3 + the project's auto-generated AI install prompt. Scenario S7 now tests the recipe activates correctly.
- *DNS provider coverage.* `click-paths.md` covers Cloudflare, Namecheap, GoDaddy explicitly + a generic "Other registrar" catch-all. Expansion is doc-only and additive.
- *Helper script distribution.* Lives inside the skill at `helpers/deployik`. Claude invokes it via the relative path; no install step.
- *Production write confirmation strength.* Mutation-tier with explicit production flag. Destructive-tier (typed confirmation) reserved for DELETE and regenerate. Validated by S4 in Task 6.
- *Sidebar overlap with Plan A.* Plan A adds `/account/tokens`. Click-paths recipes mention "Sidebar → Access tokens" only inside the auth-missing prerequisites section in `SKILL.md` — not as a goal recipe (token management isn't in v1's seven goals).
- *Tone for non-technical users.* SKILL.md has an explicit Tone section. Every recipe ends with the "stuck on a step?" footer. External-system steps explicitly announce when the user is leaving Deployik.

---

## Execution notes

- Tasks 1, 3, 4, 5, 8, 9 are pure file authoring — fast, parallelizable in principle but it's worth keeping them sequential so Task 2's baseline scenarios reflect the absent body content, then Task 6 validates the additions in order.
- Task 2 and Task 6 dispatch subagents — count those as the most expensive steps. Plan to run them in a quiet conversation where each subagent's response can be reviewed in detail.
- Task 5 Step 4 (end-to-end helper test) is the only step that depends on Plan A. Skip it cleanly if Plan A isn't merged yet — the rest of the plan completes without it and guide mode ships independently.
