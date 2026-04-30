import { useMemo, useState } from "react";

import { useQuery } from "@tanstack/react-query";
import {
  Check,
  Languages,
  type LucideIcon,
  Plus,
  Search,
  Sparkles,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { CodePanel } from "@/components/ui/code-panel";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { LoadingState } from "@/components/ui/spinner";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { cn } from "@/lib/utils";
import {
  DEFAULT_SELECTED_LOCALES,
  LOCALE_OPTIONS,
  addLocaleSelection,
  buildMultiLocalePrompt,
  filterLocaleOptions,
  getLocaleLabel,
  normalizeLocaleCode,
  toggleLocaleSelection,
} from "@/components/projects/project-multi-locale-utils";

export function ProjectMultiLocaleTab({ projectId }: { projectId: string }) {
  const { data: project, isLoading, error } = useQuery({
    queryKey: queryKeys.project(projectId),
    queryFn: () => api.getProject(projectId),
  });

  if (isLoading) {
    return (
      <LoadingState
        title="Loading locale setup..."
        description="Preparing project framework details and locale defaults."
        className="min-h-[340px]"
      />
    );
  }

  if (error || !project) {
    return (
      <div
        className="rounded-lg border border-destructive/30 bg-destructive/10 px-5 py-4 text-sm text-destructive-foreground"
        data-testid="multi-locale-error"
      >
        {error instanceof Error ? error.message : "Unknown locale setup error."}
      </div>
    );
  }

  const isNextProject = project.framework === "nextjs";

  return (
    <div className="flex flex-col gap-8" data-testid="multi-locale-page">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-lg font-semibold tracking-tight text-foreground">
              Multi Locale
            </h2>
            <Badge variant={isNextProject ? "secondary" : "outline"}>
              {isNextProject ? "Next.js optimized" : "Guide only"}
            </Badge>
          </div>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            Plan supported languages, choose the default locale, and generate a
            precise install prompt for multi-language app routing.
          </p>
        </div>
        <div
          className="flex items-center gap-2 rounded-lg border bg-card px-3 py-2 text-sm text-muted-foreground"
          data-testid="multi-locale-framework-notice"
        >
          <Languages className="size-4" />
          <span>
            Framework:{" "}
            <span className="font-medium text-foreground">
              {project.framework}
            </span>
          </span>
        </div>
      </div>

      {isNextProject ? (
        <NextLocaleWorkflow
          projectName={project.name}
          githubOwner={project.github_owner}
          githubRepo={project.github_repo}
          rootDirectory={project.root_directory}
        />
      ) : (
        <GenericLocaleGuide framework={project.framework} />
      )}
    </div>
  );
}

function NextLocaleWorkflow({
  projectName,
  githubOwner,
  githubRepo,
  rootDirectory,
}: {
  projectName: string;
  githubOwner: string;
  githubRepo: string;
  rootDirectory: string;
}) {
  const [selectedLocales, setSelectedLocales] = useState<string[]>([
    ...DEFAULT_SELECTED_LOCALES,
  ]);
  const [defaultLocale, setDefaultLocale] = useState<string>(
    DEFAULT_SELECTED_LOCALES[0],
  );
  const [search, setSearch] = useState("");
  const [customLocale, setCustomLocale] = useState("");

  const filteredLocales = useMemo(
    () => filterLocaleOptions(LOCALE_OPTIONS, search),
    [search],
  );

  const prompt = useMemo(
    () =>
      buildMultiLocalePrompt({
        projectName,
        githubOwner,
        githubRepo,
        rootDirectory,
        defaultLocale,
        selectedLocales,
      }),
    [
      defaultLocale,
      githubOwner,
      githubRepo,
      projectName,
      rootDirectory,
      selectedLocales,
    ],
  );

  const addCustomLocale = () => {
    const normalized = normalizeLocaleCode(customLocale);
    if (!normalized) {
      toast.error("Use a locale code such as cs, en, sk, or en-us");
      return;
    }

    const nextLocales = addLocaleSelection(selectedLocales, normalized);
    setSelectedLocales(nextLocales);
    setCustomLocale("");
    if (nextLocales.length === selectedLocales.length) {
      toast.message(`${normalized} is already selected`);
      return;
    }
    toast.success(`${normalized} added`);
  };

  const copyPrompt = async () => {
    try {
      await navigator.clipboard.writeText(prompt);
      toast.success("Multi Locale prompt copied");
    } catch {
      toast.error("Couldn't copy prompt");
    }
  };

  return (
    <div
      className="flex flex-col gap-6"
      data-testid="multi-locale-nextjs-workflow"
    >
      <div className="grid gap-4 md:grid-cols-3">
        <SummaryCard
          icon={Languages}
          label="Locales"
          value={String(selectedLocales.length)}
          detail={selectedLocales.join(", ")}
        />
        <SummaryCard
          icon={Check}
          label="Default"
          value={defaultLocale}
          detail={getLocaleLabel(defaultLocale)}
        />
        <SummaryCard
          icon={Sparkles}
          label="Install"
          value="next-intl"
          detail="App Router friendly prompt"
        />
      </div>

      <Card>
        <CardHeader>
          <div>
            <CardTitle>Language Picker</CardTitle>
            <CardDescription>
              Choose the locales this project should support before copying the
              generated install prompt.
            </CardDescription>
          </div>
          <CardAction>
            <Badge variant="outline" data-testid="multi-locale-selected-count">
              {selectedLocales.length} selected
            </Badge>
          </CardAction>
        </CardHeader>
        <CardContent>
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_280px]">
            <div className="flex flex-col gap-4">
              <div className="flex flex-col gap-2">
                <Label htmlFor="multi-locale-search">Search languages</Label>
                <div className="relative">
                  <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    id="multi-locale-search"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    className="pl-9"
                    placeholder="Search by language, region, or code"
                    data-testid="multi-locale-language-search"
                  />
                </div>
              </div>

              <ScrollArea className="h-72 rounded-lg border">
                <div className="grid gap-1 p-2 sm:grid-cols-2">
                  {filteredLocales.map((locale) => {
                    const checked = selectedLocales.includes(locale.code);
                    const checkboxId = `locale-${locale.code}`;
                    return (
                      <label
                        key={locale.code}
                        htmlFor={checkboxId}
                        className={cn(
                          "flex cursor-pointer items-start gap-3 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
                          checked && "bg-accent/70",
                        )}
                        data-testid={`multi-locale-option-${locale.code}`}
                      >
                        <Checkbox
                          id={checkboxId}
                          checked={checked}
                          onCheckedChange={() => {
                            const next = toggleLocaleSelection({
                              selectedLocales,
                              defaultLocale,
                              locale: locale.code,
                            });
                            setSelectedLocales(next.selectedLocales);
                            setDefaultLocale(next.defaultLocale);
                          }}
                          aria-label={`${locale.name} ${locale.code}`}
                          data-testid={`multi-locale-checkbox-${locale.code}`}
                        />
                        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                          <span className="font-medium text-foreground">
                            {locale.name}
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {locale.region} - {locale.code}
                          </span>
                        </span>
                      </label>
                    );
                  })}
                </div>
              </ScrollArea>
            </div>

            <div className="flex flex-col gap-5">
              <div className="flex flex-col gap-2">
                <Label htmlFor="multi-locale-default">Default locale</Label>
                <Select
                  value={defaultLocale}
                  onValueChange={setDefaultLocale}
                >
                  <SelectTrigger
                    id="multi-locale-default"
                    className="w-full"
                    data-testid="multi-locale-default-select"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {selectedLocales.map((locale) => (
                        <SelectItem key={locale} value={locale}>
                          {getLocaleLabel(locale)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>

              <Separator />

              <form
                className="flex flex-col gap-3"
                onSubmit={(event) => {
                  event.preventDefault();
                  addCustomLocale();
                }}
              >
                <div className="flex flex-col gap-2">
                  <Label htmlFor="multi-locale-custom">Custom locale</Label>
                  <Input
                    id="multi-locale-custom"
                    value={customLocale}
                    onChange={(event) => setCustomLocale(event.target.value)}
                    placeholder="en-us"
                    data-testid="multi-locale-custom-input"
                  />
                </div>
                <Button
                  type="submit"
                  variant="outline"
                  size="sm"
                  data-testid="multi-locale-add-custom"
                >
                  <Plus data-icon="inline-start" />
                  Add Locale
                </Button>
              </form>

              <Separator />

              <div className="flex flex-col gap-2">
                <p className="text-sm font-medium">Selected languages</p>
                <div className="flex flex-wrap gap-2">
                  {selectedLocales.map((locale) => (
                    <Badge
                      key={locale}
                      variant={
                        locale === defaultLocale ? "secondary" : "outline"
                      }
                      data-testid={`multi-locale-selected-${locale}`}
                    >
                      {locale}
                    </Badge>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <div data-testid="multi-locale-prompt-panel">
        <CodePanel
          title="AI Install Prompt"
          description="Paste this into Codex, Claude, or ChatGPT inside the target Next.js repository."
          value={prompt}
          onCopy={copyPrompt}
          copyButtonTestId="multi-locale-copy-prompt"
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Verify After Install</CardTitle>
          <CardDescription>
            The deployed app should prove locale routing, translation coverage,
            and SEO behavior before the integration is considered done.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 text-sm text-muted-foreground md:grid-cols-3">
            <VerificationItem
              label="Routing"
              value="/ redirects to default locale"
            />
            <VerificationItem
              label="Switcher"
              value="Language switch keeps path and query"
            />
            <VerificationItem
              label="SEO"
              value="Canonical and hreflang links are localized"
            />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function GenericLocaleGuide({ framework }: { framework: string }) {
  return (
    <Card data-testid="multi-locale-generic-guide">
      <CardHeader>
        <div>
          <CardTitle>Framework-Neutral Locale Guide</CardTitle>
          <CardDescription>
            Multi Locale automation is optimized for Next.js. For this{" "}
            {framework} project, use the same language plan but wire routing and
            message loading through the framework's native conventions.
          </CardDescription>
        </div>
        <CardAction>
          <Badge variant="outline">Next.js only automation</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="grid gap-4 md:grid-cols-3">
          <VerificationItem label="Locales" value="Start with cs, en, sk" />
          <VerificationItem
            label="Messages"
            value="Keep one dictionary per locale"
          />
          <VerificationItem
            label="Routing"
            value="Preserve public URLs when switching"
          />
        </div>
      </CardContent>
    </Card>
  );
}

function SummaryCard({
  icon: Icon,
  label,
  value,
  detail,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  detail: string;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Icon className="size-4" />
          {label}
        </div>
        <CardTitle className="text-2xl">{value}</CardTitle>
        <CardDescription>{detail}</CardDescription>
      </CardHeader>
    </Card>
  );
}

function VerificationItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1 rounded-lg border bg-muted/20 px-4 py-3">
      <span className="text-xs font-medium uppercase text-muted-foreground">
        {label}
      </span>
      <span className="text-sm text-foreground">{value}</span>
    </div>
  );
}
