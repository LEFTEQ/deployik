import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { Search, Lock, Globe, GitBranch, ArrowLeft } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useOrganizations } from "@/hooks/use-organizations";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
import type { GitHubRepo } from "@/types/api";

export function NewProject() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
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

  const { data: repos, isLoading: reposLoading } = useQuery({
    queryKey: ["github-repos"],
    queryFn: () => api.listGithubRepos(),
  });

  const { data: branches } = useQuery({
    queryKey: [
      "github-branches",
      selectedRepo?.owner.login,
      selectedRepo?.name,
    ],
    queryFn: () =>
      api.listGithubBranches(selectedRepo!.owner.login, selectedRepo!.name),
    enabled: !!selectedRepo,
  });

  const createMutation = useMutation({
    mutationFn: () =>
      api.createProject({
        organization_id: selectedOrganizationId ?? undefined,
        name,
        github_repo: selectedRepo!.name,
        github_owner: selectedRepo!.owner.login,
        branch: branch || selectedRepo!.default_branch,
        framework: buildSettings.framework,
        package_manager: buildSettings.packageManager,
        root_directory: buildSettings.rootDirectory,
        output_directory: buildSettings.outputDirectory,
        install_command: buildSettings.installCommand,
        build_command: buildSettings.buildCommand,
        node_version: buildSettings.nodeVersion,
      }),
    onSuccess: (project) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] });
      toast.success("Project created");
      navigate({ to: "/projects/$id", params: { id: project.id } });
    },
    onError: (err) => toast.error(err.message),
  });

  const filteredRepos = repos?.filter(
    (r) =>
      r.full_name.toLowerCase().includes(search.toLowerCase()) ||
      r.name.toLowerCase().includes(search.toLowerCase()),
  );

  // Step 1: Select repo
  if (!selectedRepo) {
    return (
      <div className="mx-auto max-w-2xl p-6">
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

        <div className="mt-4 space-y-2">
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
            filteredRepos.map((repo) => (
              <Card
                key={repo.id}
                className="cursor-pointer transition-colors hover:border-primary/50"
                onClick={() => {
                  setSelectedRepo(repo);
                  setName(repo.name.toLowerCase().replace(/[^a-z0-9-]/g, "-"));
                  setBranch(repo.default_branch);
                  setBuildSettings(getFrameworkDefaults("nextjs", "auto"));
                }}
              >
                <CardContent className="flex items-center justify-between p-4">
                  <div className="flex items-center gap-3">
                    {repo.private ? (
                      <Lock className="h-4 w-4 text-muted-foreground" />
                    ) : (
                      <Globe className="h-4 w-4 text-muted-foreground" />
                    )}
                    <div>
                      <p className="text-sm font-medium">{repo.full_name}</p>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span className="flex items-center gap-1">
                          <GitBranch className="h-3 w-3" />
                          {repo.default_branch}
                        </span>
                        {repo.language && (
                          <Badge
                            variant="outline"
                            className="text-xs px-1.5 py-0"
                          >
                            {repo.language}
                          </Badge>
                        )}
                      </div>
                    </div>
                  </div>
                  <Button variant="outline" size="sm">
                    Import
                  </Button>
                </CardContent>
              </Card>
            ))
          )}
        </div>
      </div>
    );
  }

  // Step 2: Configure
  return (
    <div className="mx-auto max-w-2xl p-6">
      <button
        onClick={() => setSelectedRepo(null)}
        className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to repository selection
      </button>

      <h1 className="text-2xl font-bold tracking-tight">Configure Project</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        {selectedRepo.owner.login}/{selectedRepo.name}
      </p>

      <Card className="mt-6">
        <CardHeader>
          <CardTitle className="text-base">Project Settings</CardTitle>
          <CardDescription>
            Configure how your project will be built and deployed
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="organization">Workspace</Label>
            <Select
              value={selectedOrganizationId ?? undefined}
              onValueChange={setSelectedOrganizationId}
            >
              <SelectTrigger id="organization">
                <SelectValue
                  placeholder={
                    organizationsLoading
                      ? "Loading workspaces..."
                      : "Select workspace"
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
                : "Choose where this project should be visible."}
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
              <span className="font-medium">
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

          <BuildSettingsFields
            value={buildSettings}
            onChange={setBuildSettings}
          />

          <div className="flex justify-end gap-3 pt-4">
            <Button variant="outline" onClick={() => setSelectedRepo(null)}>
              Cancel
            </Button>
            <Button
              onClick={() => createMutation.mutate()}
              disabled={
                !name ||
                !selectedOrganizationId ||
                createMutation.isPending ||
                organizationsLoading
              }
            >
              {createMutation.isPending ? (
                <Spinner className="size-3.5" />
              ) : null}
              {createMutation.isPending ? "Creating…" : "Create Project"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
