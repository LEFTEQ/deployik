import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  ArrowUpRight,
  Building2,
  CheckCircle2,
  CircleDot,
  Copy,
  ExternalLink,
  GitBranch,
  GitCommit,
  Globe2,
  GlobeLock,
  Link2,
  Settings,
  Rocket,
  Globe,
  Key,
  LoaderCircle,
  Trash2,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import {
  BuildSettingsFields,
  formatFrameworkLabel,
} from "@/components/projects/build-settings";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { useEffect, useState } from "react";
import type {
  Deployment,
  DeploymentStatus,
  Domain,
  PlatformInfo,
  ProjectVariable,
  VariableScope,
} from "@/types/api";

const ACTIVE_DEPLOYMENT_STATUSES = new Set<DeploymentStatus>([
  "queued",
  "building",
  "deploying",
]);

const ENVIRONMENT_META = {
  preview: {
    label: "Preview",
    description: "Auto preview URLs and branded staging domains.",
    badgeClass:
      "border-sky-400/25 bg-sky-400/12 text-sky-100 shadow-[inset_0_0_0_1px_rgba(56,189,248,0.12)]",
  },
  production: {
    label: "Production",
    description: "The real customer-facing domain you bought.",
    badgeClass:
      "border-emerald-400/25 bg-emerald-400/12 text-emerald-100 shadow-[inset_0_0_0_1px_rgba(16,185,129,0.12)]",
  },
} satisfies Record<
  Domain["environment"],
  { label: string; description: string; badgeClass: string }
>;

const VARIABLE_SCOPE_META = {
  shared: {
    label: "Shared",
    description:
      "Applied to both preview and production unless a scoped value overrides it.",
    badgeClass:
      "border-fuchsia-400/25 bg-fuchsia-400/12 text-fuchsia-100 shadow-[inset_0_0_0_1px_rgba(217,70,239,0.12)]",
  },
  preview: {
    label: "Preview",
    description: "Only used for preview deployments.",
    badgeClass: ENVIRONMENT_META.preview.badgeClass,
  },
  production: {
    label: "Production",
    description: "Only used for production deployments.",
    badgeClass: ENVIRONMENT_META.production.badgeClass,
  },
} satisfies Record<
  VariableScope,
  { label: string; description: string; badgeClass: string }
>;

const DEPLOYMENT_STATUS_META = {
  queued: {
    label: "Queued",
    badgeClass: "border-white/10 bg-white/5 text-slate-200",
    dotClass: "bg-slate-400",
  },
  building: {
    label: "Building",
    badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100",
    dotClass: "bg-amber-400",
  },
  deploying: {
    label: "Deploying",
    badgeClass: "border-sky-400/25 bg-sky-400/12 text-sky-100",
    dotClass: "bg-sky-400",
  },
  live: {
    label: "Live",
    badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100",
    dotClass: "bg-emerald-400",
  },
  failed: {
    label: "Failed",
    badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100",
    dotClass: "bg-rose-400",
  },
  rolled_back: {
    label: "Rolled back",
    badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100",
    dotClass: "bg-orange-400",
  },
  replaced: {
    label: "Replaced",
    badgeClass: "border-white/10 bg-white/5 text-slate-200",
    dotClass: "bg-slate-500",
  },
} satisfies Record<
  DeploymentStatus,
  { label: string; badgeClass: string; dotClass: string }
>;

function isDomainReady(domain: Domain) {
  return domain.dns_verified && domain.ssl_status === "active";
}

function getEnvironmentDomains(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
) {
  return (domains ?? []).filter((domain) => domain.environment === environment);
}

function getReadyEnvironmentDomains(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
) {
  return getEnvironmentDomains(domains, environment).filter(isDomainReady);
}

function getPrimaryEnvironmentUrl(
  domains: Domain[] | undefined,
  environment: Domain["environment"],
) {
  const readyDomains = getReadyEnvironmentDomains(domains, environment);
  if (!readyDomains.length) return null;

  const preferred =
    readyDomains.find((domain) =>
      environment === "preview" ? domain.is_auto : !domain.is_auto,
    ) ?? readyDomains[0];
  if (!preferred) return null;

  return `https://${preferred.domain}`;
}

