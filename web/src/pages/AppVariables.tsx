import { useParams } from "@tanstack/react-router";

import { AppVariableStore } from "@/components/apps/app-variable-store";
import { cn } from "@/lib/utils";

const REVEAL =
  "animate-in fade-in slide-in-from-bottom-2 duration-500 [animation-fill-mode:both]";
const reveal = (ms: number) => ({ className: REVEAL, style: { animationDelay: `${ms}ms` } });

export function AppVariables() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  return (
    <div className="space-y-8">
      {/* Header */}
      <div {...reveal(0)} className={cn(REVEAL, "space-y-1")}>
        <h1 className="text-2xl font-semibold tracking-tight">Variables</h1>
        <p className="text-sm text-muted-foreground">
          App-level environment variables &amp; secrets shared by every member
          project. Member-level values override these at deploy time.
        </p>
      </div>

      {/* Environment variables */}
      <section {...reveal(60)} className={cn(REVEAL, "space-y-3")}>
        <div>
          <h2 className="text-sm font-semibold text-foreground">Environment variables</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Plain configuration. NEXT_PUBLIC_* keys are baked into the build.
          </p>
        </div>
        <AppVariableStore appId={appId} kind="env" />
      </section>

      {/* Secrets */}
      <section {...reveal(120)} className={cn(REVEAL, "space-y-3")}>
        <div>
          <h2 className="text-sm font-semibold text-foreground">Secrets</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Encrypted at rest, never exposed during build, injected only at runtime.
          </p>
        </div>
        <AppVariableStore appId={appId} kind="secret" />
      </section>
    </div>
  );
}
