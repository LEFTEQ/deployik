export interface LocaleOption {
  code: string;
  name: string;
  region: string;
}

export const DEFAULT_SELECTED_LOCALES = ["cs", "en", "sk"] as const;

export const LOCALE_OPTIONS: LocaleOption[] = [
  { code: "cs", name: "Czech", region: "Czechia" },
  { code: "en", name: "English", region: "Global" },
  { code: "sk", name: "Slovak", region: "Slovakia" },
  { code: "de", name: "German", region: "Germany" },
  { code: "pl", name: "Polish", region: "Poland" },
  { code: "hu", name: "Hungarian", region: "Hungary" },
  { code: "fr", name: "French", region: "France" },
  { code: "es", name: "Spanish", region: "Spain" },
  { code: "it", name: "Italian", region: "Italy" },
  { code: "nl", name: "Dutch", region: "Netherlands" },
  { code: "pt", name: "Portuguese", region: "Portugal" },
  { code: "uk", name: "Ukrainian", region: "Ukraine" },
  { code: "ro", name: "Romanian", region: "Romania" },
  { code: "hr", name: "Croatian", region: "Croatia" },
  { code: "sl", name: "Slovenian", region: "Slovenia" },
  { code: "en-us", name: "English", region: "United States" },
  { code: "en-gb", name: "English", region: "United Kingdom" },
  { code: "de-at", name: "German", region: "Austria" },
  { code: "fr-ca", name: "French", region: "Canada" },
  { code: "es-mx", name: "Spanish", region: "Mexico" },
];

export function normalizeLocaleCode(value: string): string {
  const normalized = value.trim().toLowerCase().replace(/_/g, "-");
  if (!/^[a-z]{2,3}(-[a-z0-9]{2,8}){0,2}$/.test(normalized)) {
    return "";
  }
  return normalized;
}

export function getLocaleLabel(code: string): string {
  const known = LOCALE_OPTIONS.find((locale) => locale.code === code);
  return known ? `${known.name} (${known.code})` : code;
}

export function filterLocaleOptions(
  options: LocaleOption[],
  search: string,
): LocaleOption[] {
  const query = search.trim().toLowerCase();
  if (!query) return options;
  return options.filter((locale) =>
    [locale.code, locale.name, locale.region].some((value) =>
      value.toLowerCase().includes(query),
    ),
  );
}

export function addLocaleSelection(
  selectedLocales: string[],
  locale: string,
): string[] {
  const normalized = normalizeLocaleCode(locale);
  if (!normalized || selectedLocales.includes(normalized)) {
    return selectedLocales;
  }
  return [...selectedLocales, normalized];
}

export function toggleLocaleSelection({
  selectedLocales,
  defaultLocale,
  locale,
}: {
  selectedLocales: string[];
  defaultLocale: string;
  locale: string;
}): { selectedLocales: string[]; defaultLocale: string } {
  const normalized = normalizeLocaleCode(locale);
  if (!normalized) {
    return { selectedLocales, defaultLocale };
  }

  if (!selectedLocales.includes(normalized)) {
    return {
      selectedLocales: [...selectedLocales, normalized],
      defaultLocale,
    };
  }

  if (selectedLocales.length === 1) {
    return { selectedLocales, defaultLocale };
  }

  const nextLocales = selectedLocales.filter((code) => code !== normalized);
  return {
    selectedLocales: nextLocales,
    defaultLocale:
      defaultLocale === normalized
        ? nextLocales[0] ?? defaultLocale
        : defaultLocale,
  };
}

export function buildMultiLocalePrompt({
  projectName,
  githubOwner,
  githubRepo,
  rootDirectory,
  defaultLocale,
  selectedLocales,
}: {
  projectName: string;
  githubOwner: string;
  githubRepo: string;
  rootDirectory: string;
  defaultLocale: string;
  selectedLocales: string[];
}): string {
  const localeList = selectedLocales.join(", ");
  const messagesList = selectedLocales
    .map((locale) => `- messages/${locale}.json`)
    .join("\n");
  const root = rootDirectory.trim() || "Repository root";

  return [
    "You are working inside a deployed Next.js application. Add production-quality multi-locale support using next-intl.",
    "",
    "Project context:",
    `- Deployik project: ${projectName}`,
    `- Repository: ${githubOwner}/${githubRepo}`,
    `- Root directory: ${root}`,
    `- Default locale: ${defaultLocale}`,
    `- Supported locales: ${localeList}`,
    "",
    "Implementation requirements:",
    "1. Install and configure next-intl for the existing Next.js architecture. Preserve the current routing style and do not create a parallel app shell.",
    "2. Add locale routing with a middleware.ts file. Redirect unprefixed public routes to the default locale and keep API/static asset routes untouched.",
    "3. Create a typed locale configuration module that exports the locales array, defaultLocale, and helpers used by navigation and metadata.",
    "4. Add message dictionaries:",
    messagesList,
    "5. Wire the root layout/provider so translations are SSR-safe and work with App Router rendering.",
    "6. Migrate visible navigation, metadata, forms, validation messages, empty states, and primary CTAs to translation keys.",
    "7. Add a compact locale switcher that preserves the current pathname/search params when switching language.",
    "8. Keep SEO correct: localized metadata, canonical/alternate links, and stable hreflang values for every supported locale.",
    "9. Add or update tests for locale redirects, locale switcher behavior, and at least one translated page.",
    "10. Document how to add a new locale and how translators should update message files.",
    "",
    "Acceptance criteria:",
    "- Visiting / redirects to the default locale.",
    "- Visiting each supported locale renders translated navigation and page content.",
    "- Unknown locale prefixes return the app's existing not-found behavior.",
    "- API routes, _next assets, images, and public files are not intercepted by locale middleware.",
    "- The implementation is type-safe and does not hardcode locale strings outside the locale configuration module.",
  ].join("\n");
}
