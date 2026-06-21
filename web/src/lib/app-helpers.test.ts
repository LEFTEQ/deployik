import { describe, expect, test } from "bun:test";
import { APP_STATUS_META, MEMBER_STATUS_META, RELEASE_STATUS_META } from "./app-helpers";

describe("app status metadata", () => {
  test("combined status covers the five backend values", () => {
    expect(Object.keys(APP_STATUS_META).sort()).toEqual(
      ["degraded", "deploying", "down", "healthy", "none"],
    );
  });
  test("member status covers the seven backend values", () => {
    expect(Object.keys(MEMBER_STATUS_META).sort()).toEqual(
      ["degraded", "deploying", "down", "failed", "healthy", "none", "unknown"],
    );
  });
  test("release status covers the four backend values", () => {
    expect(Object.keys(RELEASE_STATUS_META).sort()).toEqual(
      ["failed", "pending", "rolled_back", "succeeded"],
    );
  });
});
