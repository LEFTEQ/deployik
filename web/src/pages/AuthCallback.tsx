import { useEffect, useRef } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { api } from "@/lib/api";
import { useAuthStore } from "@/store/auth";

export function AuthCallback() {
  const navigate = useNavigate();
  const search = useSearch({ from: "/auth/callback" });
  const processedRef = useRef(false);

  useEffect(() => {
    if (processedRef.current) return;
    processedRef.current = true;

    const code = (search as Record<string, string>).code;
    const state = (search as Record<string, string>).state;
    if (!code || !state) {
      navigate({ to: "/login" });
      return;
    }

    api
      .completeGithubAuth(code, state)
      .then((data) => {
        useAuthStore.getState().setAuthenticated(data.user);
        navigate({ to: "/" });
      })
      .catch((err) => {
        console.error("Auth callback failed:", err);
        useAuthStore.getState().clearAuth();
        navigate({ to: "/login" });
      });
  }, [navigate, search]);

  return (
    <div className="flex min-h-screen items-center justify-center">
      <p className="text-muted-foreground">Signing in...</p>
    </div>
  );
}
