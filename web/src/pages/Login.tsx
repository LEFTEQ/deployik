import { GitBranch } from "lucide-react";

const API_URL = import.meta.env.VITE_API_URL || "/api";

export function Login() {
  const handleGithubLogin = () => {
    window.location.href = `${API_URL}/auth/github`;
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="mx-auto flex w-full max-w-sm flex-col items-center gap-6 px-4">
        <div className="text-center">
          <h1 className="text-3xl font-bold tracking-tight">Deployik</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Self-hosted deployment platform
          </p>
        </div>

        <button
          onClick={handleGithubLogin}
          className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <GitBranch className="h-5 w-5" />
          Sign in with GitHub
        </button>

        <p className="text-xs text-muted-foreground">
          Only authorized GitHub users can sign in.
        </p>
      </div>
    </div>
  );
}
