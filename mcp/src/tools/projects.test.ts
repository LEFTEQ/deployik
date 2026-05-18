import { describe, expect, test } from "bun:test";

import { buildCreateProjectPayload } from "./projects";

const baseArgs = {
  name: "docs-site",
  github_owner: "LEFTEQ",
  github_repo: "docs",
  branch: "main",
  framework: "nextjs" as const,
  package_manager: "bun" as const,
  root_directory: "",
  output_directory: ".next",
  build_command: "bun run build",
  install_command: "bun install",
  node_version: "22",
  port: 3000,
  auto_build_enabled: true,
  auto_production_enabled: false,
};

describe("create_project payload", () => {
  test("maps group_id to the API organization_id field", () => {
    const payload = buildCreateProjectPayload({
      ...baseArgs,
      group_id: "grp_123",
    });

    expect(payload.organization_id).toBe("grp_123");
  });

  test("keeps organization_id backward compatible", () => {
    const payload = buildCreateProjectPayload({
      ...baseArgs,
      organization_id: "org_legacy",
    });

    expect(payload.organization_id).toBe("org_legacy");
  });

  test("includes dashboard runtime fields when provided", () => {
    const payload = buildCreateProjectPayload({
      ...baseArgs,
      start_command: "bun run start",
      health_path: "/api/health",
    });

    expect(payload.start_command).toBe("bun run start");
    expect(payload.health_path).toBe("/api/health");
  });
});
