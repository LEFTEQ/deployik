import { expect, test, type APIRequestContext } from "@playwright/test";

interface ProjectResponse {
  id: string;
  name: string;
  framework: string;
  resource_tier: "nano" | "small" | "medium" | "large";
}

// resetTier puts the project back to the default Small tier between tests so
// the test order is irrelevant and other specs that read the project aren't
// surprised by leftover state from a prior run.
async function resetTier(
  request: APIRequestContext,
  projectId: string,
): Promise<void> {
  const res = await request.patch(`/api/projects/${projectId}`, {
    data: { resource_tier: "small" },
  });
  expect(res.ok()).toBeTruthy();
}

async function getProjectByName(
  request: APIRequestContext,
  name: string,
): Promise<ProjectResponse> {
  const res = await request.get("/api/projects");
  expect(res.ok()).toBeTruthy();
  const projects = (await res.json()) as ProjectResponse[];
  const project = projects.find((p) => p.name === name);
  if (!project) {
    throw new Error(`seed project ${name} not found`);
  }
  return project;
}

test.describe("project resources (tier picker)", () => {
  test.beforeEach(async ({ request }) => {
    const project = await getProjectByName(request, "my-nextjs-app");
    await resetTier(request, project.id);
  });

  test("renders all four tiers with Small marked as Current", async ({
    page,
    request,
  }) => {
    const project = await getProjectByName(request, "my-nextjs-app");

    await page.goto(`/projects/${project.id}/settings/resources`);
    await expect(page.getByTestId("project-resources-page")).toBeVisible();

    // All four options must be present.
    for (const tier of ["nano", "small", "medium", "large"] as const) {
      await expect(
        page.getByTestId(`resource-tier-option-${tier}`),
      ).toBeVisible();
    }

    // Small is the current selection (also doubles as default-on-load).
    await expect(
      page.getByTestId("resource-tier-option-small"),
    ).toHaveAttribute("aria-pressed", "true");

    // Save + Reset are disabled when no change is pending.
    await expect(page.getByTestId("resource-tier-save")).toBeDisabled();
    await expect(page.getByTestId("resource-tier-reset")).toBeDisabled();
  });

  test("is reachable from the sidebar Settings → Resources sub-item", async ({
    page,
    request,
  }) => {
    const project = await getProjectByName(request, "my-nextjs-app");
    await page.goto(`/projects/${project.id}`);

    await page.getByTestId("sidebar-settings-resources").click();
    await expect(page).toHaveURL(
      new RegExp(`/projects/${project.id}/settings/resources$`),
    );
    await expect(page.getByTestId("project-resources-page")).toBeVisible();
  });

  test("selecting a new tier enables Save and persists on click", async ({
    page,
    request,
  }) => {
    const project = await getProjectByName(request, "my-nextjs-app");

    await page.goto(`/projects/${project.id}/settings/resources`);

    await page.getByTestId("resource-tier-option-medium").click();
    await expect(
      page.getByTestId("resource-tier-option-medium"),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(
      page.getByTestId("resource-tier-option-small"),
    ).toHaveAttribute("aria-pressed", "false");

    const save = page.getByTestId("resource-tier-save");
    await expect(save).toBeEnabled();
    await save.click();

    // Server-side state should now report Medium and the "Current" badge moves.
    await expect
      .poll(async () => {
        const r = await request.get(`/api/projects/${project.id}`);
        return ((await r.json()) as ProjectResponse).resource_tier;
      })
      .toBe("medium");

    // After save, the page should treat Medium as the new baseline (no
    // pending changes) and Small should lose its "Current" badge.
    await expect(page.getByTestId("resource-tier-save")).toBeDisabled();
    await expect(
      page.getByTestId("resource-tier-option-medium"),
    ).toContainText("Current");
  });

  test("Reset button reverts an unsaved selection back to the current tier", async ({
    page,
    request,
  }) => {
    const project = await getProjectByName(request, "my-nextjs-app");

    await page.goto(`/projects/${project.id}/settings/resources`);

    await page.getByTestId("resource-tier-option-large").click();
    await expect(page.getByTestId("resource-tier-save")).toBeEnabled();
    await expect(page.getByTestId("resource-tier-reset")).toBeEnabled();

    await page.getByTestId("resource-tier-reset").click();

    await expect(
      page.getByTestId("resource-tier-option-small"),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByTestId("resource-tier-save")).toBeDisabled();
    await expect(page.getByTestId("resource-tier-reset")).toBeDisabled();

    // No server mutation should have occurred.
    const r = await request.get(`/api/projects/${project.id}`);
    expect(((await r.json()) as ProjectResponse).resource_tier).toBe("small");
  });

  test("API rejects unknown tier values with HTTP 400", async ({ request }) => {
    const project = await getProjectByName(request, "my-nextjs-app");

    const bad = await request.patch(`/api/projects/${project.id}`, {
      data: { resource_tier: "enormous" },
    });
    expect(bad.status()).toBe(400);
    const body = (await bad.json()) as { error: string };
    expect(body.error).toMatch(/resource_tier/);

    // The project tier must remain unchanged.
    const r = await request.get(`/api/projects/${project.id}`);
    expect(((await r.json()) as ProjectResponse).resource_tier).toBe("small");
  });

  test("each tier displays runtime and build memory/CPU", async ({
    page,
    request,
  }) => {
    const project = await getProjectByName(request, "my-nextjs-app");
    await page.goto(`/projects/${project.id}/settings/resources`);

    // Spot-check that build memory > runtime memory is rendered visibly for
    // each tier — the row text encodes "<runtime> · <cpu> CPU build <build> / <cpu> CPU".
    const cases: Array<{
      tier: "nano" | "small" | "medium" | "large";
      runtime: string;
      build: string;
    }> = [
      { tier: "nano", runtime: "256 MB", build: "1536 MB" },
      { tier: "small", runtime: "512 MB", build: "2 GB" },
      { tier: "medium", runtime: "1 GB", build: "3 GB" },
      { tier: "large", runtime: "2 GB", build: "4 GB" },
    ];
    for (const c of cases) {
      const row = page.getByTestId(`resource-tier-option-${c.tier}`);
      await expect(row).toContainText(c.runtime);
      await expect(row).toContainText(c.build);
    }
  });
});
