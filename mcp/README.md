# @lovinka/deployik-mcp

MCP server for [Deployik](https://github.com/lefteq/lovinka-deployik). Lets any MCP-aware AI drive Deployik end-to-end — create projects, set secrets, trigger deployments, debug failed builds, manage domains — without touching the dashboard.

## Install (one-shot, no JSON editing)

```bash
npx -y @lovinka/deployik-mcp install
```

Prompts for your Deployik URL and Personal Access Token, then:

- Registers the `deployik` MCP server in `~/.claude.json` (Claude Code) **and** the Claude Desktop config (if installed) so every project + every window can use it.
- Copies the Deployik how-to recipes into `~/.claude/skills/deployik-howto/` so `/skills` surfaces them.
- Backs up any pre-existing config to `<path>.bak.<timestamp>` before merging.

Restart Claude Code / Claude Desktop afterwards.

### Scopes

```bash
# Global (recommended) — once installed, available everywhere
npx -y @lovinka/deployik-mcp install --global

# Local — writes <cwd>/.mcp.json + <cwd>/.claude/skills/
# MCP only fires when Claude is opened in this folder
npx -y @lovinka/deployik-mcp install --local
```

### Non-interactive

```bash
npx -y @lovinka/deployik-mcp install \
  --yes \
  --url=https://deployik.example.com \
  --token=dpk_xxx
```

Or set `DEPLOYIK_URL` / `DEPLOYIK_TOKEN` env vars and pass `--yes`.

### Granular subcommands

| Command | What it does |
|---|---|
| `install` | MCP registration + skill files (default) |
| `install --daemon` | Long-lived launchd daemon — one HTTP process for every Claude window (macOS only, see below) |
| `install-mcp` | MCP registration only |
| `install-skill` | Skill files only |
| `uninstall` | Removes the `deployik` MCP entry from every Claude config |
| `uninstall --daemon` | Stops + removes the launchd daemon and clears the HTTP entry from Claude configs |
| `daemon` | Runs the HTTP daemon in the foreground (for testing) |

## Daemon mode (one process, every Claude window)

By default each Claude Code window spawns its own stdio child for every configured MCP. After a few open windows you can easily have 10+ idle `node` processes eating ~100 MB each. The daemon mode collapses this to a single long-lived HTTP MCP server bound to `127.0.0.1:8788`.

```bash
npx -y @lovinka/deployik-mcp install --daemon --token=dpk_xxx
```

What it does:

- Writes a launchd plist at `~/Library/LaunchAgents/com.lovinka.deployik-mcp.plist` with `KeepAlive=true`, `RunAtLoad=true`, and `DEPLOYIK_URL` / `DEPLOYIK_TOKEN` in `EnvironmentVariables` (file mode 0600 — token only readable by your user).
- Stages the runtime into `~/.deployik-mcp/runtime/` so launchd can read it on every macOS regardless of TCC (Transparency, Consent, Control) restrictions on `~/Documents`, `~/Desktop`, etc.
- Runs `launchctl bootstrap gui/$UID <plist>` to start the daemon immediately.
- Rewrites the `deployik` entry in `~/.claude.json` (and the Claude Desktop config) from a stdio command to `{ "type": "http", "url": "http://127.0.0.1:8788/mcp" }`.

After install, **restart any open Claude windows** so they pick up the HTTP entry. From then on, opening N windows still uses **one** daemon process.

Tools that need the client's filesystem (`init_in_repo`, `show_binding`) are skipped in HTTP mode since the daemon has no per-repo context. Project resolution still works via explicit `project_id` / `slug` / single-project workspace.

Logs: `~/Library/Logs/deployik-mcp.{out,err}.log`.

To go back to per-window stdio: `npx -y @lovinka/deployik-mcp uninstall --daemon`, then re-run `install` without `--daemon`.

> Linux note: launchd is macOS-only. On Linux, run the daemon under `systemd --user` pointing at `node <prefix>/lib/node_modules/@lovinka/deployik-mcp/dist/daemon.js` and add the matching HTTP entry to your client config manually.

### Manual install (if you prefer to edit JSON yourself)

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

Get a token at **Account → Access tokens** in Deployik. The token is shown once on creation; copy it then. For a VPN-only deployment, point `DEPLOYIK_URL` at any reachable host (`http://10.x.x.x:8080`, `https://deployik.internal`, etc.).

## What it does

- **~50 thin tools** — one per Deployik HTTP endpoint (projects, deployments, env vars, secrets, domains, auto-build, password protection, volumes, analytics, email, dashboard groups, tokens, GitHub).
- **App bundles** — group several projects into one **app** and operate on it as a unit: `list_apps`, `create_app`, `get_app_health`, `update_app` (rename / toggle `deploy_ordered`), `delete_app`, `add_project_to_app`, `remove_project_from_app`; app-level env/secrets (`list_app_env_vars` / `set_app_env_var` / `delete_app_env_var` + secret variants, inherited by every member); and coordinated deploys: `deploy_app` (ordered, health-gated, fire-and-forget), `deploy_app_and_wait` (waits for the release outcome), `list_app_releases`, `rollback_app`. Member projects also gain `build_filter_enabled` / `watch_paths` (monorepo fan-out filtering) and `deploy_order` on create/update.
- **12 workflow tools** — `setup_project_from_repo`, `deploy_project`, `set_secret`, `tail_latest_logs`, `debug_failed_deployment`, `get_project_health`, `init_in_repo`, `whats_my_url`, `whats_broken`, `redeploy`, and more.
- **Dockerfile-aware project creation** — generated presets cover Next.js, Vite, Astro, static sites, and Node APIs; user-provided Dockerfiles are supported by choosing `framework: "static"`, setting `root_directory` to the Dockerfile folder, and setting `port` to the container listen port.
- **Bundled knowledge** — Deployik's how-to recipes ship as MCP prompts (`deployik_recipe_*`), plus three tools: `list_recipes`, `get_recipe(topic)`, and **`find_help(question)`** which routes free-form English ("where do I set the Stripe key for the live site?") to the right recipe automatically.
- **Repo binding** — first call inside a git repo with a unique-match `origin` auto-writes `.deployik.json` (committed, just project + dashboard group + schema URL) and gitignores the private `.deployik/` directory. Self-healing `.gitignore`.
- **Tiered safety** — destructive operations require `confirm: true`; production-touching operations also require `confirm_name: <project>`. Every destructive call is logged to `.deployik/audit.log`.
- **Loose English** — `prod` / `live` / `staging` / `dev` / `both` / `everywhere` all normalize to the right scope automatically.


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
    ├── cache.json    Project + dashboard group list (1h TTL).
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
