import { describe, expect, test } from "bun:test";

import { checkSafety } from "./safety";

describe("safety checks", () => {
  test("requires confirm_name for destructive tools when an expected name is set", () => {
    const context = {
      toolName: "delete_group",
      tier: "destructive" as const,
      expectedName: "Client Sites",
      impact: { group: "Client Sites" },
    };

    expect(
      checkSafety(context, { confirm: true, confirm_name: "Wrong" }).proceed,
    ).toBe(false);
    expect(
      checkSafety(context, {
        confirm: true,
        confirm_name: "Client Sites",
      }).proceed,
    ).toBe(true);
  });
});
