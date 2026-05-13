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
- **9 workflow tools** — `setup_project_from_repo`, `deploy_project`, `set_secret`, `tail_latest_logs`, `debug_failed_deployment`, `get_project_health`, `init_in_repo`, and more.
- **Bundled knowledge** — Deployik's how-to recipes ship as MCP prompts (`deployik_recipe_*`) and a `get_recipe(topic)` tool, so a fresh AI session can self-onboard.
- **Repo binding** — `init_in_repo` writes a `.deployik/binding.json` (gitignored) so the AI knows which Deployik project this folder maps to without asking.
- **Tiered safety** — destructive operations require `confirm: true`; production-touching operations also require `confirm_name: <project>`. Every destructive call is logged to `.deployik/audit.log`.

## Local development

```bash
cd mcp
bun install
bun run build
bun run inspect    # opens MCP Inspector against the local binary
```

Test against a local Deployik dev server with `DEPLOYIK_URL=http://127.0.0.1:8080` and a dev-mode PAT.

## Files written on the host

When the AI calls `init_in_repo` from inside a repo:

```
.deployik/
├── binding.json     project + workspace mapping for this repo
├── cache.json       cached project/workspace list (1h TTL)
├── token            optional token fallback (mode 0600)
└── audit.log        destructive-call ledger
```

`.gitignore` is auto-appended with `.deployik/` if missing.