function getLatestEnvironmentDeployment(
  deployments: Deployment[] | undefined,
  environment: Deployment["environment"],
) {
  return (deployments ?? []).find(
    (deployment) => deployment.environment === environment,
  );
}

function getLatestLiveEnvironmentDeployment(
  deployments: Deployment[] | undefined,
  environment: Deployment["environment"],
) {
  return (deployments ?? []).find(
    (deployment) =>
      deployment.environment === environment && deployment.status === "live",
  );
}

function formatRelativeDate(value: string) {
  return formatDistanceToNow(new Date(value), { addSuffix: true });
}

export function ProjectDetail() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: project, isLoading } = useQuery({
    queryKey: ["project", id],
    queryFn: () => api.getProject(id),
  });

  const { data: deployments, isLoading: deploymentsLoading } = useQuery({
    queryKey: ["deployments", id],
    queryFn: () => api.listDeployments(id),
    refetchInterval: (query) => {
      const items = query.state.data ?? [];
      return items.some((deployment) =>
        ACTIVE_DEPLOYMENT_STATUSES.has(deployment.status),
      )
        ? 3000
        : false;
    },
  });

  const { data: domains, isLoading: domainsLoading } = useQuery({
    queryKey: ["domains", id],
    queryFn: () => api.listDomains(id),
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteProject(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      toast.success("Project deleted");
      navigate({ to: "/" });
    },
    onError: (err) => toast.error(err.message),
  });

  if (isLoading) {
    return (
      <div className="p-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="mt-2 h-5 w-64" />
        <Skeleton className="mt-6 h-64 w-full" />
      </div>
    );
  }

  if (!project) {
    return (
      <div className="p-6">
        <p>Project not found</p>
        <Link to="/" className="mt-2 text-sm text-primary hover:underline">
          Back to projects
        </Link>
      </div>
    );
  }

  const primaryPreviewUrl = getPrimaryEnvironmentUrl(domains, "preview");
  const primaryProductionUrl = getPrimaryEnvironmentUrl(domains, "production");

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <Link
        to="/"
        className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to projects
      </Link>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(340px,0.8fr)]">
        <Card className="overflow-hidden border-white/10">
          <CardContent className="relative px-6 py-6">
            <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-primary/40 to-transparent" />
            <div className="mb-5 flex flex-wrap items-center gap-2">
              <Badge
                variant="outline"
                className="border-primary/25 bg-primary/10 text-primary"
              >
                Control plane
              </Badge>
              <Badge
                variant="outline"
                className={cn(
                  "border-white/10 bg-white/5 text-slate-200",
                  project.status === "active" &&
                    "border-emerald-400/25 bg-emerald-400/12 text-emerald-100",
                )}
              >
                <CircleDot className="mr-1 size-3 fill-current" />
                {project.status}
              </Badge>
            </div>

            <div className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
              <div className="space-y-3">
                <div>
                  <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
                    {project.name}
                  </h1>
                  <p className="mt-2 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
                    <span>
                      {project.github_owner}/{project.github_repo}
                    </span>
                    {project.organization_name ? (
                      <span className="flex items-center gap-1">
                        <Building2 className="h-3.5 w-3.5" />
                        {project.organization_name}
                      </span>
                    ) : null}
                    <span className="flex items-center gap-1">
                      <GitBranch className="h-3.5 w-3.5" />
                      {project.branch}
                    </span>
                  </p>
                </div>

                <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                  <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-3">
                    <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
                      Framework
                    </p>
                    <p className="mt-2 text-sm font-medium text-foreground">
                      {formatFrameworkLabel(project.framework)}
                    </p>
                  </div>
                  <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-3">
                    <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
                      Runtime
                    </p>
                    <p className="mt-2 text-sm font-medium text-foreground">
                      Node.js {project.node_version}
                    </p>
                  </div>
                  <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-3">
                    <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
                      Workspace
                    </p>
                    <p className="mt-2 truncate text-sm font-medium text-foreground">
                      {project.organization_name || "Personal"}
                    </p>
                  </div>
                  <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-3">
                    <p className="text-xs uppercase tracking-[0.18em] text-muted-foreground">
                      Auto preview
                    </p>
                    <p className="mt-2 truncate text-sm font-medium text-foreground">
                      {project.name}.preview.example.com
                    </p>
                  </div>
                </div>
              </div>

              <div className="flex flex-wrap gap-2 lg:max-w-[260px] lg:justify-end">
                {primaryPreviewUrl ? (
                  <Button asChild size="sm">
                    <a
                      href={primaryPreviewUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                      Open preview
                    </a>
                  </Button>
                ) : null}
                {primaryProductionUrl ? (
                  <Button asChild size="sm" variant="outline">
                    <a
                      href={primaryProductionUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <Globe2 className="mr-1.5 h-3.5 w-3.5" />
                      Open production
                    </a>
                  </Button>
                ) : null}
              </div>
            </div>
          </CardContent>
        </Card>

        <QuickLinksCard
          projectName={project.name}
          deployments={deployments}
          domains={domains}
          isLoading={deploymentsLoading || domainsLoading}
        />
      </div>

      {/* Tabs */}
      <Tabs defaultValue="deployments" className="mt-6">
        <TabsList className="h-auto flex-wrap justify-start gap-1 rounded-2xl border border-white/8 bg-black/10 p-1">
          <TabsTrigger value="deployments">
            <Rocket className="mr-1.5 h-3.5 w-3.5" />
            Deployments
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Settings className="mr-1.5 h-3.5 w-3.5" />
            Settings
          </TabsTrigger>
          <TabsTrigger value="domains">
            <Globe className="mr-1.5 h-3.5 w-3.5" />
            Domains
          </TabsTrigger>
          <TabsTrigger value="env">
            <Key className="mr-1.5 h-3.5 w-3.5" />
            Env Vars
          </TabsTrigger>
          <TabsTrigger value="secrets">
            <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
            Secrets
          </TabsTrigger>
        </TabsList>

        <TabsContent value="deployments" className="mt-4">
          <DeploymentsTab
            projectId={id}
            deployments={deployments}
            domains={domains}
            isLoading={deploymentsLoading}
          />
        </TabsContent>

        <TabsContent value="settings" className="mt-4">
          <SettingsTab
            project={project}
            onDelete={() => deleteMutation.mutate()}
          />
        </TabsContent>

        <TabsContent value="domains" className="mt-4">
          <DomainsTab
            projectId={id}
            domains={domains}
            isLoading={domainsLoading}
          />
        </TabsContent>

        <TabsContent value="env" className="mt-4">
          <EnvVarsTab projectId={id} />
        </TabsContent>

        <TabsContent value="secrets" className="mt-4">
          <SecretsTab projectId={id} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function QuickLinksCard({
  projectName,
  deployments,
  domains,
  isLoading,
}: {
  projectName: string;
  deployments: Deployment[] | undefined;
  domains: Domain[] | undefined;
  isLoading: boolean;
}) {
  return (
    <Card className="border-white/10">
      <CardHeader>
        <CardTitle className="text-base">Quick Links</CardTitle>
        <CardDescription>
          Jump straight into the current live environments.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : (
          (["preview", "production"] as const).map((environment) => {
            const readyDomains = getReadyEnvironmentDomains(
              domains,
              environment,
            );
            const liveDeployment = getLatestLiveEnvironmentDeployment(
              deployments,
              environment,
            );
            const latestDeployment = getLatestEnvironmentDeployment(
              deployments,
              environment,
            );

            return (
              <div
                key={environment}
                className="rounded-2xl border border-white/8 bg-black/10 p-4"
              >
                <div className="flex items-start justify-between gap-4">
                  <div className="space-y-2">
                    <Badge
                      variant="outline"
                      className={ENVIRONMENT_META[environment].badgeClass}
                    >
                      {ENVIRONMENT_META[environment].label}
                    </Badge>
                    <div>
                      <p className="text-sm font-medium text-foreground">
                        {ENVIRONMENT_META[environment].description}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {liveDeployment
                          ? `Live on ${liveDeployment.commit_sha.slice(0, 7)}`
                          : latestDeployment
                            ? `Latest deployment is ${DEPLOYMENT_STATUS_META[latestDeployment.status].label.toLowerCase()}`
                            : environment === "preview"
                              ? `${projectName}.preview.example.com will appear here after the first healthy deploy.`
                              : "Add and verify a production domain to unlock direct links."}
                      </p>
                    </div>
                  </div>

                  {liveDeployment ? (
                    <Badge
                      variant="outline"
                      className={DEPLOYMENT_STATUS_META.live.badgeClass}
                    >
                      Live
                    </Badge>
                  ) : null}
                </div>

                {readyDomains.length ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    {readyDomains.map((domain) => (
                      <Button
                        asChild
                        key={domain.id}
                        size="sm"
                        variant="outline"
                      >
                        <a
                          href={`https://${domain.domain}`}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <ArrowUpRight className="mr-1.5 h-3.5 w-3.5" />
                          {domain.is_auto ? "Auto URL" : domain.domain}
                        </a>
                      </Button>
                    ))}
                  </div>
                ) : (
                  <div className="mt-4 rounded-xl border border-dashed border-white/8 px-3 py-2 text-xs text-muted-foreground">
                    {environment === "preview"
                      ? "No active preview URL yet."
                      : "No verified production domain yet."}
                  </div>
                )}
              </div>
            );
          })
        )}
      </CardContent>
    </Card>
  );
}

