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
      framework: "node-api",
      host_network_access: true,
      data_volume_enabled: true,
      data_mount_path: "/data",
      start_command: "bun run start",
      health_path: "/api/health",
    });

    expect(payload.framework).toBe("node-api");
    expect(payload.host_network_access).toBe(true);
    expect(payload.data_volume_enabled).toBe(true);
    expect(payload.data_mount_path).toBe("/data");
    expect(payload.start_command).toBe("bun run start");
    expect(payload.health_path).toBe("/api/health");
  });

  test("supports the documented Dockerfile app payload shape", () => {
    const payload = buildCreateProjectPayload({
      ...baseArgs,
      name: "fleet",
      framework: "static",
      root_directory: "apps/fleet",
      output_directory: "",
      build_command: "",
      install_command: "",
      port: 8080,
      data_volume_enabled: true,
      data_mount_path: "/data",
    });

    expect(payload.framework).toBe("static");
    expect(payload.root_directory).toBe("apps/fleet");
    expect(payload.port).toBe(8080);
    expect(payload.data_volume_enabled).toBe(true);
    expect(payload.data_mount_path).toBe("/data");
  });
});
