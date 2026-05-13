import { describe, expect, test } from "bun:test";

import {
  RESOURCE_TIER_META,
  RESOURCE_TIER_ORDER,
  formatTierMemory,
} from "./deployment-helpers";
import type { ResourceTier } from "@/types/api";

describe("resource tier metadata", () => {
  test("covers exactly the four backend tiers", () => {
    expect(Object.keys(RESOURCE_TIER_META).sort()).toEqual(
      ["large", "medium", "nano", "small"],
    );
  });

  test("RESOURCE_TIER_ORDER lists every tier from smallest to largest", () => {
    expect(RESOURCE_TIER_ORDER).toEqual(["nano", "small", "medium", "large"]);
    let prevMem = 0;
    for (const t of RESOURCE_TIER_ORDER) {
      const mem = RESOURCE_TIER_META[t].memoryMB;
      expect(mem).toBeGreaterThan(prevMem);
      prevMem = mem;
    }
  });

  test("Small tier matches the legacy hardcoded 512 MB / 1.0 CPU", () => {
    // Migration 021 backfills existing projects to 'small'. If this drifts,
    // existing containers would change behavior on the next deploy.
    expect(RESOURCE_TIER_META.small.memoryMB).toBe(512);
    expect(RESOURCE_TIER_META.small.cpuCores).toBe(1.0);
  });

  test("build memory always exceeds runtime memory (builds peak higher)", () => {
    for (const t of RESOURCE_TIER_ORDER) {
      const meta = RESOURCE_TIER_META[t];
      expect(meta.buildMemoryMB).toBeGreaterThan(meta.memoryMB);
    }
  });

  test("build memory fits inside the 6 GB BuildKit cap", () => {
    for (const t of RESOURCE_TIER_ORDER) {
      expect(RESOURCE_TIER_META[t].buildMemoryMB).toBeLessThanOrEqual(6144);
    }
  });

  test("formatTierMemory pretty-prints GB / MB units", () => {
    expect(formatTierMemory(256)).toBe("256 MB");
    expect(formatTierMemory(512)).toBe("512 MB");
    expect(formatTierMemory(1024)).toBe("1 GB");
    expect(formatTierMemory(2048)).toBe("2 GB");
    expect(formatTierMemory(1536)).toBe("1536 MB");
  });

  test("Type covers every metadata key (compile-time only)", () => {
    // This test exists for runtime guard, but the satisfies clause in the
    // module also enforces this at compile time.
    const t: ResourceTier = "small";
    expect(RESOURCE_TIER_META[t].label).toBe("Small");
  });
});