function DeploymentsTab({
  projectId,
  deployments,
  domains,
  isLoading,
}: {
  projectId: string;
  deployments: Deployment[] | undefined;
  domains: Domain[] | undefined;
  isLoading: boolean;
}) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const openDeploymentDetails = (deploymentId: string) => {
    navigate({
      to: "/projects/$id/deployments/$did",
      params: { id: projectId, did: deploymentId },
    });
  };

  const deployMutation = useMutation({
    mutationFn: (env: string) =>
      api.triggerDeployment(projectId, { environment: env }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["deployments", projectId] });
      toast.success("Deployment triggered");
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-4">
      <Card className="border-white/10">
        <CardHeader className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <CardTitle className="text-base">Deployment Timeline</CardTitle>
            <CardDescription>
              Trigger fresh builds and open the current live environment without
              leaving the table.
            </CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              onClick={() => deployMutation.mutate("preview")}
              disabled={deployMutation.isPending}
            >
              <Rocket className="mr-1.5 h-3.5 w-3.5" />
              Deploy preview
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => deployMutation.mutate("production")}
              disabled={deployMutation.isPending}
            >
              <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
              Deploy production
            </Button>
          </div>
        </CardHeader>
      </Card>

      {isLoading ? (
        <Card>
          <CardContent className="p-6">
            <Skeleton className="h-20 w-full" />
          </CardContent>
        </Card>
      ) : !deployments?.length ? (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-sm text-muted-foreground">
              No deployments yet. Click deploy to trigger your first build.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card className="overflow-hidden border-white/10">
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
                    ? getPrimaryEnvironmentUrl(domains, deployment.environment)
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
                            ACTIVE_DEPLOYMENT_STATUSES.has(deployment.status) &&
                              "animate-pulse",
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
                        : "—"}
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
                        <span className="text-xs text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="pr-6 text-right">
                      <Button asChild size="sm" variant="ghost">
                        <Link
                          to="/projects/$id/deployments/$did"
                          params={{ id: projectId, did: deployment.id }}
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
        </Card>
      )}
    </div>
  );
}

