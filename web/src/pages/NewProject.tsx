import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  Search,
  Lock,
  Globe,
  GitBranch,
  ArrowLeft,
  Webhook,
} from "lucide-react";
import { Link } from "@tanstack/react-router";
import { toast } from "sonner";
import { api } from "@/lib/api";
import {
  resolveAutoDeploySourceBranch,
  shouldEnableProductionAutoDeploy,
} from "@/lib/autodeploy";
import { queryKeys, staleTimes } from "@/lib/queryKeys";
import { useOrganizations } from "@/hooks/use-organizations";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { LoadingState, Spinner } from "@/components/ui/spinner";
import {
  BuildSettingsFields,
  getFrameworkDefaults,
} from "@/components/projects/build-settings";
import { PickApp } from "@/components/projects/pick-app";
import type { GitHubRepo, MonorepoApp, RepoInspection } from "@/types/api";

function buildSettingsFromApp(
  app: MonorepoApp,
  inspection: RepoInspection,
  prev: ReturnType<typeof getFrameworkDefaults>,
): ReturnType<typeof getFrameworkDefaults> {
  const seeded = getFrameworkDefaults(
    app.framework,
    inspection.package_manager,
  );
  return {
    ...prev,
    framework: app.framework || prev.framework,
    packageManager: inspection.package_manager || prev.packageManager,
    rootDirectory: app.path,
    outputDirectory: app.output_directory || seeded.outputDirectory,
    buildCommand: app.suggested_build_command || seeded.buildCommand,
    installCommand: seeded.installCommand,
  };
}

