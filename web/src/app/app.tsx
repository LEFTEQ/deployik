import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  RouterProvider,
} from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useAuthStore } from "@/store/auth";
import { Login } from "@/pages/Login";
import { AuthCallback } from "@/pages/AuthCallback";
import { Projects } from "@/pages/Projects";
import { NewProject } from "@/pages/NewProject";
import { ProjectDetail } from "@/pages/ProjectDetail";
import { DeploymentDetail } from "@/pages/DeploymentDetail";
import { AppLayout } from "@/components/layout/AppLayout";
import { api } from "@/lib/api";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

// Root route
const rootRoute = createRootRoute({
  component: () => (
    <TooltipProvider>
      <Outlet />
      <Toaster position="bottom-right" richColors theme="dark" />
    </TooltipProvider>
  ),
});

let authBootstrapPromise: Promise<void> | null = null;

async function hydrateAuthState() {
  const state = useAuthStore.getState();
  if (state.status !== "unknown") {
    return;
  }

  if (!authBootstrapPromise) {
    authBootstrapPromise = api
      .getMe()
      .then((user) => {
        useAuthStore.getState().setAuthenticated(user);
      })
      .catch(() => {
        useAuthStore.getState().clearAuth();
      })
      .finally(() => {
        authBootstrapPromise = null;
      });
  }

  await authBootstrapPromise;
}

// Public routes
const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  beforeLoad: async () => {
    await hydrateAuthState();
    if (useAuthStore.getState().isAuthenticated) {
      throw redirect({ to: "/" });
    }
  },
  component: Login,
});

const authCallbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auth/callback",
  validateSearch: (search: Record<string, unknown>) => ({
    code: (search.code as string) || "",
    state: (search.state as string) || "",
  }),
  component: AuthCallback,
});

// Protected layout route (with sidebar)
const protectedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "protected",
  beforeLoad: async () => {
    await hydrateAuthState();
    const { isAuthenticated } = useAuthStore.getState();
    if (!isAuthenticated) {
      throw redirect({ to: "/login" });
    }
  },
  component: AppLayout,
});

// Dashboard (projects list)
const indexRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/",
  component: Projects,
});

// New project
const newProjectRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/new",
  component: NewProject,
});

// Project detail
const projectDetailRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/projects/$id",
  component: ProjectDetail,
});

// Deployment detail
const deploymentDetailRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/projects/$id/deployments/$did",
  component: DeploymentDetail,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  authCallbackRoute,
  protectedRoute.addChildren([
    indexRoute,
    newProjectRoute,
    projectDetailRoute,
    deploymentDetailRoute,
  ]),
]);

const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