function SettingsTab({
  project,
  onDelete,
}: {
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
  onDelete: () => void;
}) {
  const queryClient = useQueryClient();
  const [branch, setBranch] = useState(project.branch);
  const [buildSettings, setBuildSettings] = useState({
    framework: project.framework,
    packageManager: project.package_manager,
    rootDirectory: project.root_directory,
    outputDirectory: project.output_directory,
    buildCommand: project.build_command,
    installCommand: project.install_command,
    nodeVersion: project.node_version,
  });

  const updateMutation = useMutation({
    mutationFn: () =>
      api.updateProject(project.id, {
        branch,
        framework: buildSettings.framework,
        package_manager: buildSettings.packageManager,
        root_directory: buildSettings.rootDirectory,
        output_directory: buildSettings.outputDirectory,
        build_command: buildSettings.buildCommand,
        install_command: buildSettings.installCommand,
        node_version: buildSettings.nodeVersion,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project", project.id] });
      toast.success("Settings updated");
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Build Settings</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label>Workspace</Label>
            <Input value={project.organization_name || "Personal"} disabled />
          </div>
          <div className="space-y-2">
            <Label>Branch</Label>
            <Input value={branch} onChange={(e) => setBranch(e.target.value)} />
          </div>
          <BuildSettingsFields
            value={buildSettings}
            onChange={setBuildSettings}
          />
          <Button
            onClick={() => updateMutation.mutate()}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending ? "Saving..." : "Save Settings"}
          </Button>
        </CardContent>
      </Card>

      <Card className="border-destructive/50">
        <CardHeader>
          <CardTitle className="text-base text-destructive">
            Danger Zone
          </CardTitle>
        </CardHeader>
        <CardContent>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button variant="destructive">
                <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                Delete Project
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete project?</AlertDialogTitle>
                <AlertDialogDescription>
                  This will stop all running containers and remove the project.
                  This action cannot be undone.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onDelete}>Delete</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardContent>
      </Card>
    </div>
  );
}

function DomainsTab({
  projectId,
  domains,
  isLoading,
}: {
  projectId: string;
  domains: Domain[] | undefined;
  isLoading: boolean;
}) {
  const queryClient = useQueryClient();
  const [newDomain, setNewDomain] = useState("");
  const [newDomainEnvironment, setNewDomainEnvironment] =
    useState<Domain["environment"]>("production");
  const { data: platform } = useQuery({
    queryKey: ["platform"],
    queryFn: () => api.getPlatformInfo(),
  });

  const addMutation = useMutation({
    mutationFn: () =>
      api.addDomain(projectId, {
        domain: newDomain.trim().toLowerCase(),
        environment: newDomainEnvironment,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domains", projectId] });
      setNewDomain("");
      setNewDomainEnvironment("production");
      toast.success("Domain added");
    },
    onError: (err) => toast.error(err.message),
  });

  const verifyMutation = useMutation({
    mutationFn: (domainId: string) => api.verifyDomain(projectId, domainId),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["domains", projectId] });
      toast.success(result.message);
    },
    onError: (err) => toast.error(err.message),
  });

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Add Custom Domain</CardTitle>
          <CardDescription>
            Choose whether the domain should front preview or production.
            Production is usually the bought, customer-facing domain.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px_auto]">
          <Input
            placeholder="example.com"
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
          />
          <Select
            value={newDomainEnvironment}
            onValueChange={(value) =>
              setNewDomainEnvironment(value as Domain["environment"])
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="preview">Preview</SelectItem>
              <SelectItem value="production">Production</SelectItem>
            </SelectContent>
          </Select>
          <Button
            onClick={() => addMutation.mutate()}
            disabled={!newDomain.trim() || addMutation.isPending}
          >
            {addMutation.isPending ? "Adding..." : "Add domain"}
          </Button>
        </CardContent>
      </Card>

      <DnsSetupGuide
        domain={newDomain.trim().toLowerCase()}
        environment={newDomainEnvironment}
        platform={platform}
      />

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Domain Inventory</CardTitle>
          <CardDescription>
            Verified domains become quick links automatically.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : !domains?.length ? (
            <div className="rounded-2xl border border-dashed border-white/8 px-4 py-6 text-sm text-muted-foreground">
              No domains yet.
            </div>
          ) : (
            domains.map((domain) => {
              const ready = isDomainReady(domain);
              const verifying =
                verifyMutation.isPending &&
                verifyMutation.variables === domain.id;

              return (
                <div
                  key={domain.id}
                  className="flex flex-col gap-4 rounded-2xl border border-white/8 bg-black/10 p-4 md:flex-row md:items-center md:justify-between"
                >
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-sm font-medium">{domain.domain}</p>
                      <Badge
                        variant="outline"
                        className={
                          ENVIRONMENT_META[domain.environment].badgeClass
                        }
                      >
                        {ENVIRONMENT_META[domain.environment].label}
                      </Badge>
                      <Badge variant={domain.is_auto ? "secondary" : "outline"}>
                        {domain.is_auto ? "Auto" : "Custom"}
                      </Badge>
                    </div>
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span className="inline-flex items-center gap-1 rounded-full border border-white/8 px-2 py-1">
                        <Link2 className="h-3 w-3" />
                        DNS {domain.dns_verified ? "verified" : "pending"}
                      </span>
                      <span
                        className={cn(
                          "inline-flex items-center gap-1 rounded-full border px-2 py-1",
                          domain.ssl_status === "active" &&
                            "border-emerald-400/25 text-emerald-200",
                          domain.ssl_status === "pending" &&
                            "border-amber-400/25 text-amber-100",
                          domain.ssl_status === "error" &&
                            "border-rose-400/25 text-rose-200",
                        )}
                      >
                        <CheckCircle2 className="h-3 w-3" />
                        SSL {domain.ssl_status}
                      </span>
                    </div>
                  </div>

                  <div className="flex flex-wrap gap-2">
                    {ready ? (
                      <Button asChild size="sm" variant="outline">
                        <a
                          href={`https://${domain.domain}`}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <ExternalLink className="mr-1.5 h-3.5 w-3.5" />
                          Open
                        </a>
                      </Button>
                    ) : null}
                    {!domain.is_auto ? (
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => verifyMutation.mutate(domain.id)}
                        disabled={verifyMutation.isPending}
                      >
                        {verifying ? (
                          <LoaderCircle className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
                        )}
                        Verify
                      </Button>
                    ) : null}
                  </div>
                </div>
              );
            })
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function DnsSetupGuide({
  domain,
  environment,
  platform,
}: {
  domain: string;
  environment: Domain["environment"];
  platform: PlatformInfo | undefined;
}) {
  const dnsTargetIp = platform?.dns_target_ip?.trim();
  const sampleDomain =
    domain || (environment === "preview" ? "staging.example.com" : "example.com");
  const sampleHost = getDnsHostHint(sampleDomain);

  const copyValue = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`Couldn't copy ${label.toLowerCase()}`);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">DNS Setup</CardTitle>
        <CardDescription>
          Point the domain at this VPS, then click Verify. Preview domains should
          usually be subdomains; production is usually the real bought domain.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="rounded-2xl border border-white/8 bg-black/10 p-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                  Target IP
                </p>
                <p className="mt-2 font-mono text-sm">
                  {dnsTargetIp || "Unavailable"}
                </p>
              </div>
              {dnsTargetIp ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => copyValue(dnsTargetIp, "Target IP")}
                >
                  <Copy className="mr-1.5 h-3.5 w-3.5" />
                  Copy
                </Button>
              ) : null}
            </div>
          </div>

          <div className="rounded-2xl border border-white/8 bg-black/10 p-4">
            <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
              Record to create
            </p>
            <div className="mt-2 space-y-1 text-sm">
              <p>
                <span className="text-muted-foreground">Domain:</span>{" "}
                <span className="font-medium">{sampleDomain}</span>
              </p>
              <p>
                <span className="text-muted-foreground">Type:</span>{" "}
                <span className="font-medium">A</span>
              </p>
              <p>
                <span className="text-muted-foreground">Host / Name:</span>{" "}
                <span className="font-medium">{sampleHost}</span>
              </p>
            </div>
          </div>
        </div>

        <div className="grid gap-3 xl:grid-cols-2">
          <div className="rounded-2xl border border-white/8 bg-black/10 p-4">
            <p className="text-sm font-medium">GoDaddy</p>
            <div className="mt-3 space-y-2 text-sm text-muted-foreground">
              <p>1. Open your domain in GoDaddy and go to DNS.</p>
              <p>2. Add an A record for the host shown above.</p>
              <p>3. Set Points to to {dnsTargetIp || "the target VPS IP"}.</p>
              <p>4. Leave TTL on the default value.</p>
              <p>5. Remove conflicting A, AAAA, or forwarded records for the same host.</p>
            </div>
          </div>

          <div className="rounded-2xl border border-white/8 bg-black/10 p-4">
            <p className="text-sm font-medium">Vercel DNS / Domain bought on Vercel</p>
            <div className="mt-3 space-y-2 text-sm text-muted-foreground">
              <p>1. Open the domain in Vercel and go to DNS Records.</p>
              <p>2. Add an A record for the host shown above.</p>
              <p>3. Set the value to {dnsTargetIp || "the target VPS IP"}.</p>
              <p>4. If you also want `www`, point it at the same IP or CNAME it to the root.</p>
              <p>5. Remove old Vercel-specific records for the same host if they conflict.</p>
            </div>
          </div>
        </div>

        <div className="rounded-2xl border border-dashed border-white/10 px-4 py-3 text-sm text-muted-foreground">
          <p>
            Deployik verifies A-record resolution to the VPS IP. If you use a
            subdomain, prefer an A record directly to the server. After DNS
            propagates, click Verify to issue SSL and activate the domain.
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

