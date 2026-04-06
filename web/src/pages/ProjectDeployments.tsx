import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import {
  ExternalLink,
  GitCommit,
  GlobeLock,
  Rocket,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  buildReleaseTagName,
  formatRelativeDate,
  getPrimaryEnvironmentUrl,
} from "@/lib/deployment-helpers";
import { ReleasePanelContent } from "@/components/projects/release-panel";
import { useIsMobile } from "@/hooks/use-mobile";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import { LoadingState } from "@/components/ui/spinner";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

export function ProjectDeployments() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const isMobile = useIsMobile();
  const [releaseSheetOpen, setReleaseSheetOpen] = useState(false);
  const [releaseTagName, setReleaseTagName] = useState(buildReleaseTagName());

  const { data: project } = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  const { data: deployments, isLoading } = useQuery({
    queryKey: ["deployments", id],
    queryFn: () => api.listDeployments(id),
    refetchInterval: (query) => {
      const items = query.state.data ?? [];
      return items.some((d) => ACTIVE_DEPLOYMENT_STATUSES.has(d.status))
        ? 3000
        : false;
    },
  });

  const { data: domains } = useQuery({
    queryKey: ["domains", id],
    queryFn: () => api.listDomains(id),
  });

  const deploymentMutation = useMutation({
    mutationFn: (payload: {
      environment: "preview" | "production";
      branch?: string;
      create_tag?: boolean;
      tag_name?: string;
    }) => api.triggerDeployment(id, payload),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ["deployments", id] });
      toast.success(
        variables.environment === "production"
          ? "Release queued"
          : "Preview deployment queued",
      );
      if (variables.environment === "production") {
        setReleaseSheetOpen(false);
      }
    },
    onError: (err) => toast.error(err.message),
  });

  const openDeploymentDetails = (deploymentId: string) => {
    navigate({
      to: "/projects/$id/deployments/$did",
      params: { id, did: deploymentId },
    });
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold">Deployments</h2>
          <p className="text-sm text-muted-foreground">
            Every deployment stays readable and row-clickable, with direct
            access to logs and live endpoints.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            onClick={() =>
              deploymentMutation.mutate({ environment: "preview" })
            }
            disabled={deploymentMutation.isPending}
          >
            <Rocket className="mr-1.5 h-3.5 w-3.5" />
            Deploy Preview
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setReleaseTagName(buildReleaseTagName());
              setReleaseSheetOpen(true);
            }}
            disabled={deploymentMutation.isPending}
          >
            <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
            Release
          </Button>
        </div>
      </div>

      {isLoading ? (
        <LoadingState
          title="Loading deployments..."
          description="Fetching recent preview and production build history."
        />
      ) : !deployments?.length ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-sm text-muted-foreground">
              No deployments yet. Click deploy to trigger your first build.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow className="border-white/8 hover:bg-transparent">
                  <TableHead className="pl-6">Environment</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Commit</TableHead>
                  <TableHead>Branch</TableHead>
                  <TableHead>Started</TableHead>
                  <TableHead>Duration</TableHead>
                  <TableHead className="text-right">Open</TableHead>
                  <TableHead className="pr-6 text-right">Logs</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {deployments.map((deployment) => {
                  const liveUrl =
                    deployment.status === "live"
                      ? getPrimaryEnvironmentUrl(
                          domains,
                          deployment.environment,
                        )
                      : null;
                  const statusMeta = DEPLOYMENT_STATUS_META[deployment.status];

                  return (
                    <TableRow
                      key={deployment.id}
                      className={cn(
                        "cursor-pointer border-white/8 transition-colors hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40",
                        deployment.status === "live" && "bg-white/[0.03]",
                      )}
                      role="link"
                      tabIndex={0}
                      onClick={() => openDeploymentDetails(deployment.id)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          openDeploymentDetails(deployment.id);
                        }
                      }}
                    >
                      <TableCell className="pl-6">
                        <div className="flex items-center gap-3">
                          <span
                            className={cn(
                              "h-2.5 w-2.5 rounded-full",
                              statusMeta.dotClass,
                              ACTIVE_DEPLOYMENT_STATUSES.has(
                                deployment.status,
                              ) && "animate-pulse",
                            )}
                          />
                          <div>
                            <Badge
                              variant="outline"
                              className={
                                ENVIRONMENT_META[deployment.environment]
                                  .badgeClass
                              }
                            >
                              {ENVIRONMENT_META[deployment.environment].label}
                            </Badge>
                            <p className="mt-1 text-xs text-muted-foreground">
                              {deployment.id.slice(0, 8)}
                            </p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={statusMeta.badgeClass}
                        >
                          {statusMeta.label}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-[340px]">
                        <div className="space-y-1">
                          <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                            <GitCommit className="h-3.5 w-3.5 text-muted-foreground" />
                            {deployment.commit_sha
                              ? deployment.commit_sha.slice(0, 7)
                              : "pending"}
                          </div>
                          <p
                            className="truncate text-xs text-muted-foreground"
                            title={
                              deployment.commit_message ||
                              deployment.error_message
                            }
                          >
                            {deployment.commit_message ||
                              deployment.error_message ||
                              statusMeta.label}
                          </p>
                        </div>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {deployment.branch}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {formatRelativeDate(deployment.created_at)}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {deployment.build_duration > 0
                          ? `${deployment.build_duration}s`
                          : "--"}
                      </TableCell>
                      <TableCell className="text-right">
                        {liveUrl ? (
                          <Button asChild size="sm" variant="ghost">
                            <a
                              href={liveUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              onClick={(event) => event.stopPropagation()}
                            >
                              <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                              Open
                            </a>
                          </Button>
                        ) : (
                          <span className="text-xs text-muted-foreground">
                            --
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="pr-6 text-right">
                        <Button asChild size="sm" variant="ghost">
                          <Link
                            to="/projects/$id/deployments/$did"
                            params={{ id, did: deployment.id }}
                            onClick={(event) => event.stopPropagation()}
                          >
                            Logs
                          </Link>
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {/* Release panel */}
      {project &&
        (isMobile ? (
          <Drawer open={releaseSheetOpen} onOpenChange={setReleaseSheetOpen}>
            <DrawerContent className="border-white/10 bg-[#0b1220]/98">
              <DrawerHeader>
                <DrawerTitle>Release to Production</DrawerTitle>
                <DrawerDescription>
                  Deployik will create a git tag and queue a production
                  deployment from that tagged ref.
                </DrawerDescription>
              </DrawerHeader>
              <ReleasePanelContent
                project={project}
                domains={domains}
                releaseTagName={releaseTagName}
                onReleaseTagChange={setReleaseTagName}
              />
              <DrawerFooter>
                <Button
                  variant="outline"
                  onClick={() => setReleaseSheetOpen(false)}
                  disabled={deploymentMutation.isPending}
                >
                  Cancel
                </Button>
                <Button
                  onClick={() =>
                    deploymentMutation.mutate({
                      environment: "production",
                      create_tag: true,
                      tag_name: releaseTagName.trim(),
                    })
                  }
                  disabled={
                    !releaseTagName.trim() || deploymentMutation.isPending
                  }
                >
                  <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
                  {deploymentMutation.isPending
                    ? "Queueing release..."
                    : "Release"}
                </Button>
              </DrawerFooter>
            </DrawerContent>
          </Drawer>
        ) : (
          <Sheet open={releaseSheetOpen} onOpenChange={setReleaseSheetOpen}>
            <SheetContent
              side="bottom"
              className="mx-auto w-full max-w-3xl rounded-t-3xl border-white/10 bg-[#0b1220]/98 px-6 pb-6 pt-5 backdrop-blur-2xl"
            >
              <SheetHeader>
                <SheetTitle>Release to Production</SheetTitle>
                <SheetDescription>
                  Deployik will create a git tag and queue a production
                  deployment from that tagged ref.
                </SheetDescription>
              </SheetHeader>
              <ReleasePanelContent
                project={project}
                domains={domains}
                releaseTagName={releaseTagName}
                onReleaseTagChange={setReleaseTagName}
              />
              <SheetFooter className="mt-6">
                <Button
                  variant="outline"
                  onClick={() => setReleaseSheetOpen(false)}
                  disabled={deploymentMutation.isPending}
                >
                  Cancel
                </Button>
                <Button
                  onClick={() =>
                    deploymentMutation.mutate({
                      environment: "production",
                      create_tag: true,
                      tag_name: releaseTagName.trim(),
                    })
                  }
                  disabled={
                    !releaseTagName.trim() || deploymentMutation.isPending
                  }
                >
                  <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
                  {deploymentMutation.isPending
                    ? "Queueing release..."
                    : "Release"}
                </Button>
              </SheetFooter>
            </SheetContent>
          </Sheet>
        ))}
    </div>
  );
}
