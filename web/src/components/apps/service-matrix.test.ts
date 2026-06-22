import { describe, expect, test } from "bun:test";

import { buildMatrixRows } from "./service-matrix";
import type { AppHealthMember, Project } from "@/types/api";

function member(id: string): AppHealthMember {
  return {
    project: { id, name: id } as Project,
    live_status: "healthy",
  } as AppHealthMember;
}

describe("buildMatrixRows", () => {
  test("merges members by project id across environments", () => {
    const rows = buildMatrixRows([member("a"), member("b")], [member("a")]);
    expect(rows.map((r) => r.project.id)).toEqual(["a", "b"]);

    const a = rows.find((r) => r.project.id === "a");
    expect(a?.preview).toBeDefined();
    expect(a?.production).toBeDefined();

    const b = rows.find((r) => r.project.id === "b");
    expect(b?.preview).toBeDefined();
    expect(b?.production).toBeUndefined();
  });

  test("appends production-only members after preview members", () => {
    const rows = buildMatrixRows([member("a")], [member("a"), member("c")]);
    expect(rows.map((r) => r.project.id)).toEqual(["a", "c"]);

    const c = rows.find((r) => r.project.id === "c");
    expect(c?.preview).toBeUndefined();
    expect(c?.production).toBeDefined();
  });

  test("preserves preview member order", () => {
    const rows = buildMatrixRows([member("x"), member("y"), member("z")], []);
    expect(rows.map((r) => r.project.id)).toEqual(["x", "y", "z"]);
  });

  test("handles empty / missing inputs", () => {
    expect(buildMatrixRows()).toEqual([]);
    expect(buildMatrixRows([], [])).toEqual([]);
  });
});
