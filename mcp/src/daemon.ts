// HTTP daemon entrypoint.
//
// Runs ONE long-lived process bound to 127.0.0.1, so every Claude Code
// window/session shares the same server instead of each spawning its own
// stdio child. Wired up via `deployik-mcp install --daemon`, which writes
// a launchd plist and points Claude's MCP config at this URL.
//
// Stateless transport — the SDK requires a fresh transport per request in
// stateless mode (reusing one throws "Stateless transport cannot be reused").
// So each POST /mcp instantiates a new McpServer + transport pair. The build
// is in-memory only (no I/O), takes sub-millisecond, and avoids the session
// bookkeeping a stateful map would require. Identity comes from the daemon's
// process env, not per-session.

import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { buildServer } from "./server.js";
import { ConfigError } from "./config/env.js";
import { VERSION } from "./version.js";

const DEFAULT_PORT = 8788;
const DEFAULT_HOST = "127.0.0.1";
const MCP_PATH = "/mcp";
const HEALTH_PATH = "/healthz";

export async function startDaemon(): Promise<void> {
  const port = parseInt(process.env.DEPLOYIK_DAEMON_PORT ?? "", 10) || DEFAULT_PORT;
  const host = process.env.DEPLOYIK_DAEMON_HOST?.trim() || DEFAULT_HOST;

  // Validate config up front so a bad token / missing URL fails the launchd
  // bootstrap step (visible in the install output) instead of throwing on
  // every request.
  buildServer(process.env, process.cwd(), { mode: "http" });

  const httpServer = createServer((req, res) => {
    // Wrap in a promise so async errors land in the .catch — Node's HTTP
    // request callback type is synchronous void, async throws otherwise
    // become unhandled rejections.
    void handleRequest(req, res).catch((err: unknown) => {
      const message = err instanceof Error ? err.message : String(err);
      if (!res.headersSent) {
        sendJson(res, 500, { error: "internal", message });
      } else {
        try { res.end(); } catch { /* socket already torn down */ }
      }
    });
  });

  async function handleRequest(req: IncomingMessage, res: ServerResponse): Promise<void> {
    // Localhost-only daemon, but cheap sanity check anyway: refuse anything
    // not sourced from this machine. Catches stray docker bridges or a future
    // misconfigured listener pointing at 0.0.0.0.
    const remote = req.socket.remoteAddress ?? "";
    if (!isLocal(remote)) {
      sendJson(res, 403, { error: "forbidden", reason: `non-local remote: ${remote}` });
      return;
    }

    const url = new URL(req.url ?? "/", `http://${host}:${port}`);
    if (url.pathname === HEALTH_PATH) {
      sendJson(res, 200, { ok: true, version: VERSION });
      return;
    }
    if (url.pathname !== MCP_PATH) {
      sendJson(res, 404, { error: "not_found", path: url.pathname });
      return;
    }

    // Stateless: fresh server + transport per request. The whole thing is
    // in-memory wiring (no network calls during buildServer), so the cost
    // is the tool-registration loop — still sub-millisecond in practice.
    const { server } = buildServer(process.env, process.cwd(), { mode: "http" });
    const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined });
    try {
      await server.connect(transport);
      await transport.handleRequest(req, res);
    } finally {
      transport.close().catch(() => { /* best-effort */ });
      server.close().catch(() => { /* best-effort */ });
    }
  }

  await new Promise<void>((resolve, reject) => {
    httpServer.once("error", reject);
    httpServer.listen(port, host, () => {
      httpServer.off("error", reject);
      resolve();
    });
  });

  process.stderr.write(`deployik-mcp ${VERSION} daemon listening on http://${host}:${port}${MCP_PATH}\n`);

  const shutdown = (signal: string): void => {
    process.stderr.write(`deployik-mcp daemon: received ${signal}, shutting down\n`);
    httpServer.close();
    process.exit(0);
  };
  process.once("SIGINT", () => shutdown("SIGINT"));
  process.once("SIGTERM", () => shutdown("SIGTERM"));
  process.once("SIGHUP", () => shutdown("SIGHUP"));
}

function sendJson(res: ServerResponse, status: number, body: unknown): void {
  res.statusCode = status;
  res.setHeader("Content-Type", "application/json");
  res.end(JSON.stringify(body));
}

function isLocal(addr: string): boolean {
  // Strip IPv6-mapped-v4 prefix so "::ffff:127.0.0.1" compares cleanly.
  const a = addr.replace(/^::ffff:/, "");
  return a === "127.0.0.1" || a === "::1" || a === "localhost";
}

// Allow `node dist/daemon.js` to start directly without going through index.ts.
// `import.meta.url` check makes the file safe to import as a library too.
const invokedDirectly = import.meta.url === `file://${process.argv[1]}`;
if (invokedDirectly) {
  startDaemon().catch((err: unknown) => {
    if (err instanceof ConfigError) {
      process.stderr.write(`deployik-mcp daemon: ${err.message}\n`);
      process.exit(2);
    }
    process.stderr.write(`deployik-mcp daemon: ${(err as Error).message}\n`);
    process.exit(1);
  });
}
