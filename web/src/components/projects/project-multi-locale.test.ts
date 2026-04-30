import { describe, expect, test } from "bun:test";

import {
  DEFAULT_SELECTED_LOCALES,
  buildMultiLocalePrompt,
  normalizeLocaleCode,
  toggleLocaleSelection,
} from "./project-multi-locale-utils";

describe("project multi locale utilities", () => {
  test("normalizes locale codes for supported language tags", () => {
    expect(normalizeLocaleCode(" CS ")).toBe("cs");
    expect(normalizeLocaleCode("en-US")).toBe("en-us");
    expect(normalizeLocaleCode("sk_SK")).toBe("sk-sk");
    expect(normalizeLocaleCode("not a locale")).toBe("");
  });

  test("keeps Czech, English, and Slovak selected by default", () => {
    expect(DEFAULT_SELECTED_LOCALES).toEqual(["cs", "en", "sk"]);
  });

  test("keeps the default locale valid when toggling locales", () => {
    const removedDefault = toggleLocaleSelection({
      selectedLocales: ["cs", "en", "sk"],
      defaultLocale: "cs",
      locale: "cs",
    });

    expect(removedDefault).toEqual({
      selectedLocales: ["en", "sk"],
      defaultLocale: "en",
    });

    const addedLocale = toggleLocaleSelection({
      selectedLocales: ["en", "sk"],
      defaultLocale: "en",
      locale: "de",
    });

    expect(addedLocale).toEqual({
      selectedLocales: ["en", "sk", "de"],
      defaultLocale: "en",
    });
  });

  test("builds a next-intl prompt with selected locales and project context", () => {
    const prompt = buildMultiLocalePrompt({
      projectName: "my-nextjs-app",
      githubOwner: "LEFTEQ",
      githubRepo: "lovinka-deployik",
      rootDirectory: "apps/site",
      defaultLocale: "cs",
      selectedLocales: ["cs", "en", "sk"],
    });

    expect(prompt).toContain("my-nextjs-app");
    expect(prompt).toContain("LEFTEQ/lovinka-deployik");
    expect(prompt).toContain("Root directory: apps/site");
    expect(prompt).toContain("Default locale: cs");
    expect(prompt).toContain("Supported locales: cs, en, sk");
    expect(prompt).toContain("next-intl");
    expect(prompt).toContain("middleware.ts");
  });
});
