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

export type FrameworkPreset = "nextjs" | "vite" | "astro" | "static";

export interface BuildSettingsValues {
  framework: string;
  rootDirectory: string;
  outputDirectory: string;
  installCommand: string;
  buildCommand: string;
  nodeVersion: string;
}

type FrameworkOption = {
  value: FrameworkPreset;
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
    case "nextjs":
    default:
      return "nextjs";
  }
}

export function getFrameworkDefaults(
  framework?: string | null,
): BuildSettingsValues {
  const normalized = normalizeFrameworkPreset(framework);

  return {
    framework: normalized,
    rootDirectory: "",
    outputDirectory: normalized === "nextjs" ? ".next" : "dist",
    installCommand: "bun install",
    buildCommand: "bun run build",
    nodeVersion: "22",
  };
}

export function syncBuildSettingsWithFramework(
  values: BuildSettingsValues,
  nextFramework: string,
): BuildSettingsValues {
  const currentDefaults = getFrameworkDefaults(values.framework);
  const nextDefaults = getFrameworkDefaults(nextFramework);

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
  const defaults = getFrameworkDefaults(value.framework);
  const selectedFramework = normalizeFrameworkPreset(value.framework);
  const selectedFrameworkMeta =
    FRAMEWORK_OPTIONS.find((option) => option.value === selectedFramework) ??
    FRAMEWORK_OPTIONS[0]!;

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
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() =>
            onChange({
              ...value,
              framework: defaults.framework,
              outputDirectory: defaults.outputDirectory,
              installCommand: defaults.installCommand,
              buildCommand: defaults.buildCommand,
              nodeVersion: defaults.nodeVersion,
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
            <SelectTrigger>
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
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="22">Node.js 22 (LTS)</SelectItem>
              <SelectItem value="20">Node.js 20</SelectItem>
              <SelectItem value="18">Node.js 18</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        Deployik uses these settings when it generates the runtime image. A
        custom `Dockerfile` in the repo still takes precedence.
      </p>

      {footer}
    </div>
  );
}
