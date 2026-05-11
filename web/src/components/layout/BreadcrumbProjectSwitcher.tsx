import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Check, ChevronsUpDown, FolderKanban } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useOrganizations } from "@/hooks/use-organizations";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";

interface BreadcrumbProjectSwitcherProps {
  currentProjectId: string;
  currentProjectName: string;
}

export function BreadcrumbProjectSwitcher({
  currentProjectId,
  currentProjectName,
}: BreadcrumbProjectSwitcherProps) {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const { selectedOrganizationId } = useOrganizations();

  const { data: projects } = useQuery({
    queryKey: queryKeys.projects(selectedOrganizationId),
    queryFn: () => api.listProjects(selectedOrganizationId ?? undefined),
  });

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label="Switch project"
          className={cn(
            "inline-flex max-w-[200px] items-center gap-1 rounded-md px-1.5 py-0.5",
            "text-foreground transition-colors hover:bg-accent hover:text-accent-foreground",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            "data-[state=open]:bg-accent",
          )}
        >
          <span className="truncate font-medium">{currentProjectName}</span>
          <ChevronsUpDown className="size-3 shrink-0 opacity-50" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-0" align="start" side="bottom">
        <Command>
          <CommandInput placeholder="Search projects..." />
          <CommandList>
            <CommandEmpty>No projects found.</CommandEmpty>
            <CommandGroup>
              {projects?.map((project) => (
                <CommandItem
                  key={project.id}
                  value={project.name}
                  onSelect={() => {
                    navigate({
                      to: "/projects/$id",
                      params: { id: project.id },
                    });
                    setOpen(false);
                  }}
                >
                  <FolderKanban className="size-4" />
                  <span className="truncate">{project.name}</span>
                  {project.id === currentProjectId && (
                    <Check className={cn("ml-auto size-4 shrink-0")} />
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
