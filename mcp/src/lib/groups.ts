import { DeployikClient } from "../client/http.js";
import type { Group } from "../client/types.js";

export interface GroupSelector {
  group_id?: string;
  group?: string;
  organization_id?: string;
  workspace?: string;
}

export function findGroupBySelector(
  groups: Group[],
  selector: string,
): Group | undefined {
  const normalized = selector.trim().toLowerCase();
  if (!normalized) return undefined;

  return groups.find(
    (group) =>
      group.id === selector ||
      group.slug.toLowerCase() === normalized ||
      group.name.toLowerCase() === normalized,
  );
}

export async function fetchGroups(client: DeployikClient): Promise<Group[]> {
  return client.request<Group[]>("/groups");
}

export async function resolveGroup(
  client: DeployikClient,
  selector: GroupSelector,
): Promise<Group | undefined> {
  const value =
    selector.group_id ??
    selector.group ??
    selector.organization_id ??
    selector.workspace;
  if (!value) return undefined;

  const groups = await fetchGroups(client);
  const group = findGroupBySelector(groups, value);
  if (!group) {
    throw new Error(
      `No dashboard group matching '${value}' is visible to this token. Call list_groups to see available groups.`,
    );
  }
  return group;
}

export async function resolveGroupId(
  client: DeployikClient,
  selector: GroupSelector,
): Promise<string | undefined> {
  const direct = selector.group_id ?? selector.organization_id;
  if (direct) return direct;
  const group = await resolveGroup(client, selector);
  return group?.id;
}

export function renderGroupsList(groups: Group[]): string {
  if (groups.length === 0) return "(no dashboard groups visible to this token)";
  return groups
    .map((group) => {
      const kind = group.is_default ? "default" : "custom";
      return `  • ${group.slug.padEnd(24)}  ${kind}  role=${group.membership_role}  projects=${group.project_count}  id=${group.id}`;
    })
    .join("\n");
}
