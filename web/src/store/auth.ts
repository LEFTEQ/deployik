import { create } from "zustand";
import type { User } from "@/types/api";

type AuthStatus = "unknown" | "authenticated" | "unauthenticated";

interface AuthState {
  user: User | null;
  status: AuthStatus;
  isAuthenticated: boolean;
  setAuthenticated: (user: User) => void;
  clearAuth: () => void;
  isAdmin: () => boolean;
}

export const useAuthStore = create<AuthState>()((set, get) => ({
  user: null,
  status: "unknown",
  isAuthenticated: false,

  setAuthenticated: (user) =>
    set({
      user,
      status: "authenticated",
      isAuthenticated: true,
    }),

  clearAuth: () =>
    set({
      user: null,
      status: "unauthenticated",
      isAuthenticated: false,
    }),

  isAdmin: () => get().user?.role === "admin",
}));
