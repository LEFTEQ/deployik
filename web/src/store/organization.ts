import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { Organization } from "@/types/api";

export type ProjectsView = "all" | string;

interface OrganizationState {
  selectedOrganizationId: string | null;
  projectsView: ProjectsView;
  setSelectedOrganizationId: (organizationId: string | null) => void;
  setProjectsView: (view: ProjectsView) => void;
  hydrateOrganizations: (organizations: Organization[]) => void;
  clearSelection: () => void;
}

export const useOrganizationStore = create<OrganizationState>()(
  persist(
    (set, get) => ({
      selectedOrganizationId: null,
      projectsView: "all",

      setSelectedOrganizationId: (organizationId) =>
        set({
          selectedOrganizationId: organizationId,
          projectsView: organizationId ?? get().projectsView,
        }),

      setProjectsView: (view) =>
        set(
          view === "all"
            ? { projectsView: "all" }
            : { projectsView: view, selectedOrganizationId: view },
        ),

      hydrateOrganizations: (organizations) => {
        const { selectedOrganizationId, projectsView } = get();
        const hasSelectedOrganization = organizations.some(
          (organization) => organization.id === selectedOrganizationId,
        );
        const nextSelectedId = hasSelectedOrganization
          ? selectedOrganizationId
          : organizations[0]?.id ?? null;

        const projectsViewIsValid =
          projectsView === "all" ||
          organizations.some((organization) => organization.id === projectsView);

        set({
          selectedOrganizationId: nextSelectedId,
          projectsView: projectsViewIsValid ? projectsView : "all",
        });
      },

      clearSelection: () =>
        set({ selectedOrganizationId: null, projectsView: "all" }),
    }),
    {
      name: "deployik-selected-organization",
      partialize: (state) => ({
        selectedOrganizationId: state.selectedOrganizationId,
        projectsView: state.projectsView,
      }),
    },
  ),
);
