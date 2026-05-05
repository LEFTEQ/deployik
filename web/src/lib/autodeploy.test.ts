import { describe, expect, test } from "bun:test";

import {
  resolveAutoDeploySourceBranch,
  shouldEnableProductionAutoDeploy,
} from "./autodeploy";

describe("autodeploy helpers", () => {
  test("resolves the selected branch before the repository default branch", () => {
    expect(resolveAutoDeploySourceBranch("develop", "main")).toBe("develop");
  });

  test("falls back to the repository default branch and then main", () => {
    expect(resolveAutoDeploySourceBranch("", "master")).toBe("master");
    expect(resolveAutoDeploySourceBranch(" ", "")).toBe("main");
  });

  test("only enables production auto-deploy when preview auto-build is enabled", () => {
    expect(shouldEnableProductionAutoDeploy(true, true)).toBe(true);
    expect(shouldEnableProductionAutoDeploy(false, true)).toBe(false);
    expect(shouldEnableProductionAutoDeploy(true, false)).toBe(false);
  });
});
