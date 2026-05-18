import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { Group } from "@/types/api";

export type GroupsView = "all" | string;

interface GroupState {
  selectedGroupId: string | null;
  groupsView: GroupsView;
  setSelectedGroupId: (groupId: string | null) => void;
  setGroupsView: (view: GroupsView) => void;
  hydrateGroups: (groups: Group[]) => void;
  clearSelection: () => void;
}

export const useGroupStore = create<GroupState>()(
  persist(
    (set, get) => ({
      selectedGroupId: null,
      groupsView: "all",

      setSelectedGroupId: (groupId) =>
        set({
          selectedGroupId: groupId,
          groupsView: groupId ?? get().groupsView,
        }),

      setGroupsView: (view) =>
        set(
          view === "all"
            ? { groupsView: "all" }
            : { groupsView: view, selectedGroupId: view },
        ),

      hydrateGroups: (groups) => {
        const { selectedGroupId, groupsView } = get();
        const hasSelectedGroup = groups.some((group) => group.id === selectedGroupId);
        const defaultGroup =
          groups.find((group) => group.is_default) ?? groups[0] ?? null;
        const nextSelectedId = hasSelectedGroup
          ? selectedGroupId
          : defaultGroup?.id ?? null;

        const groupsViewIsValid =
          groupsView === "all" || groups.some((group) => group.id === groupsView);

        set({
          selectedGroupId: nextSelectedId,
          groupsView: groupsViewIsValid ? groupsView : "all",
        });
      },

      clearSelection: () => set({ selectedGroupId: null, groupsView: "all" }),
    }),
    {
      name: "deployik-selected-group",
      partialize: (state) => ({
        selectedGroupId: state.selectedGroupId,
        groupsView: state.groupsView,
      }),
    },
  ),
);
