import { existsSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, test } from "bun:test";

import { installAll } from "./install";
import { installSkillResult } from "./install-skill";

const originalFetch = globalThis.fetch;
const originalDeployikUrl = process.env.DEPLOYIK_URL;
const originalDeployikToken = process.env.DEPLOYIK_TOKEN;

afterEach(() => {
  globalThis.fetch = originalFetch;
  restoreEnv("DEPLOYIK_URL", originalDeployikUrl);
  restoreEnv("DEPLOYIK_TOKEN", originalDeployikToken);
});

describe("installAll", () => {
  test("reuses an existing MCP token instead of requiring onboarding again", async () => {
    const cwd = mkdtempSync(join(tmpdir(), "deployik-mcp-install-"));
    try {
      writeFileSync(
        join(cwd, ".mcp.json"),
        `${JSON.stringify(
          {
            mcpServers: {
              deployik: {
                type: "stdio",
                command: "npx",
                args: ["-y", "@lovinka/deployik-mcp"],
                env: {
                  DEPLOYIK_URL: "https://existing.deployik.test",
                  DEPLOYIK_TOKEN: "dpk_existing",
                },
              },
            },
          },
          null,
          2,
        )}\n`,
      );
      delete process.env.DEPLOYIK_URL;
      delete process.env.DEPLOYIK_TOKEN;
      const requests: string[] = [];
      globalThis.fetch = (async (input: RequestInfo | URL) => {
        requests.push(String(input));
        if (String(input).endsWith("/api/health")) {
          return Response.json({ status: "ok" });
        }
        if (String(input).endsWith("/api/auth/me")) {
          return Response.json({ username: "lukas", role: "admin" });
        }
        return Response.json({}, { status: 404 });
      }) as typeof fetch;

      const code = await installAll({
        scope: "local",
        yes: true,
        cwd,
        skipSkill: true,
      });

      const config = JSON.parse(readFileSync(join(cwd, ".mcp.json"), "utf8"));
      expect(code).toBe(0);
      expect(requests).toEqual([
        "https://existing.deployik.test/api/health",
        "https://existing.deployik.test/api/auth/me",
      ]);
      expect(config.mcpServers.deployik.env.DEPLOYIK_TOKEN).toBe(
        "dpk_existing",
      );
    } finally {
      rmSync(cwd, { recursive: true, force: true });
    }
  });
});

describe("installSkillResult", () => {
  test("installs the API helper script with executable mode", async () => {
    const cwd = mkdtempSync(join(tmpdir(), "deployik-skill-install-"));
    try {
      const result = await installSkillResult({
        scope: "local",
        yes: true,
        cwd,
      });

      const helperPath = join(cwd, ".claude", "skills", "deployik-howto", "helpers", "deployik");
      expect(result?.files).toContain("helpers/deployik");
      expect(existsSync(helperPath)).toBe(true);
      expect(statSync(helperPath).mode & 0o111).not.toBe(0);
    } finally {
      rmSync(cwd, { recursive: true, force: true });
    }
  });
});

function restoreEnv(key: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[key];
  } else {
    process.env[key] = value;
  }
}
