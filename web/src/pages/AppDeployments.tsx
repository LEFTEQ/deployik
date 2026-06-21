import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import {
  ACTIVE_DEPLOYMENT_STATUSES,
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatRelativeDate,
} from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

type Environment = "preview" | "production";

export function AppDeployments() {
  const { appId } = useParams({ strict: false }) as { appId: string };
  const navigate = useNavigate();
  const [environment, setEnvironment] = useState<Environment>("production");

  const { data: deployments, isLoading } = useQuery({
    queryKey: queryKeys.appDeployments(appId, environment),
    queryFn: () => api.listAppDeployments(appId, environment, 100),
    staleTime: staleTimes.activeDeployments,
    refetchInterval: (query) =>
      (query.state.data ?? []).some((d) =>
        ACTIVE_DEPLOYMENT_STATUSES.has(d.status),
      )
        ? 3000
        : false,
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Deployments</h2>
          <p className="text-sm text-muted-foreground">
            Every member's builds in one place. Click a row to open the
            project's deployment.
          </p>
        </div>
        <Select
          value={environment}
          onValueChange={(v) => setEnvironment(v as Environment)}
        >
          <SelectTrigger className="w-[150px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="production">Production</SelectItem>
            <SelectItem value="preview">Preview</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <LoadingState
          title="Loading deployments..."
          description="Fetching every member's recent build history."
        />
      ) : !deployments?.length ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-sm text-muted-foreground">
              No {environment} deployments yet.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow className="border-white/8 hover:bg-transparent">
                  <TableHead className="pl-6">Project</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Commit</TableHead>
                  <TableHead>Environment</TableHead>
                  <TableHead>Started</TableHead>
                  <TableHead className="pr-6 text-right">Duration</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {deployments.map((d) => {
                  const statusMeta = DEPLOYMENT_STATUS_META[d.status];
                  const envMeta = ENVIRONMENT_META[d.environment];
                  const open = () =>
                    navigate({
                      to: "/projects/$id/deployments/$did",
                      params: { id: d.project_id, did: d.id },
                    });

                  return (
                    <TableRow
                      key={d.id}
                      className={cn(
                        "cursor-pointer border-white/8 transition-colors hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40",
                        d.status === "live" && "bg-white/[0.03]",
                      )}
                      role="link"
                      tabIndex={0}
                      onClick={open}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          open();
                        }
                      }}
                    >
                      <TableCell className="pl-6 font-medium">
                        {d.project_name}
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={statusMeta.badgeClass}
                        >
                          {statusMeta.label}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-[320px]">
                        <div className="space-y-1">
                          <span className="font-mono text-xs text-foreground">
                            {d.commit_sha ? d.commit_sha.slice(0, 7) : "pending"}
                          </span>
                          <p
                            className="truncate text-xs text-muted-foreground"
                            title={d.commit_message || d.error_message}
                          >
                            {d.commit_message ||
                              d.error_message ||
                              statusMeta.label}
                          </p>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge
                          variant="outline"
                          className={envMeta?.badgeClass}
                        >
                          {envMeta?.label}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {formatRelativeDate(d.created_at)}
                      </TableCell>
                      <TableCell className="pr-6 text-right text-sm text-muted-foreground">
                        {d.build_duration > 0 ? `${d.build_duration}s` : "--"}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
