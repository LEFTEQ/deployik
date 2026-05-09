import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { useOrganizationStore } from "@/store/organization";

export function useOrganizations() {
  const {
    selectedOrganizationId,
    setSelectedOrganizationId,
    projectsView,
    setProjectsView,
    hydrateOrganizations,
  } = useOrganizationStore();

  const query = useQuery({
    queryKey: queryKeys.organizations(),
    queryFn: () => api.listOrganizations(),
  });

  useEffect(() => {
    if (query.data) {
      hydrateOrganizations(query.data);
    }
  }, [hydrateOrganizations, query.data]);

  const organizations = query.data ?? [];
  const selectedOrganization =
    organizations.find(
      (organization) => organization.id === selectedOrganizationId,
    ) ??
    organizations[0] ??
    null;

  return {
    ...query,
    organizations,
    selectedOrganizationId: selectedOrganization?.id ?? null,
    selectedOrganization,
    setSelectedOrganizationId,
    projectsView,
    setProjectsView,
  };
}
