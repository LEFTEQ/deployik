import { describe, expect, test } from "bun:test";

import {
  formatRelativeDateFrom,
  getPreferredEnvironmentDomain,
} from "./deployment-helpers";
import type { Domain } from "@/types/api";

function makeDomain(overrides: Partial<Domain> & Pick<Domain, "domain">): Domain {
  return {
    id: overrides.id ?? overrides.domain,
    project_id: overrides.project_id ?? "project-1",
    domain: overrides.domain,
    environment: overrides.environment ?? "preview",
    is_auto: overrides.is_auto ?? false,
    is_primary: overrides.is_primary ?? false,
    dns_verified: overrides.dns_verified ?? false,
    ssl_status: overrides.ssl_status ?? "pending",
    ssl_expires_at: overrides.ssl_expires_at ?? null,
    created_at: overrides.created_at ?? "2026-05-06T00:00:00.000Z",
  };
}

describe("deployment domain helpers", () => {
  test("chooses the explicit primary domain for the environment", () => {
    const domains = [
      makeDomain({
        domain: "preview.example.com",
        environment: "preview",
        is_auto: true,
      }),
      makeDomain({
        domain: "primary-preview.example.com",
        environment: "preview",
        is_primary: true,
      }),
    ];

    expect(getPreferredEnvironmentDomain(domains, "preview")?.domain).toBe(
      "primary-preview.example.com",
    );
  });

  test("falls back to the auto domain for preview deployments", () => {
    const domains = [
      makeDomain({
        domain: "custom-preview.example.com",
        environment: "preview",
      }),
      makeDomain({
        domain: "app-preview.example.com",
        environment: "preview",
        is_auto: true,
      }),
    ];

    expect(getPreferredEnvironmentDomain(domains, "preview")?.domain).toBe(
      "app-preview.example.com",
    );
  });

  test("falls back to the custom domain for production deployments", () => {
    const domains = [
      makeDomain({
        domain: "app-prod.example.com",
        environment: "production",
        is_auto: true,
      }),
      makeDomain({
        domain: "example.com",
        environment: "production",
      }),
    ];

    expect(getPreferredEnvironmentDomain(domains, "production")?.domain).toBe(
      "example.com",
    );
  });

  test("returns null when no domain exists for the environment", () => {
    const domains = [
      makeDomain({
        domain: "preview.example.com",
        environment: "preview",
        is_auto: true,
      }),
    ];

    expect(getPreferredEnvironmentDomain(domains, "production")).toBeNull();
  });
});

describe("deployment date helpers", () => {
  test("formats relative dates from a fixed base date", () => {
    expect(
      formatRelativeDateFrom(
        "2026-05-18T08:00:00.000Z",
        new Date("2026-05-18T10:00:00.000Z"),
      ),
    ).toBe("about 2 hours ago");
  });
});
