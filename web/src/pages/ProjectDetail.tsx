import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams, Link, useNavigate } from "@tanstack/react-router";
import {
  Activity,
  ArrowLeft,
  BarChart3,
  Building2,
  CheckCircle2,
  CircleDot,
  Copy,
  ExternalLink,
  GitBranch,
  GitCommit,
  Globe2,
  GlobeLock,
  LayoutGrid,
  Link2,
  Logs,
  Settings,
  Rocket,
  LoaderCircle,
  Sparkles,
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
import { AUDIENCE_STATUS_META } from "@/components/projects/project-analytics-meta";
import { ProjectAnalyticsTab } from "@/components/projects/project-analytics";
import { ProjectIntegrationTab } from "@/components/projects/project-integration";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Card,
  CardAction,
  CardContent,
  CardFooter,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { type ReactNode, useEffect, useState } from "react";
import { LoadingState } from "@/components/ui/spinner";
import { useIsMobile } from "@/hooks/use-mobile";
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

type ProjectTabValue =
  | "overview"
  | "deployments"
  | "analytics"
  | "integration"
  | "settings";

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

function buildReleaseTagName() {
  const now = new Date();
  const pad = (value: number) => value.toString().padStart(2, "0");

  return [
    "release",
    `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`,
    `${pad(now.getHours())}${pad(now.getMinutes())}`,
  ].join("-");
}

function formatCompactNumber(value: number) {
  return new Intl.NumberFormat("en", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

export function ProjectDetail() {
  const { id } = useParams({ strict: false }) as { id: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const isMobile = useIsMobile();
  const [activeTab, setActiveTab] = useState<ProjectTabValue>("overview");
  const [releaseSheetOpen, setReleaseSheetOpen] = useState(false);
  const [releaseTagName, setReleaseTagName] = useState(buildReleaseTagName());

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
      setActiveTab("deployments");
    },
    onError: (err) => toast.error(err.message),
  });

  if (isLoading) {
    return (
      <div className="p-6">
        <LoadingState
          title="Loading project…"
          description="Fetching project details, deployments, and public endpoints."
          className="min-h-[420px]"
        />
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

  const latestDeployment = deployments?.[0] ?? null;

  const openLatestLogs = () => {
    if (!latestDeployment) return;
    navigate({
      to: "/projects/$id/deployments/$did",
      params: { id, did: latestDeployment.id },
    });
  };

  return (
    <div className="space-y-5 p-6">
      <ProjectCommandBar
        project={project}
        latestDeployment={latestDeployment}
        isDeployPending={deploymentMutation.isPending}
        onDeployPreview={() =>
          deploymentMutation.mutate({ environment: "preview" })
        }
        onRelease={() => {
          setReleaseTagName(buildReleaseTagName());
          setReleaseSheetOpen(true);
        }}
        onOpenLogs={openLatestLogs}
      />

      <Tabs
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as ProjectTabValue)}
      >
        <TabsList
          variant="line"
          className="h-auto w-full justify-start gap-1 overflow-x-auto"
        >
          <TabsTrigger value="overview">
            <LayoutGrid className="mr-1.5 h-3.5 w-3.5" />
            Overview
          </TabsTrigger>
          <TabsTrigger value="deployments">
            <Rocket className="mr-1.5 h-3.5 w-3.5" />
            Deployments
          </TabsTrigger>
          <TabsTrigger value="analytics">
            <BarChart3 className="mr-1.5 h-3.5 w-3.5" />
            Analytics
          </TabsTrigger>
          <TabsTrigger value="integration">
            <Sparkles className="mr-1.5 h-3.5 w-3.5" />
            Integration
          </TabsTrigger>
          <TabsTrigger value="settings">
            <Settings className="mr-1.5 h-3.5 w-3.5" />
            Settings
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="mt-4">
          <OverviewTab
            projectId={id}
            project={project}
            deployments={deployments}
            domains={domains}
            isLoading={deploymentsLoading || domainsLoading}
            onShowDeployments={() => setActiveTab("deployments")}
            onShowIntegration={() => setActiveTab("integration")}
          />
        </TabsContent>

        <TabsContent value="deployments" className="mt-4">
          <DeploymentsTab
            projectId={id}
            deployments={deployments}
            domains={domains}
            isLoading={deploymentsLoading}
            onDeployPreview={() =>
              deploymentMutation.mutate({ environment: "preview" })
            }
            onRelease={() => {
              setReleaseTagName(buildReleaseTagName());
              setReleaseSheetOpen(true);
            }}
            isActionPending={deploymentMutation.isPending}
          />
        </TabsContent>

        <TabsContent value="analytics" className="mt-4">
          <ProjectAnalyticsTab
            projectId={id}
            onSetupAnalytics={() => setActiveTab("integration")}
          />
        </TabsContent>

        <TabsContent value="integration" className="mt-4">
          <ProjectIntegrationTab projectId={id} />
        </TabsContent>

        <TabsContent value="settings" className="mt-4">
          <SettingsTab
            projectId={id}
            project={project}
            domains={domains}
            isDomainsLoading={domainsLoading}
            onDelete={() => deleteMutation.mutate()}
          />
        </TabsContent>
      </Tabs>

      {isMobile ? (
        <Drawer open={releaseSheetOpen} onOpenChange={setReleaseSheetOpen}>
          <DrawerContent className="border-white/10 bg-[#0b1220]/98">
            <DrawerHeader>
              <DrawerTitle>Release to Production</DrawerTitle>
              <DrawerDescription>
                Deployik will create a git tag and queue a production deployment
                from that tagged ref.
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
                {deploymentMutation.isPending ? "Queueing release…" : "Release"}
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
                Deployik will create a git tag and queue a production deployment
                from that tagged ref.
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
                {deploymentMutation.isPending ? "Queueing release…" : "Release"}
              </Button>
            </SheetFooter>
          </SheetContent>
        </Sheet>
      )}
    </div>
  );
}

function ReleasePanelContent({
  project,
  domains,
  releaseTagName,
  onReleaseTagChange,
}: {
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
  domains: Domain[] | undefined;
  releaseTagName: string;
  onReleaseTagChange: (value: string) => void;
}) {
  return (
    <div className="mt-6 grid gap-4 px-4 lg:grid-cols-[minmax(0,0.8fr)_minmax(280px,1fr)] lg:px-0">
      <div className="space-y-4">
        <div className="rounded-xl border border-white/8 bg-black/10 p-4">
          <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">
            Source branch
          </p>
          <p className="mt-2 text-sm font-medium text-foreground">
            {project.branch}
          </p>
          <p className="mt-2 text-sm text-muted-foreground">
            Repository: {project.github_owner}/{project.github_repo}
          </p>
        </div>

        <div className="rounded-xl border border-white/8 bg-black/10 p-4">
          <Label htmlFor="release-tag">Release tag</Label>
          <Input
            id="release-tag"
            value={releaseTagName}
            onChange={(event) => onReleaseTagChange(event.target.value)}
            className="mt-3"
            placeholder="release-20260402-1455"
          />
          <p className="mt-3 text-sm text-muted-foreground">
            This tag becomes the production deploy ref so the released build
            stays traceable.
          </p>
        </div>
      </div>

      <div className="space-y-4">
        <div className="rounded-xl border border-white/8 bg-black/10 p-4">
          <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">
            Production endpoints
          </p>
          <div className="mt-3 space-y-2">
            {getReadyEnvironmentDomains(domains, "production").length ? (
              getReadyEnvironmentDomains(domains, "production").map(
                (domain) => (
                  <div
                    key={domain.id}
                    className="rounded-xl border border-white/8 px-3 py-2 text-sm text-foreground"
                  >
                    {domain.domain}
                  </div>
                ),
              )
            ) : (
              <div className="rounded-xl border border-dashed border-white/10 px-3 py-6 text-sm text-muted-foreground">
                No verified production domain yet. The release will still build
                and deploy, but no public production URL is active.
              </div>
            )}
          </div>
        </div>

        <div className="rounded-xl border border-primary/15 bg-primary/10 p-4 text-sm text-slate-100">
          Release is the intentional production action. Preview remains the fast
          iteration path.
        </div>
      </div>
    </div>
  );
}

function ProjectCommandBar({
  project,
  latestDeployment,
  isDeployPending,
  onDeployPreview,
  onRelease,
  onOpenLogs,
}: {
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
  latestDeployment: Deployment | null;
  isDeployPending: boolean;
  onDeployPreview: () => void;
  onRelease: () => void;
  onOpenLogs: () => void;
}) {
  return (
    <Card className="@container/card">
      <CardHeader>
        <div className="min-w-0 space-y-3">
          <Button
            asChild
            variant="ghost"
            size="sm"
            className="-ml-2 w-fit text-muted-foreground"
          >
            <Link to="/">
              <ArrowLeft className="h-4 w-4" />
              Back to Projects
            </Link>
          </Button>
          <div className="flex flex-wrap items-center gap-2">
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
            <Badge
              variant="outline"
              className="border-primary/20 bg-primary/10 font-mono text-primary"
            >
              {formatFrameworkLabel(project.framework)}
            </Badge>
          </div>
          <div className="min-w-0">
            <CardTitle className="text-xl tracking-tight sm:text-2xl">
              {project.name}
            </CardTitle>
            <CardDescription className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
              <span className="min-w-0 truncate">
                {project.github_owner}/{project.github_repo}
              </span>
              <span className="flex items-center gap-1">
                <GitBranch className="h-3.5 w-3.5" />
                {project.branch}
              </span>
              {project.organization_name ? (
                <span className="flex items-center gap-1">
                  <Building2 className="h-3.5 w-3.5" />
                  {project.organization_name}
                </span>
              ) : null}
            </CardDescription>
          </div>
        </div>

        <CardAction className="flex flex-wrap gap-2 @[760px]/card:justify-end">
          <Button onClick={onDeployPreview} disabled={isDeployPending}>
            <Rocket className="mr-1.5 h-3.5 w-3.5" />
            Deploy Preview
          </Button>
          <Button
            variant="outline"
            onClick={onRelease}
            disabled={isDeployPending}
          >
            <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
            Release
          </Button>
          <Button
            variant="ghost"
            onClick={onOpenLogs}
            disabled={!latestDeployment}
          >
            <Logs className="mr-1.5 h-3.5 w-3.5" />
            Open Logs
          </Button>
        </CardAction>
      </CardHeader>
    </Card>
  );
}

function OverviewTab({
  projectId,
  project,
  deployments,
  domains,
  isLoading,
  onShowDeployments,
  onShowIntegration,
}: {
  projectId: string;
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
  deployments: Deployment[] | undefined;
  domains: Domain[] | undefined;
  isLoading: boolean;
  onShowDeployments: () => void;
  onShowIntegration: () => void;
}) {
  const timezone =
    Intl.DateTimeFormat().resolvedOptions().timeZone?.trim() || "UTC";
  const navigate = useNavigate();
  const latestDeployment = deployments?.[0] ?? null;
  const latestRelease = getLatestLiveEnvironmentDeployment(
    deployments,
    "production",
  );
  const latestPreview = getLatestEnvironmentDeployment(deployments, "preview");
  const latestProduction = getLatestEnvironmentDeployment(
    deployments,
    "production",
  );
  const previewUrl = getPrimaryEnvironmentUrl(domains, "preview");
  const productionUrl = getPrimaryEnvironmentUrl(domains, "production");
  const readyDomainCount = (domains ?? []).filter(isDomainReady).length;

  const { data: analytics, isLoading: analyticsLoading } = useQuery({
    queryKey: ["project-overview-analytics", projectId, timezone],
    queryFn: () =>
      api.getProjectAnalytics(projectId, {
        environment: "all",
        range: "24h",
        timezone,
      }),
  });
  const overviewAudienceMeta = AUDIENCE_STATUS_META[
    analytics?.audience.status ?? ""
  ] ??
    AUDIENCE_STATUS_META.ready_to_install ?? {
      label: "Ready to install",
      badgeClass: "border-primary/25 bg-primary/12 text-primary",
      description:
        "The website exists. Add the tracker to start collecting audience data.",
    };

  const copyUrl = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(`${label} copied`);
    } catch {
      toast.error(`Couldn't copy ${label.toLowerCase()}`);
    }
  };

  return (
    <div className="space-y-4">
      <Card className="@container/card">
        <CardHeader>
          <div className="flex flex-wrap items-center gap-2">
            <CardTitle className="text-base">Live Endpoints</CardTitle>
            <CardDescription>
              Quick public links to the current preview and production
              endpoints.
            </CardDescription>
          </div>
          <CardAction>
            <Badge variant="outline" className="hidden sm:inline-flex">
              {project.name}.preview.example.com
            </Badge>
          </CardAction>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 md:grid-cols-2">
            <LiveEndpointChip
              environment="preview"
              url={previewUrl}
              deployment={latestPreview}
              onCopy={copyUrl}
            />
            <LiveEndpointChip
              environment="production"
              url={productionUrl}
              deployment={latestProduction}
              onCopy={copyUrl}
            />
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        <OverviewStatCard
          label="Preview Health"
          value={
            latestPreview
              ? DEPLOYMENT_STATUS_META[latestPreview.status].label
              : "Not deployed"
          }
          icon={<Globe2 className="h-4 w-4" />}
          hint={
            previewUrl
              ? "Preview has an active public endpoint."
              : "Deploy preview to create a public staging URL."
          }
        />
        <OverviewStatCard
          label="Production Health"
          value={
            latestProduction
              ? DEPLOYMENT_STATUS_META[latestProduction.status].label
              : "Not released"
          }
          icon={<GlobeLock className="h-4 w-4" />}
          hint={
            productionUrl
              ? "Production has a verified domain."
              : "Release once production domains are ready."
          }
        />
        <OverviewStatCard
          label="Latest Release"
          value={latestRelease ? latestRelease.commit_sha.slice(0, 7) : "None"}
          icon={<Rocket className="h-4 w-4" />}
          hint={
            latestRelease
              ? `Released ${formatRelativeDate(latestRelease.created_at)}`
              : "No successful production release yet."
          }
        />
        <OverviewStatCard
          label="Active Domains"
          value={readyDomainCount.toString()}
          icon={<Link2 className="h-4 w-4" />}
          hint="Verified domains with active SSL."
        />
        <OverviewStatCard
          label="Traffic"
          value={
            analyticsLoading
              ? "—"
              : formatCompactNumber(analytics?.runtime.summary.requests ?? 0)
          }
          icon={<Activity className="h-4 w-4" />}
          hint="Requests over the last 24 hours."
        />
        <OverviewStatCard
          label="Analytics Status"
          value={analytics ? overviewAudienceMeta.label : "Loading"}
          icon={<BarChart3 className="h-4 w-4" />}
          hint={
            analytics?.audience.status === "ready_to_install"
              ? "Setup is still pending."
              : "Audience analytics status from Umami."
          }
        />
      </div>

      <Card className="@container/card overflow-hidden">
        <CardHeader>
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <Badge
                variant="outline"
                className={
                  latestDeployment
                    ? DEPLOYMENT_STATUS_META[latestDeployment.status].badgeClass
                    : "border-white/10 bg-white/5 text-slate-200"
                }
              >
                {latestDeployment
                  ? DEPLOYMENT_STATUS_META[latestDeployment.status].label
                  : "No deployments yet"}
              </Badge>
              {latestDeployment ? (
                <Badge
                  variant="outline"
                  className={
                    ENVIRONMENT_META[latestDeployment.environment].badgeClass
                  }
                >
                  {ENVIRONMENT_META[latestDeployment.environment].label}
                </Badge>
              ) : null}
            </div>
            <CardTitle className="text-base">Latest Deployment</CardTitle>
            <CardDescription>
              The newest build is the fastest way to read the current state of
              this project.
            </CardDescription>
          </div>
          <CardAction className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={onShowDeployments}>
              See All
            </Button>
            {analytics?.audience.status === "ready_to_install" ? (
              <Button onClick={onShowIntegration}>Setup Analytics</Button>
            ) : null}
          </CardAction>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <LoadingState
              title="Loading deployments…"
              description="Preparing the latest deployment and endpoint activity."
              className="min-h-[280px]"
            />
          ) : latestDeployment ? (
            <div className="grid gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(280px,0.9fr)]">
              <div className="rounded-xl border bg-muted/30 p-5">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                      <GitCommit className="h-4 w-4 text-muted-foreground" />
                      {latestDeployment.commit_sha
                        ? latestDeployment.commit_sha.slice(0, 7)
                        : "pending"}
                    </div>
                    <p
                      className="mt-3 truncate text-lg font-semibold text-foreground"
                      title={
                        latestDeployment.commit_message ||
                        latestDeployment.error_message
                      }
                    >
                      {latestDeployment.commit_message ||
                        latestDeployment.error_message ||
                        "Waiting for commit metadata"}
                    </p>
                  </div>
                </div>
                <div className="mt-5 grid gap-3 sm:grid-cols-3">
                  <MiniMeta label="Branch" value={latestDeployment.branch} />
                  <MiniMeta
                    label="Started"
                    value={formatRelativeDate(latestDeployment.created_at)}
                  />
                  <MiniMeta
                    label="Duration"
                    value={
                      latestDeployment.build_duration > 0
                        ? `${latestDeployment.build_duration}s`
                        : "—"
                    }
                  />
                </div>
              </div>

              <div className="space-y-3">
                <p className="text-xs uppercase tracking-[0.2em] text-muted-foreground">
                  Previous deployments
                </p>
                {(deployments ?? []).slice(1, 4).length ? (
                  (deployments ?? []).slice(1, 4).map((deployment) => (
                    <button
                      type="button"
                      key={deployment.id}
                      onClick={() =>
                        navigate({
                          to: "/projects/$id/deployments/$did",
                          params: { id: projectId, did: deployment.id },
                        })
                      }
                      className="flex w-full items-center justify-between gap-3 rounded-xl border bg-muted/30 px-4 py-3 text-left transition-colors hover:bg-accent"
                    >
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-foreground">
                          {deployment.commit_sha
                            ? deployment.commit_sha.slice(0, 7)
                            : deployment.id.slice(0, 8)}
                        </p>
                        <p className="truncate text-xs text-muted-foreground">
                          {deployment.commit_message ||
                            DEPLOYMENT_STATUS_META[deployment.status].label}
                        </p>
                      </div>
                      <div className="text-right">
                        <p className="text-xs text-muted-foreground">
                          {ENVIRONMENT_META[deployment.environment].label}
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {formatRelativeDate(deployment.created_at)}
                        </p>
                      </div>
                    </button>
                  ))
                ) : (
                  <div className="rounded-xl border border-dashed border-border/70 px-4 py-8 text-sm text-muted-foreground">
                    No previous deployments yet.
                  </div>
                )}
              </div>
            </div>
          ) : (
            <div className="rounded-xl border border-dashed border-border/70 px-5 py-12 text-sm text-muted-foreground">
              No deployments yet. Use the command bar to queue the first preview
              build.
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function LiveEndpointChip({
  environment,
  url,
  deployment,
  onCopy,
}: {
  environment: Domain["environment"];
  url: string | null;
  deployment: Deployment | undefined;
  onCopy: (value: string, label: string) => void;
}) {
  const isLive = Boolean(url);

  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border bg-muted/30 px-4 py-3">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "h-2.5 w-2.5 rounded-full",
              isLive ? "bg-emerald-400" : "bg-slate-500",
            )}
          />
          <p className="text-sm font-medium text-foreground">
            {ENVIRONMENT_META[environment].label}
          </p>
        </div>
        <p className="mt-1 truncate text-sm text-muted-foreground">
          {url || "Not live yet"}
        </p>
        <p className="mt-1 text-xs text-muted-foreground">
          {deployment
            ? DEPLOYMENT_STATUS_META[deployment.status].label
            : "No deployment yet"}
        </p>
      </div>
      <div className="flex shrink-0 gap-2">
        {url ? (
          <>
            <Button asChild size="sm" variant="ghost">
              <a href={url} target="_blank" rel="noopener noreferrer">
                Open
              </a>
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                onCopy(url, `${ENVIRONMENT_META[environment].label} URL`)
              }
            >
              <Copy className="h-3.5 w-3.5" />
            </Button>
          </>
        ) : (
          <Badge
            variant="outline"
            className="border-white/10 bg-white/5 text-slate-200"
          >
            Pending
          </Badge>
        )}
      </div>
    </div>
  );
}

