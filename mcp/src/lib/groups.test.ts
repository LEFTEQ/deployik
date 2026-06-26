import { describe, expect, test } from "bun:test";

import { findGroupBySelector } from "./groups";
import type { Group } from "../client/types";

const groups: Group[] = [
  {
    id: "grp_personal",
    name: "lefteq Personal",
    slug: "lefteq-personal",
    is_default: true,
    membership_role: "owner",
    project_count: 2,
    display_order: 1,
    created_at: "2026-05-18T08:00:00.000Z",
    updated_at: "2026-05-18T08:00:00.000Z",
  },
  {
    id: "grp_clients",
    name: "Client Sites",
    slug: "client-sites",
    is_default: false,
    membership_role: "member",
    project_count: 4,
    display_order: 2,
    created_at: "2026-05-18T08:00:00.000Z",
    updated_at: "2026-05-18T08:00:00.000Z",
  },
];

describe("group selector helpers", () => {
  test("matches a dashboard group by id, slug, or name", () => {
    expect(findGroupBySelector(groups, "grp_clients")?.id).toBe("grp_clients");
    expect(findGroupBySelector(groups, "client-sites")?.id).toBe("grp_clients");
    expect(findGroupBySelector(groups, "client sites")?.id).toBe("grp_clients");
  });

  test("returns undefined for unknown groups", () => {
    expect(findGroupBySelector(groups, "missing")).toBeUndefined();
  });
});