export function NewProject() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [appPicked, setAppPicked] = useState(false);
  const [name, setName] = useState("");
  const [branch, setBranch] = useState("");
  const {
    organizations,
    selectedOrganization,
    selectedOrganizationId,
    setSelectedOrganizationId,
    isLoading: organizationsLoading,
  } = useOrganizations();
  const [buildSettings, setBuildSettings] = useState(() =>
    getFrameworkDefaults("nextjs", "auto"),
  );
  const [autoBuildEnabled, setAutoBuildEnabled] = useState(true);
  const [autoProductionEnabled, setAutoProductionEnabled] = useState(false);
  const [attachPostgres, setAttachPostgres] = useState(false);

  const { data: repos, isLoading: reposLoading } = useQuery({
    queryKey: queryKeys.githubRepos(),
    queryFn: () => api.listGithubRepos(),
    staleTime: staleTimes.github,
  });

  const { data: branches } = useQuery({
    queryKey: queryKeys.githubBranches(
      selectedRepo?.owner.login ?? "",
      selectedRepo?.name ?? "",
    ),
    queryFn: () =>
      api.listGithubBranches(selectedRepo!.owner.login, selectedRepo!.name),
    enabled: !!selectedRepo,
    staleTime: staleTimes.github,
  });

  const { data: inspection, isLoading: inspectionLoading } = useQuery({
    queryKey: queryKeys.repoInspection(
      selectedRepo?.owner.login ?? "",
      selectedRepo?.name ?? "",
      branch || selectedRepo?.default_branch || "",
    ),
    queryFn: () =>
      api.inspectRepo(
        selectedRepo!.owner.login,
        selectedRepo!.name,
        branch || selectedRepo!.default_branch,
      ),
    enabled: !!selectedRepo && !!(branch || selectedRepo?.default_branch),
    staleTime: 5 * 60_000,
    retry: 1,
  });

  // Auto-prefill on single-app inspection result — skip State B
  useEffect(() => {
    if (!inspection || appPicked || inspection.is_monorepo) return;
    const firstApp = inspection.apps[0];
    if (!firstApp) return;
    setBuildSettings((prev) =>
      buildSettingsFromApp(firstApp, inspection, prev),
    );
    setAppPicked(true);
  }, [inspection, appPicked]);

  const createMutation = useMutation({
    mutationFn: () =>
      api.createProject({
        organization_id: selectedOrganizationId ?? undefined,
        name,
        github_repo: selectedRepo!.name,
        github_owner: selectedRepo!.owner.login,
        branch: resolveAutoDeploySourceBranch(
          branch,
          selectedRepo!.default_branch,
        ),
        framework: buildSettings.framework,
        package_manager: buildSettings.packageManager,
        root_directory: buildSettings.rootDirectory,
        output_directory: buildSettings.outputDirectory,
        install_command: buildSettings.installCommand,
        build_command: buildSettings.buildCommand,
        node_version: buildSettings.nodeVersion,
        port: buildSettings.port,
        start_command: buildSettings.startCommand,
        health_path: buildSettings.healthPath,
        auto_build_enabled: autoBuildEnabled,
        auto_production_enabled: shouldEnableProductionAutoDeploy(
          autoBuildEnabled,
          autoProductionEnabled,
        ),
      }),
    onSuccess: async (project) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });

      if (attachPostgres) {
        try {
          await api.attachService(project.id, {
            environment: "preview",
            type: "postgres",
          });
          await api.attachService(project.id, {
            environment: "production",
            type: "postgres",
          });
          toast.success(
            "Project created with Postgres attached for both environments.",
          );
        } catch (err) {
          // Best-effort: the project is created, so the user can attach
          // manually from Services if this failed. Surface a toast.
          toast.error(
            "Project created, but Postgres attach failed: " +
              (err as Error).message,
          );
        }
      } else {
        toast.success("Project created");
      }

      navigate({
        to: "/projects/$id",
        params: { id: project.id },
      });
    },
    onError: (err) => toast.error(err.message),
  });

  const filteredRepos = repos?.filter(
    (r) =>
      r.full_name.toLowerCase().includes(search.toLowerCase()) ||
      r.name.toLowerCase().includes(search.toLowerCase()),
  );

  const handleAutoBuildEnabledChange = (checked: boolean) => {
    setAutoBuildEnabled(checked);
    if (!checked) {
      setAutoProductionEnabled(false);
    }
  };

  // State A: No repo selected — show RepoPicker
  if (!selectedRepo) {
    return (
      <div className="mx-auto w-full min-w-0 max-w-2xl p-6 pb-[calc(1.5rem+env(safe-area-inset-bottom))]">
        <Link
          to="/"
          className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to projects
        </Link>
        <h1 className="text-2xl font-bold tracking-tight">Import Repository</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Select a GitHub repository to deploy
        </p>

        <div className="relative mt-6">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search repositories..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>

        <div className="mt-4">
          {reposLoading ? (
            <LoadingState
              title="Loading repositories…"
              description="Fetching available GitHub repositories for import."
              className="min-h-[320px]"
            />
          ) : !filteredRepos?.length ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              {search ? "No matching repositories" : "No repositories found"}
            </p>
          ) : (
            <div className="rounded-lg border border-white/5">
              {filteredRepos.map((repo) => (
                <div
                  key={repo.id}
                  className="flex cursor-pointer items-center gap-3 border-b border-white/5 px-5 py-3.5 transition-colors last:border-b-0 hover:bg-muted/50"
                  onClick={() => {
                    setSelectedRepo(repo);
                    setAppPicked(false);
                    setName(
                      repo.name.toLowerCase().replace(/[^a-z0-9-]/g, "-"),
                    );
                    setBranch(repo.default_branch);
                    setBuildSettings(getFrameworkDefaults("nextjs", "auto"));
                    setAutoBuildEnabled(true);
                    setAutoProductionEnabled(false);
                    setAttachPostgres(false);
                  }}
                >
                  {repo.private ? (
                    <Lock className="h-4 w-4 shrink-0 text-muted-foreground" />
                  ) : (
                    <Globe className="h-4 w-4 shrink-0 text-muted-foreground" />
                  )}
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-sm font-medium">
                      {repo.full_name}
                    </p>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span className="flex items-center gap-1 font-mono">
                        <GitBranch className="h-3 w-3" />
                        {repo.default_branch}
                      </span>
                      {repo.language && (
                        <Badge
                          variant="outline"
                          className="px-1.5 py-0 text-xs"
                        >
                          {repo.language}
                        </Badge>
                      )}
                    </div>
                  </div>
                  <Button variant="outline" size="sm">
                    Import
                  </Button>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }

  if (inspectionLoading && !appPicked) {
    return (
      <div className="mx-auto w-full min-w-0 max-w-2xl p-6">
        <LoadingState
          title="Inspecting repository…"
          description="Detecting groups, frameworks, and build commands."
          className="min-h-[320px]"
        />
      </div>
    );
  }

  // State B: monorepo detected, user hasn't picked an app yet
  if (inspection?.is_monorepo && !appPicked) {
    return (
      <PickApp
        inspection={inspection}
        repoFullName={`${selectedRepo.owner.login}/${selectedRepo.name}`}
        onSelectApp={(app) => {
          setBuildSettings((prev) =>
            buildSettingsFromApp(app, inspection, prev),
          );
          setAppPicked(true);
        }}
        onSetManually={() => {
          setAppPicked(true);
        }}
        onBack={() => {
          setSelectedRepo(null);
          setAppPicked(false);
        }}
      />
    );
  }

  // State C: Configure (existing step 2, structurally unchanged)
  return (
    <div className="mx-auto w-full min-w-0 max-w-2xl p-6 pb-[calc(1.5rem+env(safe-area-inset-bottom))]">
      <button
        onClick={() => {
          if (inspection?.is_monorepo) {
            setAppPicked(false);
          } else {
            setSelectedRepo(null);
            setAppPicked(false);
          }
        }}
        className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {inspection?.is_monorepo
          ? "Back to app selection"
          : "Back to repository selection"}
      </button>

      <h1 className="text-2xl font-bold tracking-tight">Configure Project</h1>
      <p className="mt-1 font-mono text-sm text-muted-foreground">
        {selectedRepo.owner.login}/{selectedRepo.name}
      </p>

      <div className="mt-8 space-y-6">
        <div>
          <h3 className="border-b border-white/5 pb-2 text-sm font-semibold text-foreground">
            Project Settings
          </h3>
          <p className="mt-3 text-sm text-muted-foreground">
            Configure how your project will be built and deployed
          </p>
        </div>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="organization">Group</Label>
            <Select
              value={selectedOrganizationId ?? undefined}
              onValueChange={setSelectedOrganizationId}
            >
              <SelectTrigger id="organization">
                <SelectValue
                  placeholder={
                    organizationsLoading ? "Loading groups..." : "Select group"
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {organizations.map((organization) => (
                  <SelectItem key={organization.id} value={organization.id}>
                    {organization.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {selectedOrganization
                ? `This project will live in ${selectedOrganization.name}.`
                : "Choose which group this project should use."}
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="name">Project Name</Label>
            <Input
              id="name"
              value={name}
              onChange={(e) =>
                setName(
                  e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-"),
                )
              }
              placeholder="my-app"
            />
            <p className="text-xs text-muted-foreground">
              Used as subdomain:{" "}
              <span className="font-mono font-medium">
                {name || "my-app"}.preview.example.com
              </span>
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="branch">Branch</Label>
            <Select value={branch} onValueChange={setBranch}>
              <SelectTrigger>
                <SelectValue placeholder="Select branch" />
              </SelectTrigger>
              <SelectContent>
                {branches?.map((b) => (
                  <SelectItem key={b.name} value={b.name}>
                    {b.name}
                  </SelectItem>
                )) ?? (
                  <SelectItem value={selectedRepo.default_branch}>
                    {selectedRepo.default_branch}
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-3 rounded-lg border p-4">
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-start gap-3">
                <Webhook className="mt-0.5 h-4 w-4 text-muted-foreground" />
                <div className="space-y-1">
                  <Label htmlFor="auto-build-enabled">
                    Auto-deploy previews from source branch
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    Set up GitHub push deploys for the selected/default branch
                    after import.
                  </p>
                </div>
              </div>
              <Switch
                id="auto-build-enabled"
                checked={autoBuildEnabled}
                onCheckedChange={handleAutoBuildEnabledChange}
              />
            </div>

            <div className="flex items-center justify-between gap-4 border-t border-white/5 pt-3">
              <div className="space-y-1">
                <Label htmlFor="auto-production-enabled">
                  Also auto-deploy production
                </Label>
                <p className="text-xs text-muted-foreground">
                  When enabled, pushes to{" "}
                  <span className="font-mono">
                    {resolveAutoDeploySourceBranch(
                      branch,
                      selectedRepo.default_branch,
                    )}
                  </span>{" "}
                  create both preview and production deployments from the same
                  commit.
                </p>
              </div>
              <Switch
                id="auto-production-enabled"
                checked={autoBuildEnabled && autoProductionEnabled}
                onCheckedChange={setAutoProductionEnabled}
                disabled={!autoBuildEnabled}
              />
            </div>
          </div>
        </div>

        <BuildSettingsFields
          value={buildSettings}
          onChange={setBuildSettings}
        />

        <div className="flex items-start justify-between gap-3 rounded-2xl border border-white/10 bg-black/10 p-4">
          <div className="space-y-1">
            <Label htmlFor="attach-postgres" className="text-sm">
              Attach Postgres database
            </Label>
            <p className="text-xs text-muted-foreground">
              Each environment (preview + production) gets its own postgres:16
              container with a persistent volume.{" "}
              <code className="rounded bg-white/10 px-1 py-0.5">
                DATABASE_URL
              </code>{" "}
              is auto-injected.
            </p>
          </div>
          <Switch
            id="attach-postgres"
            checked={attachPostgres}
            onCheckedChange={setAttachPostgres}
          />
        </div>

        <div className="flex flex-col-reverse gap-3 border-t border-white/5 pt-6 md:flex-row md:justify-end">
          <Button
            variant="outline"
            className="h-11 w-full md:h-9 md:w-auto"
            onClick={() => setSelectedRepo(null)}
          >
            Cancel
          </Button>
          <Button
            className="h-11 w-full md:h-9 md:w-auto"
            onClick={() => createMutation.mutate()}
            disabled={
              !name ||
              !selectedOrganizationId ||
              createMutation.isPending ||
              organizationsLoading
            }
          >
            {createMutation.isPending ? <Spinner className="size-3.5" /> : null}
            {createMutation.isPending ? "Creating…" : "Create Project"}
          </Button>
        </div>
      </div>
    </div>
  );
}
