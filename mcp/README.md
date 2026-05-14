# @lovinka/deployik-mcp

MCP server for [Deployik](https://github.com/lefteq/lovinka-deployik). Lets any MCP-aware AI drive Deployik end-to-end — create projects, set secrets, trigger deployments, debug failed builds, manage domains — without touching the dashboard.

## Install (one-shot config)

Add to your MCP client config (Claude Desktop, Claude Code, etc.):

```json
{
  "mcpServers": {
    "deployik": {
      "command": "npx",
      "args": ["-y", "@lovinka/deployik-mcp"],
      "env": {
        "DEPLOYIK_URL": "https://deployik.example.com",
        "DEPLOYIK_TOKEN": "dpk_..."
      }
    }
  }
}
```

Get a token at **Account → Access tokens** in Deployik. The token is shown once on creation; copy it then.

For a VPN-only deployment, point `DEPLOYIK_URL` at any reachable host (`http://10.x.x.x:8080`, `https://deployik.internal`, etc.).

## What it does

- **~32 thin tools** — one per Deployik HTTP endpoint (projects, deployments, env vars, secrets, domains, auto-build, password protection, volumes, analytics, email, workspaces, tokens, GitHub).
- **12 workflow tools** — `setup_project_from_repo`, `deploy_project`, `set_secret`, `tail_latest_logs`, `debug_failed_deployment`, `get_project_health`, `init_in_repo`, `whats_my_url`, `whats_broken`, `redeploy`, and more.
- **Bundled knowledge** — Deployik's how-to recipes ship as MCP prompts (`deployik_recipe_*`), plus three tools: `list_recipes`, `get_recipe(topic)`, and **`find_help(question)`** which routes free-form English ("where do I set the Stripe key for the live site?") to the right recipe automatically.
- **Repo binding** — first call inside a git repo with a unique-match `origin` auto-writes `.deployik.json` (committed, just project + workspace + schema URL) and gitignores the private `.deployik/` directory. Self-healing `.gitignore`.
- **Tiered safety** — destructive operations require `confirm: true`; production-touching operations also require `confirm_name: <project>`. Every destructive call is logged to `.deployik/audit.log`.
- **Loose English** — `prod` / `live` / `staging` / `dev` / `both` / `everywhere` all normalize to the right scope automatically.

## Also: install the Deployik skill into Claude Code

For Claude Code users who want the recipes available via `/skills` (independent of any MCP tool call):

```bash
npx -y @lovinka/deployik-mcp install-skill
```

This copies `SKILL.md` + `click-paths.md` + `api-actions.md` into `~/.claude/skills/deployik-howto/`. Add `--yes` to skip the confirmation prompt. Run it any time you upgrade the package to refresh the recipes.

## Local development

```bash
cd mcp
bun install
bun run build
bun run inspect    # opens MCP Inspector against the local binary
```

Test against a local Deployik dev server with `DEPLOYIK_URL=http://127.0.0.1:8080` and a dev-mode PAT.

## Files written on the host

Project ↔ repo state is split into two layers — **public** (commit it) and
**private** (gitignored, per developer):

```
<repo-root>/
├── .deployik.json    PUBLIC, commit this. Just { project, workspace, $schema }.
│                     Teammates pulling your repo immediately know which
│                     Deployik project this folder deploys to.
└── .deployik/        PRIVATE. Auto-added to .gitignore (and re-added on every
                      MCP call if a teammate clobbers the .gitignore line).
    ├── cache.json    Project + workspace list (1h TTL).
    ├── token         Optional token fallback (mode 0600) — only used if
                      DEPLOYIK_TOKEN env var is unset.
    └── audit.log     Append-only ledger of destructive calls (secret values
                      redacted automatically).
```

**Automatic binding**: the first time you run any tool inside a git repo whose
`origin` remote uniquely matches one Deployik project, the MCP writes
`.deployik.json` silently — no explicit setup needed. If multiple projects
deploy the same repo (monorepos with several Deployik apps), the MCP returns a
friendly "which one?" with the candidate slugs.

**Manual binding** (also fine): "deployik bind this repo to acme-app" → the
AI calls `init_in_repo({ project: "acme-app" })` and writes `.deployik.json`.
