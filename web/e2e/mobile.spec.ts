import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";

interface ProjectResponse {
  id: string;
  name: string;
}

const IPHONE_VIEWPORT = { width: 390, height: 844 };

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

// The page must never scroll sideways on a phone — horizontal overflow is the
// canonical mobile regression.
async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  const overflow = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  expect(
    overflow.scrollWidth,
    `page is ${overflow.scrollWidth}px wide but the viewport is ${overflow.innerWidth}px`,
  ).toBeLessThanOrEqual(overflow.innerWidth + 1);
}

test.describe("mobile layout (iPhone viewport)", () => {
  test.use({ viewport: IPHONE_VIEWPORT, hasTouch: true });

  test("workspace tab bar navigates and opens the drawer", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("mobile-tab-bar")).toBeVisible();

    // Tab navigation: Projects -> New.
    await page.getByTestId("mobile-tab-new").click();
    await expect(page).toHaveURL(/\/new$/);

    await page.goto("/");
    // "More" opens the sidebar drawer (single source of full navigation).
    await page.getByTestId("mobile-tab-more").click();
    await expect(
      page.locator('[data-sidebar="sidebar"][data-mobile="true"]'),
    ).toBeVisible();
  });

  test("project tab bar shows project tabs and navigates", async ({
    page,
    request,
  }) => {
    const project = await getProjectByName(request, "my-nextjs-app");
    await page.goto(`/projects/${project.id}`);

    await expect(page.getByTestId("mobile-tab-bar")).toBeVisible();
    await page.getByTestId("mobile-tab-deploys").click();
    await expect(page).toHaveURL(
      new RegExp(`/projects/${project.id}/deployments$`),
    );

    await page.getByTestId("mobile-tab-overview").click();
    await expect(page).toHaveURL(new RegExp(`/projects/${project.id}$`));
  });

  test("key pages have no horizontal overflow", async ({ page, request }) => {
    const project = await getProjectByName(request, "my-nextjs-app");
    const paths = [
      "/",
      `/projects/${project.id}`,
      `/projects/${project.id}/deployments`,
      `/projects/${project.id}/settings`,
      `/projects/${project.id}/settings/domains`,
      `/projects/${project.id}/settings/env`,
      `/projects/${project.id}/settings/protection`,
      `/projects/${project.id}/integrations/analytics`,
      "/new",
    ];

    for (const path of paths) {
      await page.goto(path);
      await page.waitForLoadState("networkidle");
      await expectNoHorizontalOverflow(page);
    }
  });

  test("release dialog fits the phone viewport", async ({ page, request }) => {
    const project = await getProjectByName(request, "my-nextjs-app");
    await page.goto(`/projects/${project.id}/deployments`);

    await page.getByRole("button", { name: "Release" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();

    const box = await dialog.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeLessThanOrEqual(IPHONE_VIEWPORT.height * 0.85 + 1);
    expect(box!.width).toBeLessThanOrEqual(IPHONE_VIEWPORT.width);
  });
});

test.describe("desktop keeps its layout", () => {
  test("tab bar is hidden on desktop viewports", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("mobile-tab-bar")).toBeHidden();
  });
});
