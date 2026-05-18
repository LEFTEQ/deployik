import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ChevronRight,
  Plus,
  Search,
  Settings2,
  Trash2,
  UserPlus,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useGroups } from "@/hooks/use-groups";
import {
  DEPLOYMENT_STATUS_META,
  ENVIRONMENT_META,
  formatRelativeDate,
} from "@/lib/deployment-helpers";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LoadingState } from "@/components/ui/spinner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import type {
  DeploymentStatus,
  Group,
  GroupInvite,
  GroupMember,
  Project,
} from "@/types/api";

type GroupRole = "owner" | "member";

function ProjectTableRow({ project }: { project: Project }) {
  const navigate = useNavigate();
  const status = project.latest_deployment_status as DeploymentStatus | null;
  const statusMeta =
    status && status in DEPLOYMENT_STATUS_META
      ? DEPLOYMENT_STATUS_META[status]
      : null;
  const environment = project.latest_deployment_environment as
    | keyof typeof ENVIRONMENT_META
    | null;
  const environmentMeta = environment ? ENVIRONMENT_META[environment] : null;

  const open = () => navigate({ to: "/projects/$id", params: { id: project.id } });

  return (
    <TableRow
      className="cursor-pointer border-white/8 transition-colors hover:bg-white/[0.04] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary/40"
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
      <TableCell className="pl-6">
        <div className="flex items-center gap-3">
          <span
            className={cn(
              "h-2 w-2 shrink-0 rounded-full",
              project.status === "active" ? "bg-emerald-400" : "bg-slate-500",
            )}
          />
          <span className="truncate text-sm font-semibold text-foreground">
            {project.name}
          </span>
        </div>
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        <span className="font-mono">
          {project.github_owner}/{project.github_repo}
        </span>
        <span className="mx-1.5">·</span>
        <span className="font-mono">{project.branch}</span>
      </TableCell>
      <TableCell>
        {environmentMeta && statusMeta ? (
          <div className="flex flex-wrap items-center gap-1.5">
            <Badge variant="outline" className={environmentMeta.badgeClass}>
              {environmentMeta.label}
            </Badge>
            <Badge variant="outline" className={statusMeta.badgeClass}>
              {statusMeta.label}
            </Badge>
          </div>
        ) : (
          <span className="text-xs text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {formatRelativeDate(project.updated_at)}
      </TableCell>
      <TableCell className="pr-6 text-right">
        <ChevronRight className="ml-auto h-4 w-4 text-muted-foreground" />
      </TableCell>
    </TableRow>
  );
}

function invalidateGroupQueries(queryClient: ReturnType<typeof useQueryClient>) {
  queryClient.invalidateQueries({ queryKey: queryKeys.groups() });
  queryClient.invalidateQueries({ queryKey: queryKeys.organizations() });
  queryClient.invalidateQueries({ queryKey: ["projects"] });
  queryClient.invalidateQueries({ queryKey: ["command-projects"] });
}

function ProjectSelectionList({
  projects,
  selectedProjectIds,
  alreadyInGroupIds,
  onToggle,
  search,
  onSearchChange,
}: {
  projects: Project[];
  selectedProjectIds: Set<string>;
  alreadyInGroupIds?: Set<string>;
  onToggle: (projectId: string, checked: boolean) => void;
  search: string;
  onSearchChange: (value: string) => void;
}) {
  const trimmedSearch = search.trim().toLowerCase();
  const filteredProjects = useMemo(() => {
    if (!trimmedSearch) return projects;
    const tokens = trimmedSearch.split(/\s+/).filter(Boolean);
    return projects.filter((project) => {
      const haystack = [
        project.name,
        project.organization_name ?? "",
        `${project.github_owner}/${project.github_repo}`,
        project.branch,
      ]
        .join(" ")
        .toLowerCase();
      return tokens.every((token) => haystack.includes(token));
    });
  }, [projects, trimmedSearch]);

  return (
    <div className="space-y-3">
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={search}
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder="Search projects, repos, branches…"
          className="pl-9"
        />
      </div>
      <div className="max-h-72 overflow-y-auto rounded-md border">
        {filteredProjects.length ? (
          filteredProjects.map((project) => {
            const alreadyInGroup = alreadyInGroupIds?.has(project.id) ?? false;
            const checked = selectedProjectIds.has(project.id) || alreadyInGroup;
            return (
              <label
                key={project.id}
                className={cn(
                  "flex cursor-pointer items-center gap-3 border-b px-3 py-3 last:border-b-0 hover:bg-muted/40",
                  alreadyInGroup && "cursor-default bg-muted/20",
                )}
              >
                <Checkbox
                  checked={checked}
                  disabled={alreadyInGroup}
                  onCheckedChange={(value) =>
                    onToggle(project.id, value === true)
                  }
                  aria-label={`Move ${project.name}`}
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {project.name}
                  </span>
                  <span className="block truncate font-mono text-xs text-muted-foreground">
                    {project.github_owner}/{project.github_repo} · {project.branch}
                  </span>
                </span>
                {alreadyInGroup ? (
                  <Badge variant="outline">In group</Badge>
                ) : project.organization_name ? (
                  <span className="max-w-32 truncate text-xs text-muted-foreground">
                    {project.organization_name}
                  </span>
                ) : null}
              </label>
            );
          })
        ) : (
          <div className="py-10 text-center text-sm text-muted-foreground">
            No projects match this search.
          </div>
        )}
      </div>
    </div>
  );
}

function MembersTab({
  members,
  invites,
  isLoading,
  onRoleChange,
  onRemoveMember,
  onCreateInvite,
  onCancelInvite,
  isMutating,
}: {
  members: GroupMember[];
  invites: GroupInvite[];
  isLoading: boolean;
  onRoleChange: (member: GroupMember, role: GroupRole) => void;
  onRemoveMember: (member: GroupMember) => void;
  onCreateInvite: (username: string, role: GroupRole) => void;
  onCancelInvite: (invite: GroupInvite) => void;
  isMutating: boolean;
}) {
  const [githubUsername, setGithubUsername] = useState("");
  const [role, setRole] = useState<GroupRole>("member");
  const ownerCount = members.filter((member) => member.role === "owner").length;

  const submitInvite = () => {
    const trimmed = githubUsername.trim();
    if (!trimmed) return;
    onCreateInvite(trimmed, role);
    setGithubUsername("");
    setRole("member");
  };

  return (
    <div className="space-y-5">
      <div className="grid gap-3 rounded-md border p-3 sm:grid-cols-[1fr_140px_auto]">
        <Input
          value={githubUsername}
          onChange={(event) => setGithubUsername(event.target.value)}
          placeholder="GitHub username"
          aria-label="GitHub username"
        />
        <Select value={role} onValueChange={(value) => setRole(value as GroupRole)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="member">Member</SelectItem>
            <SelectItem value="owner">Owner</SelectItem>
          </SelectContent>
        </Select>
        <Button
          type="button"
          onClick={submitInvite}
          disabled={!githubUsername.trim() || isMutating}
        >
          <UserPlus className="h-4 w-4" />
          Invite
        </Button>
      </div>

      {isLoading ? (
        <LoadingState title="Loading members…" className="min-h-48" />
      ) : (
        <div className="rounded-md border">
          {members.map((member) => {
            const isLastOwner =
              member.role === "owner" && ownerCount <= 1;
            return (
              <div
                key={member.user_id}
                className="flex flex-col gap-3 border-b px-3 py-3 last:border-b-0 sm:flex-row sm:items-center"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">
                    {member.username}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    Active member
                  </div>
                </div>
                <Select
                  value={member.role}
                  onValueChange={(value) =>
                    onRoleChange(member, value as GroupRole)
                  }
                  disabled={isMutating || isLastOwner}
                >
                  <SelectTrigger className="w-full sm:w-32">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="member">Member</SelectItem>
                    <SelectItem value="owner">Owner</SelectItem>
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => onRemoveMember(member)}
                  disabled={isMutating || isLastOwner}
                >
                  Remove
                </Button>
              </div>
            );
          })}
          {!members.length ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              No members found.
            </div>
          ) : null}
        </div>
      )}

      <div>
        <h3 className="mb-2 text-sm font-semibold">Pending invitations</h3>
        <div className="rounded-md border">
          {invites.map((invite) => (
            <div
              key={invite.id}
              className="flex flex-col gap-3 border-b px-3 py-3 last:border-b-0 sm:flex-row sm:items-center"
            >
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">
                  {invite.github_username}
                </div>
                <div className="text-xs text-muted-foreground">
                  Pending · {invite.role}
                </div>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => onCancelInvite(invite)}
                disabled={isMutating}
              >
                Cancel
              </Button>
            </div>
          ))}
          {!invites.length ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No pending invitations.
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function GroupDialog({
  mode,
  open,
  onOpenChange,
  group,
  projects,
}: {
  mode: "create" | "edit";
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group?: Group | null;
  projects: Project[];
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [projectSearch, setProjectSearch] = useState("");
  const [selectedProjectIds, setSelectedProjectIds] = useState<Set<string>>(
    () => new Set(),
  );

  useEffect(() => {
    if (!open) return;
    setName(mode === "edit" ? group?.name ?? "" : "");
    setProjectSearch("");
    setSelectedProjectIds(new Set());
  }, [group?.id, group?.name, mode, open]);

  const membersQuery = useQuery({
    queryKey: queryKeys.groupMembers(group?.id ?? ""),
    queryFn: () => api.listGroupMembers(group!.id),
    enabled: open && mode === "edit" && !!group,
  });

  const invitesQuery = useQuery({
    queryKey: queryKeys.groupInvites(group?.id ?? ""),
    queryFn: () => api.listGroupInvites(group!.id),
    enabled: open && mode === "edit" && !!group,
  });

  const alreadyInGroupIds = useMemo(() => {
    if (mode !== "edit" || !group) return undefined;
    return new Set(
      projects
        .filter((project) => project.organization_id === group.id)
        .map((project) => project.id),
    );
  }, [group, mode, projects]);

  const selectedProjects = useMemo(
    () => projects.filter((project) => selectedProjectIds.has(project.id)),
    [projects, selectedProjectIds],
  );

  const toggleProject = (projectId: string, checked: boolean) => {
    setSelectedProjectIds((current) => {
      const next = new Set(current);
      if (checked) {
        next.add(projectId);
      } else {
        next.delete(projectId);
      }
      return next;
    });
  };

  const createMutation = useMutation({
    mutationFn: () =>
      api.createGroup({
        name: name.trim(),
        project_ids: Array.from(selectedProjectIds),
      }),
    onSuccess: () => {
      invalidateGroupQueries(queryClient);
      toast.success("Group created");
      onOpenChange(false);
    },
    onError: (err) => toast.error(err.message),
  });

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!group) throw new Error("Group is missing");
      if (name.trim() !== group.name) {
        await api.updateGroup(group.id, { name: name.trim() });
      }
      if (selectedProjectIds.size > 0) {
        await api.moveProjectsToGroup(group.id, Array.from(selectedProjectIds));
      }
    },
    onSuccess: () => {
      invalidateGroupQueries(queryClient);
      toast.success("Group updated");
      onOpenChange(false);
    },
    onError: (err) => toast.error(err.message),
  });

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteGroup(group!.id),
    onSuccess: () => {
      invalidateGroupQueries(queryClient);
      toast.success("Group deleted");
      onOpenChange(false);
    },
    onError: (err) => toast.error(err.message),
  });

  const createInviteMutation = useMutation({
    mutationFn: (input: { username: string; role: GroupRole }) =>
      api.createGroupInvite(group!.id, {
        github_username: input.username,
        role: input.role,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.groupInvites(group!.id),
      });
      toast.success("Invitation created");
    },
    onError: (err) => toast.error(err.message),
  });

  const cancelInviteMutation = useMutation({
    mutationFn: (invite: GroupInvite) => api.cancelGroupInvite(group!.id, invite.id),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.groupInvites(group!.id),
      });
      toast.success("Invitation canceled");
    },
    onError: (err) => toast.error(err.message),
  });

  const updateMemberMutation = useMutation({
    mutationFn: (input: { member: GroupMember; role: GroupRole }) =>
      api.updateGroupMember(group!.id, input.member.user_id, { role: input.role }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.groupMembers(group!.id),
      });
      toast.success("Member updated");
    },
    onError: (err) => toast.error(err.message),
  });

  const removeMemberMutation = useMutation({
    mutationFn: (member: GroupMember) =>
      api.removeGroupMember(group!.id, member.user_id),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.groupMembers(group!.id),
      });
      toast.success("Member removed");
    },
    onError: (err) => toast.error(err.message),
  });

  const isSaving = createMutation.isPending || saveMutation.isPending;
  const isMemberMutating =
    createInviteMutation.isPending ||
    cancelInviteMutation.isPending ||
    updateMemberMutation.isPending ||
    removeMemberMutation.isPending;

  const submit = () => {
    if (!name.trim()) return;
    if (mode === "create") {
      createMutation.mutate();
    } else {
      saveMutation.mutate();
    }
  };

  const projectSummary =
    selectedProjects.length > 0
      ? `${selectedProjects.length} project${selectedProjects.length === 1 ? "" : "s"} will move into ${name.trim() || "this group"}.`
      : "No projects selected to move.";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{mode === "create" ? "New group" : "Manage group"}</DialogTitle>
          <DialogDescription>
            {mode === "create"
              ? "Create a group and move selected projects into it."
              : "Edit group details, project membership, members, and deletion."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <Label htmlFor="group-name">Group name</Label>
          <Input
            id="group-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Client Apps"
          />
        </div>

        {mode === "create" ? (
          <div className="space-y-3">
            <ProjectSelectionList
              projects={projects}
              selectedProjectIds={selectedProjectIds}
              onToggle={toggleProject}
              search={projectSearch}
              onSearchChange={setProjectSearch}
            />
            <p className="text-xs text-muted-foreground">{projectSummary}</p>
          </div>
        ) : (
          <Tabs defaultValue="projects" className="gap-4">
            <TabsList>
              <TabsTrigger value="projects">Projects</TabsTrigger>
              <TabsTrigger value="members">Members</TabsTrigger>
              <TabsTrigger value="danger">Danger</TabsTrigger>
            </TabsList>
            <TabsContent value="projects" className="space-y-3">
              <ProjectSelectionList
                projects={projects}
                selectedProjectIds={selectedProjectIds}
                alreadyInGroupIds={alreadyInGroupIds}
                onToggle={toggleProject}
                search={projectSearch}
                onSearchChange={setProjectSearch}
              />
              <p className="text-xs text-muted-foreground">{projectSummary}</p>
            </TabsContent>
            <TabsContent value="members">
              {group ? (
                <MembersTab
                  members={membersQuery.data ?? []}
                  invites={invitesQuery.data ?? []}
                  isLoading={membersQuery.isLoading || invitesQuery.isLoading}
                  onRoleChange={(member, role) =>
                    updateMemberMutation.mutate({ member, role })
                  }
                  onRemoveMember={(member) => removeMemberMutation.mutate(member)}
                  onCreateInvite={(username, role) =>
                    createInviteMutation.mutate({ username, role })
                  }
                  onCancelInvite={(invite) => cancelInviteMutation.mutate(invite)}
                  isMutating={isMemberMutating}
                />
              ) : null}
            </TabsContent>
            <TabsContent value="danger">
              <div className="rounded-md border border-destructive/30 p-4">
                <div className="font-medium text-destructive">Delete group</div>
                <p className="mt-1 text-sm text-muted-foreground">
                  Projects in this group will move to your default personal group.
                </p>
                <Button
                  type="button"
                  variant="destructive"
                  className="mt-4"
                  onClick={() => deleteMutation.mutate()}
                  disabled={!group || group.is_default || deleteMutation.isPending}
                >
                  <Trash2 className="h-4 w-4" />
                  Delete group
                </Button>
              </div>
            </TabsContent>
          </Tabs>
        )}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button type="button" onClick={submit} disabled={!name.trim() || isSaving}>
            {isSaving
              ? "Saving..."
              : mode === "create"
                ? "Create group"
                : "Save changes"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function Projects() {
  const {
    groups,
    selectedGroup,
    groupsView,
    setGroupsView,
    isLoading: groupsLoading,
  } = useGroups();
  const [createOpen, setCreateOpen] = useState(false);
  const [manageOpen, setManageOpen] = useState(false);

  const groupsViewIsValid = useMemo(() => {
    if (groupsView === "all") return true;
    return groups.some((group) => group.id === groupsView);
  }, [groups, groupsView]);
  const activeView = groupsViewIsValid ? groupsView : "all";
  const activeGroup =
    activeView === "all"
      ? null
      : groups.find((group) => group.id === activeView) ?? selectedGroup;

  const [search, setSearch] = useState("");
  const trimmedSearch = search.trim();

  const { data: projects, isLoading } = useQuery({
    queryKey: queryKeys.projects(activeView),
    queryFn: () => api.listProjects(activeView === "all" ? undefined : activeView),
    enabled: !groupsLoading,
  });

  const { data: allProjects } = useQuery({
    queryKey: queryKeys.projects("all"),
    queryFn: () => api.listProjects(),
    enabled: !groupsLoading && (createOpen || manageOpen),
  });

  const dialogProjects = allProjects ?? projects ?? [];

  const filteredProjects = useMemo(() => {
    if (!projects) return undefined;
    if (!trimmedSearch) return projects;
    const needle = trimmedSearch.toLowerCase();
    const tokens = needle.split(/\s+/).filter(Boolean);
    return projects.filter((project) => {
      const haystack = [
        project.name,
        project.organization_name ?? "",
        `${project.github_owner}/${project.github_repo}`,
        project.branch,
        project.latest_deployment_branch ?? "",
        project.latest_deployment_environment ?? "",
        project.latest_deployment_status ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return tokens.every((token) => haystack.includes(token));
    });
  }, [projects, trimmedSearch]);

  const subtitle =
    activeView === "all"
      ? "Across every group you can access"
      : `${activeGroup?.name ?? "Group"} group`;

  return (
    <div className="p-6">
      <div className="mb-6 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Projects</h1>
          <p className="text-sm text-muted-foreground">{subtitle}</p>
        </div>
        <Link to="/new">
          <Button>
            <Plus className="mr-2 h-4 w-4" />
            New Project
          </Button>
        </Link>
      </div>

      {groups.length > 0 && (
        <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-2">
            <div className="min-w-0 overflow-x-auto pb-2">
              <Tabs value={activeView} onValueChange={(value) => setGroupsView(value)}>
                <TabsList variant="line" className="min-w-max">
                  <TabsTrigger value="all">All</TabsTrigger>
                  {groups.map((group) => (
                    <TabsTrigger key={group.id} value={group.id}>
                      {group.name}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </Tabs>
            </div>
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              onClick={() => setCreateOpen(true)}
              aria-label="Create group"
            >
              <Plus className="h-4 w-4" />
            </Button>
            {activeGroup?.membership_role === "owner" ? (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                onClick={() => setManageOpen(true)}
                aria-label={`Manage ${activeGroup.name}`}
              >
                <Settings2 className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
          <div className="relative w-full sm:w-72">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search projects, repos, branches…"
              className="pl-9 pr-9"
              aria-label="Search projects"
            />
            {search && (
              <button
                type="button"
                onClick={() => setSearch("")}
                className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                aria-label="Clear search"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </div>
      )}

      {groupsLoading || isLoading ? (
        <LoadingState
          title="Loading projects…"
          description="Fetching your groups and deployed projects."
          className="min-h-[360px]"
        />
      ) : !groups.length ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-white/10 py-16">
          <p className="text-lg font-medium">No groups found</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Sign in again if your access was just changed.
          </p>
        </div>
      ) : !projects?.length ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-white/10 py-16">
          <p className="text-lg font-medium">
            {activeView === "all"
              ? "No projects yet across your groups"
              : "No projects in this group"}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            {activeView === "all"
              ? "Create your first project to get started."
              : `Create a project in ${activeGroup?.name ?? "this group"} to get started.`}
          </p>
          <Link to="/new" className="mt-4">
            <Button>
              <Plus className="mr-2 h-4 w-4" />
              New Project
            </Button>
          </Link>
        </div>
      ) : !filteredProjects?.length ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-white/10 py-16">
          <p className="text-lg font-medium">No projects match your search</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Nothing in {activeView === "all" ? "any group" : activeGroup?.name ?? "this group"} matched
            <span className="mx-1 font-mono">“{trimmedSearch}”</span>.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="mt-4"
            onClick={() => setSearch("")}
          >
            Clear search
          </Button>
        </div>
      ) : (
        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow className="border-white/8 hover:bg-transparent">
                  <TableHead className="pl-6">Project</TableHead>
                  <TableHead>Repository</TableHead>
                  <TableHead>Last Deploy</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead className="w-10 pr-6" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredProjects.map((project) => (
                  <ProjectTableRow key={project.id} project={project} />
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <GroupDialog
        mode="create"
        open={createOpen}
        onOpenChange={setCreateOpen}
        projects={dialogProjects}
      />
      <GroupDialog
        mode="edit"
        open={manageOpen}
        onOpenChange={setManageOpen}
        group={activeGroup}
        projects={dialogProjects}
      />
    </div>
  );
}
