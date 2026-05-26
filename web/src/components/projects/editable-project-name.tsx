// Inline-rename for the project name on the project overview page.
//
// Backend contract (internal/api/handlers/projects.go Update):
//   - Validates slug regex; 400 on mismatch.
//   - Rejects 409 when data_volume_enabled OR services attached (because
//     volume/service names are still keyed by project.Name today).
//   - Rename is a pure DB label change — running containers, auto-domains,
//     and nginx configs are NOT renamed. We surface that in helper text so
//     the user isn't surprised.
//
// We preempt the volume-rejection client-side (cheap, common case) and let
// the server's error message surface for the rest (invalid slug, services
// attached, name collision).
import { useEffect, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Pencil, X } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { Project } from "@/types/api";

// Mirrors backend slugRegex (project name = DNS subdomain).
const SLUG_RE = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;
const MAX_LEN = 63; // DNS label limit

interface Props {
  project: Project;
}

export function EditableProjectName({ project }: Props) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(project.name);
  const inputRef = useRef<HTMLInputElement>(null);

  // Re-sync draft if the underlying project changes (e.g. switched projects,
  // or a successful rename arrives via cache invalidation).
  useEffect(() => {
    if (!editing) setDraft(project.name);
  }, [project.name, editing]);

  useEffect(() => {
    if (editing) {
      inputRef.current?.focus();
      inputRef.current?.select();
    }
  }, [editing]);

  const renameLocked = project.data_volume_enabled;
  const trimmed = draft.trim().toLowerCase();
  const isUnchanged = trimmed === project.name;
  const isValid = SLUG_RE.test(trimmed) && trimmed.length <= MAX_LEN;
  const canSubmit = isValid && !isUnchanged;

  const mutation = useMutation({
    mutationFn: (name: string) => api.updateProject(project.id, { name }),
    onSuccess: (updated) => {
      setEditing(false);
      toast.success(`Renamed to "${updated.name}"`);
      // Refresh: detail page query, the workspace project lists, and the
      // command-palette project list (which uses its own cache key).
      queryClient.invalidateQueries({ queryKey: queryKeys.project(project.id) });
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      queryClient.invalidateQueries({ queryKey: ["command-projects"] });
    },
    onError: (err: Error) => {
      // Backend messages are already user-readable ("invalid name",
      // "cannot rename project while a persistent data volume…", etc.).
      toast.error(err.message || "Rename failed");
    },
  });

  const cancel = () => {
    setDraft(project.name);
    setEditing(false);
  };

  const submit = () => {
    if (!canSubmit) {
      cancel();
      return;
    }
    mutation.mutate(trimmed);
  };

  if (!editing) {
    const heading = (
      <h1
        className={cn(
          "group inline-flex items-center gap-2 text-xl font-semibold tracking-tight sm:text-2xl",
          renameLocked
            ? "cursor-not-allowed text-foreground"
            : "cursor-pointer rounded-md px-1 -mx-1 transition-colors hover:bg-muted/50",
        )}
        onClick={() => {
          if (!renameLocked) setEditing(true);
        }}
        title={renameLocked ? undefined : "Click to rename"}
      >
        {project.name}
        <Pencil
          className={cn(
            "h-3.5 w-3.5 shrink-0 text-muted-foreground/60 transition-opacity",
            renameLocked ? "opacity-0" : "opacity-0 group-hover:opacity-100",
          )}
          aria-hidden
        />
      </h1>
    );
    if (!renameLocked) return heading;
    return (
      <TooltipProvider delayDuration={150}>
        <Tooltip>
          <TooltipTrigger asChild>{heading}</TooltipTrigger>
          <TooltipContent side="bottom">
            Detach the persistent volume before renaming this project.
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <Input
          ref={inputRef}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              submit();
            } else if (e.key === "Escape") {
              e.preventDefault();
              cancel();
            }
          }}
          disabled={mutation.isPending}
          maxLength={MAX_LEN}
          aria-label="Project name"
          aria-invalid={!isValid && draft.length > 0}
          className={cn(
            "h-9 max-w-sm font-mono text-base",
            !isValid && draft.length > 0 && "border-destructive focus-visible:ring-destructive/40",
          )}
        />
        <Button
          size="icon"
          variant="default"
          onClick={submit}
          disabled={!canSubmit || mutation.isPending}
          aria-label="Save name"
          className="h-9 w-9"
        >
          <Check className="h-4 w-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          onClick={cancel}
          disabled={mutation.isPending}
          aria-label="Cancel rename"
          className="h-9 w-9"
        >
          <X className="h-4 w-4" />
        </Button>
      </div>
      <p
        className={cn(
          "text-xs",
          !isValid && draft.length > 0 ? "text-destructive" : "text-muted-foreground",
        )}
      >
        {!isValid && draft.length > 0
          ? "Lowercase letters, digits, and hyphens. Must start and end with a letter or digit."
          : "URL and container names stay the same — this only changes the dashboard label."}
      </p>
    </div>
  );
}
