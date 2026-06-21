import { useParams } from "@tanstack/react-router";
import { KeyRound, Lock } from "lucide-react";

import { AppVariableStore } from "@/components/apps/app-variable-store";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const REVEAL =
  "animate-in fade-in slide-in-from-bottom-2 duration-500 [animation-fill-mode:both]";
const reveal = (ms: number) => ({ className: REVEAL, style: { animationDelay: `${ms}ms` } });

export function AppVariables() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  return (
    <div className="space-y-7">
      {/* Header */}
      <div {...reveal(0)} className={cn(REVEAL, "space-y-1")}>
        <h1 className="text-2xl font-semibold tracking-tight">Variables</h1>
        <p className="text-sm text-muted-foreground">
          App-level environment variables &amp; secrets shared by every member
          project. Member-level values override these at deploy time.
        </p>
      </div>

      {/* Environment variables */}
      <Card
        {...reveal(60)}
        className={cn(REVEAL, "overflow-hidden bg-gradient-to-t from-primary/5 to-card")}
      >
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
              <KeyRound className="h-4 w-4" />
            </span>
            Environment variables
          </CardTitle>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Plain configuration. NEXT_PUBLIC_* keys are baked into the build.
          </p>
        </CardHeader>
        <CardContent>
          <AppVariableStore appId={appId} kind="env" />
        </CardContent>
      </Card>

      {/* Secrets */}
      <Card
        {...reveal(120)}
        className={cn(REVEAL, "overflow-hidden bg-gradient-to-t from-primary/5 to-card")}
      >
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary">
              <Lock className="h-4 w-4" />
            </span>
            Secrets
          </CardTitle>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Encrypted at rest, never exposed during build, injected only at runtime.
          </p>
        </CardHeader>
        <CardContent>
          <AppVariableStore appId={appId} kind="secret" />
        </CardContent>
      </Card>
    </div>
  );
}
