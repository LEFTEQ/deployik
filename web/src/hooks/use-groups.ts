import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useGroupStore } from "@/store/group";

export function useGroups() {
  const {
    selectedGroupId,
    setSelectedGroupId,
    groupsView,
    setGroupsView,
    hydrateGroups,
  } = useGroupStore();

  const query = useQuery({
    queryKey: queryKeys.groups(),
    queryFn: () => api.listGroups(),
  });

  useEffect(() => {
    if (query.data) {
      hydrateGroups(query.data);
    }
  }, [hydrateGroups, query.data]);

  const groups = query.data ?? [];
  const selectedGroup =
    groups.find((group) => group.id === selectedGroupId) ??
    groups.find((group) => group.is_default) ??
    groups[0] ??
    null;

  return {
    ...query,
    groups,
    selectedGroupId: selectedGroup?.id ?? null,
    selectedGroup,
    setSelectedGroupId,
    groupsView,
    setGroupsView,
  };
}
