---
name: deployik-howto
description: Use when a user asks how to do something in the Deployik dashboard — connecting a GitHub repo, deploying a Dockerfile/Go/long-running app, custom domains, environment variables or secrets, auto-deploy on push, password protection, sending email from a contact form (Webglobe SMTP + reCAPTCHA v3), or rolling back a deployment — or asks Claude to perform one of those actions for them. Triggers include "how do I…", "where do I click…", "I want my own domain", "I want my contact form to send emails", "I want to make X work", "deploy my repo", "deploy this Dockerfile", "deploy my Go app", and "just do X for me" phrased against Deployik. Do NOT activate for questions about Deployik's source code (Go handlers, React pages, migrations) — those are codebase tasks, not dashboard tasks.
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

## Critical build facts

- Deployik supports user-provided Dockerfiles. There is no special `framework: docker` value.
- If a `Dockerfile` exists in the repo root, Deployik builds it as-is with repo root as the Docker context.
- If `root_directory` is set and a `Dockerfile` exists inside that folder, Deployik builds that Dockerfile with `root_directory` as the Docker context.
- For Dockerfile, Go, SQLite, or other long-running non-frontend apps, use `framework: static` as the neutral preset, set `root_directory` to the Dockerfile folder, and set `port` to the port the container listens on.
- The build/install/output fields are for generated Dockerfiles. A user Dockerfile controls its own build and start command.

## When NOT to use

- Questions about Deployik's source code, build pipeline internals, or how a Go handler/React page is implemented. Those are not dashboard tasks.
- Self-hosting / VPS / nginx / SSL provider questions — those are infrastructure/hosting questions for a different audience.

## How to choose the mode

Read the user's phrasing:

- *"How do I…"*, *"Where do I click…"*, *"I want to learn how to…"* → **guide mode** → see [click-paths.md](click-paths.md).
- *"Do X for me"*, *"Add this env var"*, *"Trigger a deploy"*, imperative phrasing → **action mode** → see [api-actions.md](api-actions.md).

If ambiguous, ask warmly: "Want me to walk you through the dashboard, or do it for you?"

## Action mode prerequisites

Action mode requires a Personal Access Token, but it may already be available through the Deployik MCP server. Prefer Deployik MCP tools when they are available in the current session (`list_projects`, `create_project`, `deploy_project`, `get_recipe`, etc.). Do not stop just because `~/.config/deployik/config` is missing if MCP Deployik tool calls work.

If no Deployik MCP tools are available, use the bundled `helpers/deployik` script. If `~/.config/deployik/config` doesn't exist or is missing `DEPLOYIK_BASE_URL` / `DEPLOYIK_TOKEN`, stop and explain in friendly tone:

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

Use Deployik MCP tools first. If they are unavailable, invoke the HTTP API via `helpers/deployik` (bundled with this skill). Never hand-roll curl with the token inline — keeps the token out of shell history.
