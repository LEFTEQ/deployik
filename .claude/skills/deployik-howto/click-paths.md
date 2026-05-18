# Deployik — Where to click

Eight recipes for the most common things a user wants to do in the Deployik dashboard. If the user's goal isn't in the table, ask them warmly to rephrase, or check whether their goal is actually outside Deployik's scope.

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
| I want a Postgres database for my app | [#attach-postgres](#attach-postgres) |
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
   - **Group** — choose the dashboard group/tab this project should live in.
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

**Route:** `/projects/$id/integrations/email`
**Sidebar:** Project → **Integrations** (expand) → **Email**

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

1. In the project sidebar, expand **Integrations** and click **Email**.
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

## attach-postgres

**Goal:** Get a real Postgres database for your app, per environment (preview / production), with credentials wired into the runtime automatically.

**Route:** `/projects/$id/services`
**Sidebar:** Project → **Services**

**What this gives you:**
- A dedicated Postgres container (a "sidecar") that runs next to your app, one per environment.
- Standard environment variables (`DATABASE_URL` + discrete `PGHOST` / `PGPORT` / `PGDATABASE` / `PGUSER` / `PGPASSWORD`) injected into the deployed app automatically — your code just reads them.
- A 16xxx loopback port on the VPS so you can connect from your laptop via an SSH tunnel for migrations / inspection.
- The database **persists across deploys** — the volume isn't touched by a new deployment.

**Steps:**

1. *(Optional, brand-new project only)* On the **New project** page, scroll to **Attach Postgres database**. Toggle it on for preview / production. Skip the rest of this recipe — it's done.
2. *(Existing project)* Click **Services** in the project sidebar.
3. Two cards: **Preview** and **Production**. Each starts as **Not attached** with an **Attach Postgres** button.
4. Click **Attach Postgres** on the environment you want. Deployik records the row, generates a strong random password, and assigns a unique 16xxx host port. The card flips to **Attached · pending** — the container itself isn't running yet.
5. Trigger any deploy for that environment (push a commit, or click **Redeploy** on Overview). The build pipeline's `EnsureServices` hook starts the postgres container, waits for it to be healthy, then starts your app with the env vars injected. The card shows **running** once it's up.
6. On the Services card, click **Show credentials** to reveal the password and the **SSH tunnel command**. Copy the SSH command, run it on your laptop — it forwards `127.0.0.1:15432` on your machine to the postgres in the VPS. Connect with `psql postgres://...@127.0.0.1:15432/...` or any client.
7. To restart the container without touching data, click **Restart**. To rotate the password, click **Regenerate password** — the new password is shown once; the running container keeps the old password until the next deploy/restart, so trigger a redeploy right after.

**Gotcha — Reset wipes everything:** the **Reset** button drops the volume and starts an empty database. Use it only for preview when you want a clean slate. On production it requires you to type `<project>-production` to confirm. There is no undo.

**Gotcha — project rename is blocked:** while a service is attached, the project name is frozen (renaming would orphan the volume). Detach first, rename, re-attach (and re-import data) if you really need a new name.

**Gotcha — detach deletes data:** clicking **Detach** stops the container AND removes the named volume. There is no undo. Take a `pg_dump` first if you want to keep the data.

**Stuck on any of these steps? Tell me which one and I'll walk through it with you.** I can also attach Postgres for you via the API and tail the container logs to confirm it came up clean — just ask.

**API equivalent:** [api-actions.md#attach-postgres](api-actions.md#attach-postgres)

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
