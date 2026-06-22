import { create } from "zustand";

import type { LogTarget } from "@/components/apps/service-matrix";

export interface LogTab extends LogTarget {
  /** Stable id, independent of the (mutable) env/branch target. */
  id: string;
}

interface LogTabsState {
  tabs: LogTab[];
  activeTabId: string | null;
  /** Open (or focus) a tab for the given container target. */
  openLogs: (target: LogTarget) => void;
  closeTab: (id: string) => void;
  setActiveTab: (id: string) => void;
  /** Re-point an existing tab at a different environment/branch. */
  retarget: (
    id: string,
    patch: Partial<Pick<LogTarget, "environment" | "branch">>,
  ) => void;
  closeAll: () => void;
}

let seq = 0;

const sameTarget = (a: LogTarget, b: LogTarget) =>
  a.projectId === b.projectId &&
  a.environment === b.environment &&
  (a.branch ?? "") === (b.branch ?? "");

export const useLogTabsStore = create<LogTabsState>()((set) => ({
  tabs: [],
  activeTabId: null,

  openLogs: (target) =>
    set((state) => {
      const existing = state.tabs.find((tab) => sameTarget(tab, target));
      if (existing) return { activeTabId: existing.id };
      const id = `logtab-${++seq}`;
      return { tabs: [...state.tabs, { id, ...target }], activeTabId: id };
    }),

  closeTab: (id) =>
    set((state) => {
      const tabs = state.tabs.filter((tab) => tab.id !== id);
      const activeTabId =
        state.activeTabId === id
          ? (tabs[tabs.length - 1]?.id ?? null)
          : state.activeTabId;
      return { tabs, activeTabId };
    }),

  setActiveTab: (id) => set({ activeTabId: id }),

  retarget: (id, patch) =>
    set((state) => ({
      tabs: state.tabs.map((tab) => {
        if (tab.id !== id) return tab;
        const environment = patch.environment ?? tab.environment;
        const branch =
          environment === "production"
            ? undefined
            : patch.branch !== undefined
              ? patch.branch
              : tab.branch;
        return { ...tab, environment, branch };
      }),
    })),

  closeAll: () => set({ tabs: [], activeTabId: null }),
}));
