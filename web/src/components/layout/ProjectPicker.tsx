import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Check, ChevronsUpDown, FolderKanban } from "lucide-react";

import { api } from "@/lib/api";
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
import { SidebarMenuButton } from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";

interface ProjectPickerProps {
  currentProjectId: string;
  currentProjectName: string;
}

export function ProjectPicker({
  currentProjectId,
  currentProjectName,
}: ProjectPickerProps) {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const { selectedOrganizationId } = useOrganizations();

  const { data: projects } = useQuery({
    queryKey: ["projects", selectedOrganizationId ?? "all"],
    queryFn: () => api.listProjects(selectedOrganizationId ?? undefined),
  });

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <SidebarMenuButton
          size="lg"
          className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
        >
          <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
            <FolderKanban className="size-4" />
          </div>
          <div className="flex min-w-0 flex-col gap-0.5 leading-none">
            <span className="truncate font-semibold">
              {currentProjectName}
            </span>
            <span className="text-xs text-muted-foreground">
              Switch project
            </span>
          </div>
          <ChevronsUpDown className="ml-auto size-4 shrink-0 opacity-50" />
        </SidebarMenuButton>
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