function OverviewStatCard({
  label,
  value,
  icon,
  hint,
}: {
  label: string;
  value: string;
  icon: ReactNode;
  hint: string;
}) {
  return (
    <Card className="@container/card bg-gradient-to-t from-primary/5 to-card shadow-xs">
      <CardHeader>
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {value}
        </CardTitle>
        <CardAction className="text-muted-foreground">{icon}</CardAction>
      </CardHeader>
      <CardFooter className="flex-col items-start gap-1.5 text-sm">
        <p className="line-clamp-2 text-muted-foreground">{hint}</p>
      </CardFooter>
    </Card>
  );
}

function MiniMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border bg-background px-3 py-3">
      <p className="text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
        {label}
      </p>
      <p className="mt-2 truncate text-sm font-medium text-foreground">
        {value}
      </p>
    </div>
  );
}

function DeploymentsTab({
  projectId,
  deployments,
  domains,
  isLoading,
  onDeployPreview,
  onRelease,
  isActionPending,
}: {
  projectId: string;
  deployments: Deployment[] | undefined;
  domains: Domain[] | undefined;
  isLoading: boolean;
  onDeployPreview: () => void;
  onRelease: () => void;
  isActionPending: boolean;
}) {
  const navigate = useNavigate();

  const openDeploymentDetails = (deploymentId: string) => {
    navigate({
      to: "/projects/$id/deployments/$did",
      params: { id: projectId, did: deploymentId },
    });
  };

  return (
    <div className="space-y-4">
      <Card className="@container/card">
        <CardHeader>
          <div>
            <CardTitle className="text-base">Deployments</CardTitle>
            <CardDescription>
              Every deployment stays readable and row-clickable, with direct
              access to logs and live endpoints.
            </CardDescription>
          </div>
          <CardAction className="flex flex-wrap gap-2">
            <Button
              size="sm"
              onClick={onDeployPreview}
              disabled={isActionPending}
            >
              <Rocket className="mr-1.5 h-3.5 w-3.5" />
              Deploy Preview
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={onRelease}
              disabled={isActionPending}
            >
              <GlobeLock className="mr-1.5 h-3.5 w-3.5" />
              Release
            </Button>
          </CardAction>
        </CardHeader>
      </Card>

      {isLoading ? (
        <LoadingState
          title="Loading deployments…"
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
                          <span className="text-xs text-muted-foreground">
                            —
                          </span>
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
          </CardContent>
        </Card>
      )}
    </div>
  );
}

