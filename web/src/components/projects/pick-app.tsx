import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  Layers,
  Package,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { MonorepoApp, RepoInspection } from "@/types/api";

interface PickAppProps {
  inspection: RepoInspection;
  repoFullName: string;
  onSelectApp: (app: MonorepoApp) => void;
  onSetManually: () => void;
  onBack: () => void;
}

const FRAMEWORK_LABELS: Record<string, string> = {
  nextjs: "Next.js",
  vite: "Vite",
  astro: "Astro",
  static: "Static",
};

const TOOLING_LABELS: Record<string, string> = {
  turborepo: "Turborepo",
  nx: "Nx",
};

export function PickApp({
  inspection,
  repoFullName,
  onSelectApp,
  onSetManually,
  onBack,
}: PickAppProps) {
  const buildableCount = inspection.apps.filter((a) => a.buildable).length;

  const subtitle = (() => {
    if (inspection.apps.length === 0) {
      return "No apps were detected in this repository.";
    }
    if (!inspection.is_monorepo && inspection.apps.length === 1) {
      return "This repository was detected as a single project. Select it to pre-fill build settings on the next step.";
    }
    const count = inspection.apps.length;
    const buildableNote =
      buildableCount < count
        ? ` (${count - buildableCount} without a build script)`
        : "";
    return `Found ${count} app${count !== 1 ? "s" : ""} inside this monorepo. Pick one to pre-fill the build settings on the next step.${buildableNote}`;
  })();

  return (
    <div className="mx-auto max-w-2xl p-6 pb-[calc(1.5rem+env(safe-area-inset-bottom))]">
      {/* Back button */}
      <button
        onClick={onBack}
        className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to repository selection
      </button>

      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            Select app to deploy
          </h1>
          <p className="mt-1 font-mono text-sm text-muted-foreground">
            {repoFullName}
          </p>
        </div>

        {/* Tooling + package manager badges */}
        <div className="flex flex-wrap items-center gap-1.5 pt-1">
          {inspection.package_manager &&
            inspection.package_manager !== "auto" && (
              <Badge variant="outline" className="gap-1 px-2 py-0.5 text-xs">
                <Layers className="h-3 w-3" />
                {inspection.package_manager}
              </Badge>
            )}
          {inspection.tooling.map((t) => (
            <Badge
              key={t}
              variant="secondary"
              className="gap-1 px-2 py-0.5 text-xs"
            >
              <Layers className="h-3 w-3" />
              {TOOLING_LABELS[t] ?? t}
            </Badge>
          ))}
        </div>
      </div>

      <p className="mt-3 text-sm text-muted-foreground">{subtitle}</p>

      {/* App list */}
      <div className="mt-6">
        {inspection.apps.length === 0 ? (
          <div className="rounded-lg border border-white/5 px-5 py-8 text-center text-sm text-muted-foreground">
            No apps were detected. Use the manual path option below.
          </div>
        ) : (
          <div className="rounded-lg border border-white/5">
            {inspection.apps.map((app, idx) => {
              const displayName =
                app.name || (app.path ? app.path : "Root project");
              const frameworkLabel =
                FRAMEWORK_LABELS[app.framework] ?? app.framework;
              const isLast = idx === inspection.apps.length - 1;
              const pathLabel = app.path || "root";
              const outputSuffix = app.output_directory
                ? ` · output: ${app.output_directory}`
                : "";

              return (
                <div
                  key={app.path || app.name || String(idx)}
                  className={cn(
                    "flex items-start gap-3 border-b border-white/5 px-5 py-4 transition-colors",
                    isLast && "border-b-0",
                    app.buildable
                      ? "cursor-pointer hover:bg-muted/50"
                      : "cursor-not-allowed opacity-60",
                  )}
                  onClick={() => {
                    if (app.buildable) {
                      onSelectApp(app);
                    }
                  }}
                >
                  {/* Icon */}
                  <Package
                    className={cn(
                      "mt-0.5 h-4 w-4 shrink-0",
                      app.buildable
                        ? "text-muted-foreground"
                        : "text-muted-foreground/50",
                    )}
                  />

                  {/* Content */}
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate font-mono text-sm font-medium">
                        {displayName}
                      </span>
                      <Badge
                        variant="secondary"
                        className="shrink-0 px-1.5 py-0 text-xs"
                      >
                        {frameworkLabel}
                      </Badge>
                      {!app.buildable && (
                        <Badge
                          variant="outline"
                          className="shrink-0 gap-1 px-1.5 py-0 text-xs text-muted-foreground"
                        >
                          <AlertCircle className="h-3 w-3" />
                          no build script
                        </Badge>
                      )}
                    </div>

                    {/* Path + output */}
                    <p className="mt-0.5 break-all font-mono text-xs text-muted-foreground">
                      {pathLabel}
                      {outputSuffix}
                    </p>

                    {/* Build command */}
                    {app.buildable && app.suggested_build_command && (
                      <p
                        className="mt-1 truncate font-mono text-xs text-muted-foreground/70"
                        title={app.suggested_build_command}
                      >
                        {app.suggested_build_command}
                      </p>
                    )}
                  </div>

                  {/* Select arrow (only for buildable) */}
                  {app.buildable && (
                    <ArrowRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground/50" />
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Divider + escape hatch */}
      <div className="mt-6 border-t border-white/5 pt-5">
        <button
          onClick={onSetManually}
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          Or set root directory manually
          <ArrowRight className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}
