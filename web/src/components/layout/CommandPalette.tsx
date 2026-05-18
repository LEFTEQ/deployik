import * as React from "react";

import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Building2, FolderKanban, LogOut, Plus, Search } from "lucide-react";

import { useOrganizations } from "@/hooks/use-organizations";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useAuthStore } from "@/store/auth";
import { useGroupStore } from "@/store/group";
import { useOrganizationStore } from "@/store/organization";
import { Button } from "@/components/ui/button";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import { cn } from "@/lib/utils";

export function CommandPalette({ compact = false }: { compact?: boolean }) {
  const navigate = useNavigate();
  const { clearAuth } = useAuthStore();
  const {
    organizations,
    selectedOrganization,
    selectedOrganizationId,
    setSelectedOrganizationId,
  } = useOrganizations();
  const [open, setOpen] = React.useState(false);

  const { data: projects } = useQuery({
    queryKey: queryKeys.commandProjects(selectedOrganizationId),
    queryFn: () => api.listProjects(selectedOrganizationId ?? undefined),
  });

  React.useEffect(() => {
    const down = (event: KeyboardEvent) => {
      if (event.key.toLowerCase() === "k" && (event.metaKey || event.ctrlKey)) {
        event.preventDefault();
        setOpen((value) => !value);
      }
    };

    document.addEventListener("keydown", down);
    return () => document.removeEventListener("keydown", down);
  }, []);

  const handleLogout = async () => {
    setOpen(false);
    try {
      await api.logout();
    } finally {
      useGroupStore.getState().clearSelection();
      useOrganizationStore.getState().clearSelection();
      clearAuth();
    }
    window.location.href = "/login";
  };

  const runAndClose = (callback: () => void) => {
    setOpen(false);
    callback();
  };

  return (
    <>
      <Button
        variant="outline"
        size={compact ? "icon-sm" : "default"}
        className={cn(
          compact
            ? "h-9 w-9 shrink-0"
            : "h-9 w-64 shrink-0 justify-between gap-3 text-muted-foreground",
        )}
        onClick={() => setOpen(true)}
        aria-label="Open search"
      >
        {compact ? (
          <>
            <Search className="size-4" />
            <span className="sr-only">Search projects, groups, actions</span>
          </>
        ) : (
          <>
            <span className="inline-flex min-w-0 items-center gap-2">
              <Search className="size-4" />
              <span className="truncate">
                Search projects, groups, actions…
              </span>
            </span>
            <kbd className="hidden rounded-md border bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground sm:inline-flex">
              ⌘K
            </kbd>
          </>
        )}
      </Button>

      <CommandDialog
        open={open}
        onOpenChange={setOpen}
        title="Search Deployik"
        description="Search projects, groups, and actions."
      >
        <CommandInput placeholder="Type a command or search…" />
        <CommandList>
          <CommandEmpty>No results found.</CommandEmpty>

          <CommandGroup heading="Navigation">
            <CommandItem
              onSelect={() => runAndClose(() => navigate({ to: "/" }))}
            >
              <FolderKanban />
              <span>Projects</span>
              <CommandShortcut>⌘1</CommandShortcut>
            </CommandItem>
            <CommandItem
              onSelect={() => runAndClose(() => navigate({ to: "/new" }))}
            >
              <Plus />
              <span>New Project</span>
              <CommandShortcut>⌘N</CommandShortcut>
            </CommandItem>
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Groups">
            {organizations.map((organization) => (
              <CommandItem
                key={organization.id}
                onSelect={() =>
                  runAndClose(() => {
                    setSelectedOrganizationId(organization.id);
                    navigate({ to: "/" });
                  })
                }
              >
                <Building2 />
                <span>{organization.name}</span>
                {selectedOrganization?.id === organization.id ? (
                  <CommandShortcut>Current</CommandShortcut>
                ) : null}
              </CommandItem>
            ))}
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Projects">
            {(projects ?? []).length ? (
              (projects ?? []).slice(0, 12).map((project) => (
                <CommandItem
                  key={project.id}
                  onSelect={() =>
                    runAndClose(() =>
                      navigate({
                        to: "/projects/$id",
                        params: { id: project.id },
                      }),
                    )
                  }
                >
                  <FolderKanban />
                  <div className="min-w-0">
                    <p className="truncate">{project.name}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {project.github_owner}/{project.github_repo}
                    </p>
                  </div>
                  <CommandShortcut>{project.branch}</CommandShortcut>
                </CommandItem>
              ))
            ) : (
              <CommandItem disabled>
                <FolderKanban />
                <span>No projects in this group</span>
              </CommandItem>
            )}
          </CommandGroup>

          <CommandSeparator />

          <CommandGroup heading="Account">
            <CommandItem onSelect={handleLogout}>
              <LogOut />
              <span>Log Out</span>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
}
