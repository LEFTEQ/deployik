import type { ReactNode } from "react";

import { RotateCcw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export type FrameworkPreset =
  | "nextjs"
  | "vite"
  | "astro"
  | "static"
  | "node-api";
export type PackageManagerPreset = "auto" | "bun" | "pnpm" | "npm" | "yarn";

export interface BuildSettingsValues {
  framework: string;
  packageManager: string;
  rootDirectory: string;
  outputDirectory: string;
  installCommand: string;
  buildCommand: string;
  nodeVersion: string;
  port: number;
  startCommand: string;
  healthPath: string;
}

// Default container listen port. Deployik's generated runtimes (Next.js
// standalone + `serve`) both bind to this; user-provided Dockerfiles override
// via this same field when they listen on a different port.
export const DEFAULT_PROJECT_PORT = 3000;

type FrameworkOption = {
  value: FrameworkPreset;
  label: string;
  description: string;
};

type PackageManagerOption = {
  value: PackageManagerPreset;
  label: string;
  description: string;
};

const FRAMEWORK_OPTIONS: FrameworkOption[] = [
  {
    value: "nextjs",
    label: "Next.js",
    description: "Standalone server build with `.next` output.",
  },
  {
    value: "vite",
    label: "Vite / SPA",
    description: "Static `dist` output served on port 3000.",
  },
  {
    value: "astro",
    label: "Astro",
    description: "Static `dist` output served on port 3000.",
  },
  {
    value: "static",
    label: "Static Site",
    description: "Generic static build output with a configurable folder.",
  },
  {
    value: "node-api",
    label: "Node API",
    description:
      "NestJS / Express / Hono / Fastify on Node.js with a configurable start command.",
  },
];

const PACKAGE_MANAGER_OPTIONS: PackageManagerOption[] = [
  {
    value: "auto",
    label: "Auto Detect",
    description:
      "Keep current compatibility behavior and prefer repo lockfiles.",
  },
  {
    value: "bun",
    label: "Bun",
    description: "Use Bun commands and install Bun in the build container.",
  },
  {
    value: "pnpm",
    label: "pnpm",
    description: "Use pnpm with Corepack enabled in the build container.",
  },
  {
    value: "npm",
    label: "npm",
    description: "Use npm / package-lock based installs.",
  },
  {
    value: "yarn",
    label: "Yarn",
    description: "Use Yarn with Corepack enabled in the build container.",
  },
];

export function normalizeFrameworkPreset(
  framework?: string | null,
): FrameworkPreset {
  switch ((framework ?? "").trim().toLowerCase()) {
    case "vite":
      return "vite";
    case "astro":
      return "astro";
    case "static":
      return "static";
    case "node-api":
      return "node-api";
    case "nextjs":
    default:
      return "nextjs";
  }
}

export function normalizePackageManagerPreset(
  packageManager?: string | null,
): PackageManagerPreset {
  switch ((packageManager ?? "").trim().toLowerCase()) {
    case "bun":
      return "bun";
    case "pnpm":
      return "pnpm";
    case "npm":
      return "npm";
    case "yarn":
      return "yarn";
    case "auto":
    default:
      return "auto";
  }
}

function defaultInstallCommandForPackageManager(
  packageManager: PackageManagerPreset,
): string {
  switch (packageManager) {
    case "pnpm":
      return "pnpm install --frozen-lockfile";
    case "npm":
      return "npm ci";
    case "yarn":
      return "yarn install --frozen-lockfile";
    case "auto":
    case "bun":
    default:
      return "bun install --frozen-lockfile";
  }
}

function defaultBuildCommandForPackageManager(
  packageManager: PackageManagerPreset,
): string {
  switch (packageManager) {
    case "pnpm":
      return "pnpm run build";
    case "npm":
      return "npm run build";
    case "yarn":
      return "yarn build";
    case "auto":
    case "bun":
    default:
      return "bun run build";
  }
}

export function getFrameworkDefaults(
  framework?: string | null,
  packageManager?: string | null,
): BuildSettingsValues {
  const normalized = normalizeFrameworkPreset(framework);
  const normalizedPackageManager =
    normalizePackageManagerPreset(packageManager);

  return {
    framework: normalized,
    packageManager: normalizedPackageManager,
    rootDirectory: "",
    outputDirectory: normalized === "nextjs" ? ".next" : "dist",
    installCommand: defaultInstallCommandForPackageManager(
      normalizedPackageManager,
    ),
    buildCommand: defaultBuildCommandForPackageManager(
      normalizedPackageManager,
    ),
    nodeVersion: "22",
    port: DEFAULT_PROJECT_PORT,
    startCommand: normalized === "node-api" ? "node dist/main.js" : "",
    healthPath: normalized === "node-api" ? "/health" : "",
  };
}

export function syncBuildSettingsWithFramework(
  values: BuildSettingsValues,
  nextFramework: string,
): BuildSettingsValues {
  const currentDefaults = getFrameworkDefaults(
    values.framework,
    values.packageManager,
  );
  const nextDefaults = getFrameworkDefaults(
    nextFramework,
    values.packageManager,
  );

  return {
    ...values,
    framework: nextDefaults.framework,
    outputDirectory:
      values.outputDirectory.trim() === "" ||
      values.outputDirectory === currentDefaults.outputDirectory
        ? nextDefaults.outputDirectory
        : values.outputDirectory,
    installCommand:
      values.installCommand.trim() === "" ||
      values.installCommand === currentDefaults.installCommand
        ? nextDefaults.installCommand
        : values.installCommand,
    buildCommand:
      values.buildCommand.trim() === "" ||
      values.buildCommand === currentDefaults.buildCommand
        ? nextDefaults.buildCommand
        : values.buildCommand,
    nodeVersion:
      values.nodeVersion.trim() === "" ||
      values.nodeVersion === currentDefaults.nodeVersion
        ? nextDefaults.nodeVersion
        : values.nodeVersion,
    port: values.port,
    startCommand:
      values.startCommand.trim() === "" ||
      values.startCommand === currentDefaults.startCommand
        ? nextDefaults.startCommand
        : values.startCommand,
    healthPath:
      values.healthPath.trim() === "" ||
      values.healthPath === currentDefaults.healthPath
        ? nextDefaults.healthPath
        : values.healthPath,
  };
}

function isKnownInstallDefault(command: string): boolean {
  const trimmed = command.trim();
  return PACKAGE_MANAGER_OPTIONS.some(
    (option) =>
      trimmed === defaultInstallCommandForPackageManager(option.value),
  );
}

function isKnownBuildDefault(command: string): boolean {
  const trimmed = command.trim();
  return PACKAGE_MANAGER_OPTIONS.some(
    (option) => trimmed === defaultBuildCommandForPackageManager(option.value),
  );
}

export function syncBuildSettingsWithPackageManager(
  values: BuildSettingsValues,
  nextPackageManager: string,
): BuildSettingsValues {
  const currentPackageManager = normalizePackageManagerPreset(
    values.packageManager,
  );
  const normalizedNextPackageManager =
    normalizePackageManagerPreset(nextPackageManager);
  const currentDefaults = getFrameworkDefaults(
    values.framework,
    currentPackageManager,
  );
  const nextDefaults = getFrameworkDefaults(
    values.framework,
    normalizedNextPackageManager,
  );
  const shouldReplaceInstallCommand =
    values.installCommand.trim() === "" ||
    values.installCommand === currentDefaults.installCommand ||
    isKnownInstallDefault(values.installCommand);
  const shouldReplaceBuildCommand =
    values.buildCommand.trim() === "" ||
    values.buildCommand === currentDefaults.buildCommand ||
    isKnownBuildDefault(values.buildCommand);

  return {
    ...values,
    packageManager: normalizedNextPackageManager,
    installCommand: shouldReplaceInstallCommand
      ? nextDefaults.installCommand
      : values.installCommand,
    buildCommand: shouldReplaceBuildCommand
      ? nextDefaults.buildCommand
      : values.buildCommand,
  };
}

export function formatFrameworkLabel(framework?: string | null): string {
  return (
    FRAMEWORK_OPTIONS.find(
      (option) => option.value === normalizeFrameworkPreset(framework),
    )?.label ?? "Next.js"
  );
}

type BuildSettingsFieldsProps = {
  value: BuildSettingsValues;
  onChange: (nextValue: BuildSettingsValues) => void;
  footer?: ReactNode;
};

export function BuildSettingsFields({
  value,
  onChange,
  footer,
}: BuildSettingsFieldsProps) {
  const defaults = getFrameworkDefaults(value.framework, value.packageManager);
  const selectedFramework = normalizeFrameworkPreset(value.framework);
  const selectedPackageManager = normalizePackageManagerPreset(
    value.packageManager,
  );
  const selectedFrameworkMeta =
    FRAMEWORK_OPTIONS.find((option) => option.value === selectedFramework) ??
    FRAMEWORK_OPTIONS[0]!;
  const selectedPackageManagerMeta =
    PACKAGE_MANAGER_OPTIONS.find(
      (option) => option.value === selectedPackageManager,
    ) ?? PACKAGE_MANAGER_OPTIONS[0]!;

  const patch = (partial: Partial<BuildSettingsValues>) =>
    onChange({ ...value, ...partial });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3 rounded-2xl border border-white/10 bg-black/10 p-4">
        <div className="space-y-1">
          <p className="text-sm font-medium text-foreground">
            {selectedFrameworkMeta.label}
          </p>
          <p className="text-xs text-muted-foreground">
            {selectedFrameworkMeta.description}
          </p>
          <p className="text-xs text-muted-foreground">
            {selectedPackageManagerMeta.label}:{" "}
            {selectedPackageManagerMeta.description}
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() =>
            onChange({
              ...value,
              framework: defaults.framework,
              packageManager: defaults.packageManager,
              outputDirectory: defaults.outputDirectory,
              installCommand: defaults.installCommand,
              buildCommand: defaults.buildCommand,
              nodeVersion: defaults.nodeVersion,
              port: DEFAULT_PROJECT_PORT,
              startCommand: defaults.startCommand,
              healthPath: defaults.healthPath,
            })
          }
        >
          <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
          Reset Defaults
        </Button>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label>Framework Preset</Label>
          <Select
            value={selectedFramework}
            onValueChange={(nextFramework) =>
              onChange(syncBuildSettingsWithFramework(value, nextFramework))
            }
          >
            <SelectTrigger className="w-full md:w-fit">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FRAMEWORK_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label>Root Directory</Label>
          <Input
            value={value.rootDirectory}
            onChange={(event) => patch({ rootDirectory: event.target.value })}
            placeholder="apps/web"
          />
          <p className="text-xs text-muted-foreground">
            Leave empty to build from the repository root.
          </p>
        </div>

        <div className="space-y-2">
          <Label>Package Manager</Label>
          <Select
            value={selectedPackageManager}
            onValueChange={(nextPackageManager) =>
              onChange(
                syncBuildSettingsWithPackageManager(value, nextPackageManager),
              )
            }
          >
            <SelectTrigger className="w-full md:w-fit">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PACKAGE_MANAGER_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label>Install Command</Label>
          <Input
            value={value.installCommand}
            onChange={(event) => patch({ installCommand: event.target.value })}
            placeholder={defaults.installCommand}
          />
        </div>

        <div className="space-y-2">
          <Label>Build Command</Label>
          <Input
            value={value.buildCommand}
            onChange={(event) => patch({ buildCommand: event.target.value })}
            placeholder={defaults.buildCommand}
          />
          {selectedPackageManager === "auto" ? (
            <p className="text-xs text-muted-foreground">
              Auto detect keeps backward compatibility and may prefer repo
              lockfiles if these commands are still on defaults.
            </p>
          ) : null}
        </div>

        <div className="space-y-2">
          <Label>Output Directory</Label>
          <Input
            value={value.outputDirectory}
            onChange={(event) => patch({ outputDirectory: event.target.value })}
            placeholder={defaults.outputDirectory}
          />
          <p className="text-xs text-muted-foreground">
            Next.js uses this as the base for standalone/static assets. Static
            presets serve this folder directly.
          </p>
        </div>

        <div className="space-y-2">
          <Label>Node.js Version</Label>
          <Select
            value={value.nodeVersion}
            onValueChange={(nodeVersion) => patch({ nodeVersion })}
          >
            <SelectTrigger className="w-full md:w-fit">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="22">Node.js 22 (LTS)</SelectItem>
              <SelectItem value="20">Node.js 20</SelectItem>
              <SelectItem value="18">Node.js 18</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label>Container Port</Label>
          <Input
            type="number"
            min={1}
            max={65535}
            value={value.port || ""}
            onChange={(event) => {
              const raw = event.target.value.trim();
              patch({
                port:
                  raw === ""
                    ? DEFAULT_PROJECT_PORT
                    : Number.parseInt(raw, 10) || DEFAULT_PROJECT_PORT,
              });
            }}
            placeholder={String(DEFAULT_PROJECT_PORT)}
          />
          <p className="text-xs text-muted-foreground">
            The port your container listens on. Deployik's generated runtimes
            use {DEFAULT_PROJECT_PORT}; change this when your own Dockerfile
            serves on a different port (e.g. nginx on 80, Flask on 5000).
          </p>
        </div>

        {selectedFramework === "node-api" ? (
          <div className="space-y-2">
            <Label>Start Command</Label>
            <Input
              value={value.startCommand}
              onChange={(event) => patch({ startCommand: event.target.value })}
              placeholder={defaults.startCommand}
            />
            <p className="text-xs text-muted-foreground">
              How the container starts. Chain a migration (e.g.{" "}
              <code>prisma migrate deploy &amp;&amp; node dist/main.js</code>)
              when you need one.
            </p>
          </div>
        ) : null}

        {selectedFramework === "node-api" ? (
          <div className="space-y-2">
            <Label>Health Check Path</Label>
            <Input
              value={value.healthPath}
              onChange={(event) => patch({ healthPath: event.target.value })}
              placeholder={defaults.healthPath}
            />
            <p className="text-xs text-muted-foreground">
              Path the container's HEALTHCHECK probes. Common:{" "}
              <code>/health</code>, <code>/healthz</code>,{" "}
              <code>/api/health</code>.
            </p>
          </div>
        ) : null}
      </div>

      <p className="text-xs text-muted-foreground">
        Deployik uses these settings when it generates the runtime image. A
        custom `Dockerfile` in the repo still takes precedence.
      </p>

      {footer}
    </div>
  );
}