function SettingsTab({
  projectId,
  project,
  domains,
  isDomainsLoading,
  onDelete,
}: {
  projectId: string;
  project: NonNullable<Awaited<ReturnType<typeof api.getProject>>>;
  domains: Domain[] | undefined;
  isDomainsLoading: boolean;
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
      <Tabs defaultValue="build">
        <TabsList
          variant="line"
          className="h-auto w-full justify-start gap-1 overflow-x-auto"
        >
          <TabsTrigger value="build">Build</TabsTrigger>
          <TabsTrigger value="domains">Domains</TabsTrigger>
          <TabsTrigger value="env">Env Vars</TabsTrigger>
          <TabsTrigger value="secrets">Secrets</TabsTrigger>
          <TabsTrigger value="danger">Danger</TabsTrigger>
        </TabsList>

        <TabsContent value="build" className="mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Build Settings</CardTitle>
              <CardDescription>
                Source control branch, framework defaults, and package/runtime
                behavior for this project.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label>Workspace</Label>
                <Input
                  value={project.organization_name || "Personal"}
                  disabled
                />
              </div>
              <div className="space-y-2">
                <Label>Branch</Label>
                <Input
                  value={branch}
                  onChange={(event) => setBranch(event.target.value)}
                />
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
        </TabsContent>

        <TabsContent value="domains" className="mt-4">
          <DomainsTab
            projectId={projectId}
            domains={domains}
            isLoading={isDomainsLoading}
          />
        </TabsContent>

        <TabsContent value="env" className="mt-4">
          <EnvVarsTab projectId={projectId} />
        </TabsContent>

        <TabsContent value="secrets" className="mt-4">
          <SecretsTab projectId={projectId} />
        </TabsContent>

        <TabsContent value="danger" className="mt-4">
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
                      This will stop all running containers and remove the
                      project. This action cannot be undone.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={onDelete}>
                      Delete
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
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
            <LoadingState
              title="Loading domains…"
              description="Fetching custom domains, verification, and SSL state."
              className="min-h-[220px]"
            />
          ) : !domains?.length ? (
            <div className="rounded-xl border border-dashed border-border/70 px-4 py-6 text-sm text-muted-foreground">
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
                  className="flex flex-col gap-4 rounded-xl border bg-muted/30 p-4 md:flex-row md:items-center md:justify-between"
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
    domain ||
    (environment === "preview" ? "staging.example.com" : "example.com");
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
          Point the domain at this VPS, then click Verify. Preview domains
          should usually be subdomains; production is usually the real bought
          domain.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="rounded-xl border bg-muted/30 p-4">
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

          <div className="rounded-xl border bg-muted/30 p-4">
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
          <div className="rounded-xl border bg-muted/30 p-4">
            <p className="text-sm font-medium">GoDaddy</p>
            <div className="mt-3 space-y-2 text-sm text-muted-foreground">
              <p>1. Open your domain in GoDaddy and go to DNS.</p>
              <p>2. Add an A record for the host shown above.</p>
              <p>3. Set Points to to {dnsTargetIp || "the target VPS IP"}.</p>
              <p>
                4. For root domains, also add `www` and point it to the same
                place.
              </p>
              <p>5. Leave TTL on the default value.</p>
              <p>
                6. Remove conflicting A, AAAA, or forwarded records for the same
                host.
              </p>
            </div>
          </div>

          <div className="rounded-xl border bg-muted/30 p-4">
            <p className="text-sm font-medium">
              Vercel DNS / Domain bought on Vercel
            </p>
            <div className="mt-3 space-y-2 text-sm text-muted-foreground">
              <p>1. Open the domain in Vercel and go to DNS Records.</p>
              <p>2. Add an A record for the host shown above.</p>
              <p>3. Set the value to {dnsTargetIp || "the target VPS IP"}.</p>
              <p>
                4. For root domains, also create `www` and point it at the same
                target or CNAME it to the root.
              </p>
              <p>
                5. Remove old Vercel-specific records for the same host if they
                conflict.
              </p>
            </div>
          </div>
        </div>

        <div className="rounded-xl border border-dashed border-border/70 px-4 py-3 text-sm text-muted-foreground">
          <p>
            Deployik verifies A-record resolution to the VPS IP. If you use a
            subdomain, prefer an A record directly to the server. After DNS
            propagates, click Verify to issue SSL and activate the domain. Root
            domains are served without `www`; Deployik also issues SSL for the
            `www` variant and redirects it to the apex host.
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
      <Card>
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

      <Card>
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
            <LoadingState
              title={`Loading ${storeTitle.toLowerCase()}…`}
              description={`Fetching stored ${storeTitle.toLowerCase()} for the selected scope.`}
              className="min-h-[220px]"
            />
          ) : existingVars?.length ? (
            <div className="space-y-2 font-mono text-sm">
              {existingVars.map((variable) => {
                const deleting =
                  deleteMutation.isPending &&
                  deleteMutation.variables === variable.key;

                return (
                  <div
                    key={variable.id}
                    className="flex flex-col gap-3 rounded-xl border bg-muted/30 p-3 md:flex-row md:items-center md:justify-between"
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

      <Card>
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