function getDnsHostHint(domain: string) {
  const labels = domain.split(".").filter(Boolean);
  if (labels.length <= 2) {
    return "@";
  }
  if (labels.length === 3) {
    return labels[0];
  }
  return `${labels.slice(0, -2).join(".")} (subdomain part)`;
}

function EnvVarsTab({ projectId }: { projectId: string }) {
  return <VariableStoreTab projectId={projectId} kind="env" />;
}

function SecretsTab({ projectId }: { projectId: string }) {
  return <VariableStoreTab projectId={projectId} kind="secret" />;
}

function VariableStoreTab({
  projectId,
  kind,
}: {
  projectId: string;
  kind: ProjectVariable["kind"];
}) {
  const queryClient = useQueryClient();
  const [scope, setScope] = useState<VariableScope>("shared");
  const [rows, setRows] = useState<{ key: string; value: string }[]>([
    { key: "", value: "" },
  ]);

  useEffect(() => {
    setRows([{ key: "", value: "" }]);
  }, [kind, scope]);

  const isSecret = kind === "secret";
  const storeTitle = isSecret ? "Secrets" : "Env Vars";
  const storeDescription = isSecret
    ? "Encrypted at rest, never exposed in the build, and injected only at runtime."
    : "Configuration values for your app. Use NEXT_PUBLIC_* only for values that are safe to expose to the client bundle.";
  const scopeDescription = isSecret
    ? "Shared secrets apply to both preview and production. Scope-specific secrets override shared ones with the same key."
    : "Shared env vars apply to both preview and production. Scope-specific env vars override shared ones with the same key.";
  const emptyState = isSecret
    ? "No secrets stored for this scope yet."
    : "No environment variables stored for this scope yet.";
  const replaceDescription = isSecret
    ? "Saving here replaces all secrets for the selected scope."
    : "Saving here replaces all env vars for the selected scope.";

  const { data: existingVars, isLoading } = useQuery({
    queryKey: ["project-variables", kind, projectId, scope],
    queryFn: () =>
      isSecret
        ? api.listSecrets(projectId, scope)
        : api.listEnvVars(projectId, scope),
  });

  const saveMutation = useMutation({
    mutationFn: () => {
      const variables = rows.filter((row) => row.key.trim() !== "");
      return isSecret
        ? api.bulkSetSecrets(projectId, { environment: scope, variables })
        : api.bulkSetEnvVars(projectId, { environment: scope, variables });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["project-variables", kind, projectId],
      });
      toast.success(isSecret ? "Secrets saved" : "Environment variables saved");
      setRows([{ key: "", value: "" }]);
    },
    onError: (err) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (key: string) =>
      isSecret
        ? api.deleteSecret(projectId, key, scope)
        : api.deleteEnvVar(projectId, key, scope),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["project-variables", kind, projectId],
      });
      toast.success(
        isSecret ? "Secret deleted" : "Environment variable deleted",
      );
    },
    onError: (err) => toast.error(err.message),
  });

  const addRow = () => setRows([...rows, { key: "", value: "" }]);
  const updateRow = (idx: number, field: "key" | "value", value: string) => {
    const nextRows = [...rows];
    nextRows[idx] = { ...nextRows[idx]!, [field]: value };
    setRows(nextRows);
  };
  const removeRow = (idx: number) =>
    setRows(rows.filter((_, index) => index !== idx));

  return (
    <div className="space-y-4">
      <Card className="border-white/10">
        <CardHeader>
          <CardTitle className="text-base">{storeTitle}</CardTitle>
          <CardDescription>
            {storeDescription} {scopeDescription}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap gap-2">
            {(Object.keys(VARIABLE_SCOPE_META) as VariableScope[]).map(
              (value) => (
                <Button
                  key={value}
                  size="sm"
                  variant={scope === value ? "default" : "outline"}
                  onClick={() => setScope(value)}
                >
                  {VARIABLE_SCOPE_META[value].label}
                </Button>
              ),
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
            <Badge
              variant="outline"
              className={VARIABLE_SCOPE_META[scope].badgeClass}
            >
              {VARIABLE_SCOPE_META[scope].label}
            </Badge>
            <span>{VARIABLE_SCOPE_META[scope].description}</span>
          </div>
        </CardContent>
      </Card>

      <Card className="border-white/10">
        <CardHeader>
          <CardTitle className="text-base">
            Current {storeTitle} ({VARIABLE_SCOPE_META[scope].label})
          </CardTitle>
          <CardDescription>
            Values are masked after save. Delete a key here or replace the full
            scope below.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : existingVars?.length ? (
            <div className="space-y-2 font-mono text-sm">
              {existingVars.map((variable) => {
                const deleting =
                  deleteMutation.isPending &&
                  deleteMutation.variables === variable.key;

                return (
                  <div
                    key={variable.id}
                    className="flex flex-col gap-3 rounded-2xl border border-white/8 bg-black/10 p-3 md:flex-row md:items-center md:justify-between"
                  >
                    <div className="space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-foreground">{variable.key}</span>
                        <Badge
                          variant="outline"
                          className={
                            VARIABLE_SCOPE_META[variable.environment].badgeClass
                          }
                        >
                          {VARIABLE_SCOPE_META[variable.environment].label}
                        </Badge>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        {variable.value}
                      </p>
                    </div>

                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => deleteMutation.mutate(variable.key)}
                      disabled={deleteMutation.isPending}
                    >
                      {deleting ? (
                        <LoaderCircle className="h-4 w-4 animate-spin" />
                      ) : (
                        <Trash2 className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                );
              })}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{emptyState}</p>
          )}
        </CardContent>
      </Card>

      <Card className="border-white/10">
        <CardHeader>
          <CardTitle className="text-base">
            Replace {storeTitle} ({VARIABLE_SCOPE_META[scope].label})
          </CardTitle>
          <CardDescription>{replaceDescription}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          {rows.map((row, idx) => (
            <div key={idx} className="flex gap-2">
              <Input
                placeholder="KEY"
                value={row.key}
                onChange={(e) =>
                  updateRow(idx, "key", e.target.value.toUpperCase())
                }
                className="font-mono"
              />
              <Input
                placeholder={isSecret ? "secret value" : "value"}
                type={isSecret ? "password" : "text"}
                value={row.value}
                onChange={(e) => updateRow(idx, "value", e.target.value)}
                className="font-mono"
              />
              <Button
                variant="ghost"
                size="icon"
                onClick={() => removeRow(idx)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
          <div className="flex flex-wrap gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={addRow}>
              Add Row
            </Button>
            <Button
              size="sm"
              onClick={() => saveMutation.mutate()}
              disabled={saveMutation.isPending}
            >
              {saveMutation.isPending
                ? "Saving..."
                : isSecret
                  ? "Save Secrets"
                  : "Save Env Vars"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
