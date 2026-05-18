import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useGroupStore } from "@/store/group";

export function useOrganizations() {
  const {
    selectedGroupId,
    setSelectedGroupId,
    groupsView,
    setGroupsView,
    hydrateGroups,
  } = useGroupStore();

  const query = useQuery({
    queryKey: queryKeys.groups(),
    queryFn: async () => {
      const groups = await api.listGroups();
      return groups.map((group) => ({
        id: group.id,
        name: group.name,
        slug: group.slug,
        is_personal: group.is_default,
        personal_owner_user_id: group.personal_owner_user_id,
        membership_role: group.membership_role,
        project_count: group.project_count,
        created_at: group.created_at,
        updated_at: group.updated_at,
      }));
    },
  });

  useEffect(() => {
    if (query.data) {
      const groups = query.data.map((organization) => ({
        id: organization.id,
        name: organization.name,
        slug: organization.slug,
        is_default: organization.is_personal,
        personal_owner_user_id: organization.personal_owner_user_id,
        membership_role: organization.membership_role,
        project_count: organization.project_count,
        display_order: 0,
        created_at: organization.created_at,
        updated_at: organization.updated_at,
      }));
      hydrateGroups(groups);
    }
  }, [hydrateGroups, query.data]);

  const organizations = query.data ?? [];
  const selectedOrganization =
    organizations.find((organization) => organization.id === selectedGroupId) ??
    organizations.find((organization) => organization.is_personal) ??
    organizations[0] ??
    null;

  return {
    ...query,
    organizations,
    selectedOrganizationId: selectedOrganization?.id ?? null,
    selectedOrganization,
    setSelectedOrganizationId: setSelectedGroupId,
    projectsView: groupsView,
    setProjectsView: setGroupsView,
  };
}
