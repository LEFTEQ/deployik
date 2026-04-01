import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { Organization } from "@/types/api";

interface OrganizationState {
  selectedOrganizationId: string | null;
  setSelectedOrganizationId: (organizationId: string | null) => void;
  hydrateOrganizations: (organizations: Organization[]) => void;
  clearSelection: () => void;
}

export const useOrganizationStore = create<OrganizationState>()(
  persist(
    (set, get) => ({
      selectedOrganizationId: null,

      setSelectedOrganizationId: (organizationId) =>
        set({ selectedOrganizationId: organizationId }),

      hydrateOrganizations: (organizations) => {
        const selectedOrganizationId = get().selectedOrganizationId;
        const hasSelectedOrganization = organizations.some(
          (organization) => organization.id === selectedOrganizationId,
        );
        set({
          selectedOrganizationId:
            hasSelectedOrganization
              ? selectedOrganizationId
              : organizations[0]?.id ?? null,
        });
      },

      clearSelection: () => set({ selectedOrganizationId: null }),
    }),
    {
      name: "deployik-selected-organization",
      partialize: (state) => ({
        selectedOrganizationId: state.selectedOrganizationId,
      }),
    },
  ),
);
