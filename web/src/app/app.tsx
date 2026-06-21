import { lazy } from "react";
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
import { OfflineBanner } from "@/components/layout/OfflineBanner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useAuthStore } from "@/store/auth";
// Hot paths stay eager so the initial render is a single network round-trip.
import { Login } from "@/pages/Login";
import { AuthCallback } from "@/pages/AuthCallback";
import { Projects } from "@/pages/Projects";
import { Apps } from "@/pages/Apps";
import { AppBundleLayout } from "@/components/layout/AppBundleLayout";
import { AppOverview } from "@/pages/AppOverview";
import { AppDeployments } from "@/pages/AppDeployments";
import { AppTopology } from "@/pages/AppTopology";
import { AppVariables } from "@/pages/AppVariables";
import { AppReleases } from "@/pages/AppReleases";
import { AppSettings } from "@/pages/AppSettings";
import { ProjectOverview } from "@/pages/ProjectOverview";
import { ProjectDeployments } from "@/pages/ProjectDeployments";
// Heavy or less-visited routes are code-split so charts, form primitives,
// and log-streaming hooks don't block the main bundle on first paint.
const NewProject = lazy(() =>
  import("@/pages/NewProject").then((m) => ({ default: m.NewProject })),
);
const ProjectAnalytics = lazy(() =>
  import("@/pages/ProjectAnalytics").then((m) => ({
    default: m.ProjectAnalytics,
  })),
);
const ProjectEmail = lazy(() =>
  import("@/pages/ProjectEmail").then((m) => ({
    default: m.ProjectEmail,
  })),
);
const ProjectMultiLocale = lazy(() =>
  import("@/pages/ProjectMultiLocale").then((m) => ({
    default: m.ProjectMultiLocale,
  })),
);
const ProjectSettings = lazy(() =>
  import("@/pages/ProjectSettings").then((m) => ({
    default: m.ProjectSettings,
  })),
);
const ProjectSettingsDomains = lazy(() =>
  import("@/pages/ProjectSettingsDomains").then((m) => ({
    default: m.ProjectSettingsDomains,
  })),
);
const ProjectSettingsEnv = lazy(() =>
  import("@/pages/ProjectSettingsEnv").then((m) => ({
    default: m.ProjectSettingsEnv,
  })),
);
const ProjectSettingsProtection = lazy(() =>
  import("@/pages/ProjectSettingsProtection").then((m) => ({
    default: m.ProjectSettingsProtection,
  })),
);
const ProjectSettingsResources = lazy(() =>
  import("@/pages/ProjectSettingsResources").then((m) => ({
    default: m.ProjectSettingsResources,
  })),
);
const ProjectSettingsServices = lazy(() =>
  import("@/pages/ProjectSettingsServices").then((m) => ({
    default: m.ProjectSettingsServices,
  })),
);
const DeploymentDetail = lazy(() =>
  import("@/pages/DeploymentDetail").then((m) => ({
    default: m.DeploymentDetail,
  })),
);
const UserTokens = lazy(() =>
  import("@/pages/UserTokens").then((m) => ({ default: m.UserTokens })),
);
const NotificationSettings = lazy(() =>
  import("@/pages/NotificationSettings").then((m) => ({
    default: m.NotificationSettings,
  })),
);
import { AppLayout } from "@/components/layout/AppLayout";
import { WorkspaceLayout } from "@/components/layout/WorkspaceLayout";
import { ProjectLayout } from "@/components/layout/ProjectLayout";
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
      <OfflineBanner />
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

// Protected layout route (wraps everything with sidebar provider + top bar)
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

// Workspace layout (sidebar shows workspace nav items)
const workspaceLayoutRoute = createRoute({
  getParentRoute: () => protectedRoute,
  id: "workspace",
  component: WorkspaceLayout,
});

// Dashboard (projects list)
const indexRoute = createRoute({
  getParentRoute: () => workspaceLayoutRoute,
  path: "/",
  component: Projects,
});

const userTokensRoute = createRoute({
  getParentRoute: () => workspaceLayoutRoute,
  path: "/account/tokens",
  component: UserTokens,
});

const notificationSettingsRoute = createRoute({
  getParentRoute: () => workspaceLayoutRoute,
  path: "/account/notifications",
  component: NotificationSettings,
});

const appsRoute = createRoute({
  getParentRoute: () => workspaceLayoutRoute,
  path: "/apps",
  component: Apps,
});

// App bundle layout (first-class app shell with its own sub-routes)
const appLayoutRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/apps/$appId",
  component: AppBundleLayout,
});

const appOverviewRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/",
  component: AppOverview,
});

const appDeploymentsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/deployments",
  component: AppDeployments,
});

const appTopologyRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/topology",
  component: AppTopology,
});

const appVariablesRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/variables",
  component: AppVariables,
});

const appReleasesRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/releases",
  component: AppReleases,
});

const appSettingsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: "/settings",
  component: AppSettings,
});

// New project (no sidebar context needed, uses workspace layout)
const newProjectRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/new",
  component: NewProject,
});

// Project layout (sidebar shows project nav items)
const projectLayoutRoute = createRoute({
  getParentRoute: () => protectedRoute,
  path: "/projects/$id",
  component: ProjectLayout,
});

// Project sub-pages
const projectOverviewRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/",
  component: ProjectOverview,
});

const projectDeploymentsRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/deployments",
  component: ProjectDeployments,
});

const deploymentDetailRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/deployments/$did",
  component: DeploymentDetail,
});

const projectAnalyticsRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/integrations/analytics",
  component: ProjectAnalytics,
});

const projectEmailRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/integrations/email",
  component: ProjectEmail,
});

const projectMultiLocaleRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/integrations/multi-locale",
  component: ProjectMultiLocale,
});

const projectSettingsRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings",
  component: ProjectSettings,
});

const projectSettingsDomainsRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings/domains",
  component: ProjectSettingsDomains,
});

const projectSettingsEnvRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings/env",
  component: ProjectSettingsEnv,
});

const projectSettingsProtectionRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings/protection",
  component: ProjectSettingsProtection,
});

const projectSettingsResourcesRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings/resources",
  component: ProjectSettingsResources,
});

const projectSettingsServicesRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: "/settings/services",
  component: ProjectSettingsServices,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  authCallbackRoute,
  protectedRoute.addChildren([
    workspaceLayoutRoute.addChildren([
      indexRoute,
      userTokensRoute,
      notificationSettingsRoute,
      appsRoute,
    ]),
    newProjectRoute,
    appLayoutRoute.addChildren([
      appOverviewRoute,
      appDeploymentsRoute,
      appTopologyRoute,
      appVariablesRoute,
      appReleasesRoute,
      appSettingsRoute,
    ]),
    projectLayoutRoute.addChildren([
      projectOverviewRoute,
      projectDeploymentsRoute,
      deploymentDetailRoute,
      projectAnalyticsRoute,
      projectEmailRoute,
      projectMultiLocaleRoute,
      projectSettingsRoute,
      projectSettingsDomainsRoute,
      projectSettingsEnvRoute,
      projectSettingsProtectionRoute,
      projectSettingsResourcesRoute,
      projectSettingsServicesRoute,
    ]),
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
