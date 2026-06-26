import { expect, test as setup, type APIRequestContext } from "@playwright/test";

const authFile = "playwright/.auth/admin.json";

interface SeedProject {
  name: string;
  github_repo: string;
  github_owner: string;
  branch: string;
  framework: "nextjs" | "vite";
  package_manager: "bun" | "npm";
  build_command: string;
  install_command: string;
  node_version: string;
}

interface ProjectResponse {
  id: string;
  name: string;
  framework: string;
}

const projects: SeedProject[] = [
  {
    name: "my-nextjs-app",
    github_repo: "lovinka-deployik",
    github_owner: "lefteq",
    branch: "main",
    framework: "nextjs",
    package_manager: "bun",
    build_command: "bun run build",
    install_command: "bun install",
    node_version: "22",
  },
  {
    name: "static-site",
    github_repo: "demo-repo",
    github_owner: "lefteq",
    branch: "main",
    framework: "vite",
    package_manager: "npm",
    build_command: "npm run build",
    install_command: "npm ci",
    node_version: "22",
  },
];

setup("authenticate and seed deterministic projects", async ({ page }) => {
  await page.goto("/login");

  const login = await page.request.post("/api/auth/dev-login", {
    data: { username: "test-admin" },
  });
  expect(login.ok()).toBeTruthy();

  for (const project of projects) {
    await ensureProject(page.request, project);
  }

  await page.context().storageState({ path: authFile });
});

async function ensureProject(
  request: APIRequestContext,
  project: SeedProject,
): Promise<ProjectResponse> {
  const created = await request.post("/api/projects", { data: project });
  if (created.ok()) {
    return created.json();
  }

  if (created.status() !== 409) {
    throw new Error(
      `Failed to create ${project.name}: ${created.status()} ${await created.text()}`,
    );
  }

  const listed = await request.get("/api/projects");
  expect(listed.ok()).toBeTruthy();
  const existing = ((await listed.json()) as ProjectResponse[]).find(
    (candidate) => candidate.name === project.name,
  );
  if (!existing) {
    throw new Error(`Project ${project.name} already exists but is not visible`);
  }
  return existing;
}
