import { Link, useMatchRoute } from "@tanstack/react-router";

import { Plus } from "lucide-react";

import { useOrganizations } from "@/hooks/use-organizations";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";

function getPageLabel(matchRoute: ReturnType<typeof useMatchRoute>) {
  if (matchRoute({ to: "/new" })) return "New Project";
  if (matchRoute({ to: "/projects/$id/deployments/$did", fuzzy: true })) {
    return "Deployment";
  }
  if (matchRoute({ to: "/projects/$id", fuzzy: true })) return "Project";
  return "Projects";
}

export function SiteHeader() {
  const matchRoute = useMatchRoute();
  const { selectedOrganization } = useOrganizations();
  const pageLabel = getPageLabel(matchRoute);

  return (
    <header className="sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b bg-background/85 backdrop-blur supports-[backdrop-filter]:bg-background/70">
      <div className="flex w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
        <SidebarTrigger className="-ml-1" />
        <Separator
          orientation="vertical"
          className="mx-2 data-[orientation=vertical]:h-4"
        />
        <div className="min-w-0">
          <p className="text-sm font-medium">{pageLabel}</p>
          <p className="truncate text-xs text-muted-foreground">
            {selectedOrganization?.name ?? "Workspace"}
          </p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="ghost" asChild size="sm" className="hidden sm:flex">
            <Link to="/">Projects</Link>
          </Button>
          <Button asChild size="sm">
            <Link to="/new">
              <Plus />
              New Project
            </Link>
          </Button>
        </div>
      </div>
    </header>
  );
}
