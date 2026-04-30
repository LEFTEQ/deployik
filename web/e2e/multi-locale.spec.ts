import { expect, test, type APIRequestContext } from "@playwright/test";

interface ProjectResponse {
  id: string;
  name: string;
  framework: string;
}

test("opens Multi Locale from the sidebar for a Next.js project", async ({
  page,
  request,
}) => {
  const project = await getProjectByName(request, "my-nextjs-app");

  await page.goto(`/projects/${project.id}`);
  await page.getByTestId("sidebar-integrations-multi-locale").click();

  await expect(page).toHaveURL(
    new RegExp(`/projects/${project.id}/integrations/multi-locale$`),
  );
  await expect(page.getByTestId("multi-locale-page")).toBeVisible();
  await expect(page.getByTestId("multi-locale-nextjs-workflow")).toBeVisible();
});

test("generates a next-intl prompt from selected locales", async ({
  page,
  request,
}, testInfo) => {
  const project = await getProjectByName(request, "my-nextjs-app");

  await page.goto(`/projects/${project.id}/integrations/multi-locale`);

  await expect(page.getByTestId("multi-locale-selected-cs")).toBeVisible();
  await expect(page.getByTestId("multi-locale-selected-en")).toBeVisible();
  await expect(page.getByTestId("multi-locale-selected-sk")).toBeVisible();
  await expect(page.getByText("Supported locales: cs, en, sk")).toBeVisible();

  await page.getByTestId("multi-locale-language-search").fill("German");
  await expect(page.getByTestId("multi-locale-option-de")).toBeVisible();
  await page.getByTestId("multi-locale-checkbox-de").click();
  await expect(page.getByTestId("multi-locale-selected-de")).toBeVisible();

  await page.getByTestId("multi-locale-default-select").click();
  await page.getByRole("option", { name: /German/ }).click();
  await expect(page.getByText("Default locale: de")).toBeVisible();

  await page.getByTestId("multi-locale-custom-input").fill("da");
  await page.getByTestId("multi-locale-add-custom").click();
  await expect(page.getByTestId("multi-locale-selected-da")).toBeVisible();
  await expect(page.getByText("Supported locales: cs, en, sk, de, da")).toBeVisible();

  await page.screenshot({
    path: testInfo.outputPath("multi-locale-nextjs-desktop.png"),
    fullPage: true,
  });

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByTestId("multi-locale-page")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("multi-locale-nextjs-mobile.png"),
    fullPage: true,
  });
});

test("shows the generic guide for non-Next.js projects", async ({
  page,
  request,
}, testInfo) => {
  const project = await getProjectByName(request, "static-site");

  await page.goto(`/projects/${project.id}/integrations/multi-locale`);

  await expect(page.getByTestId("multi-locale-page")).toBeVisible();
  await expect(page.getByTestId("multi-locale-framework-notice")).toContainText(
    "vite",
  );
  await expect(page.getByTestId("multi-locale-generic-guide")).toBeVisible();
  await expect(page.getByTestId("multi-locale-nextjs-workflow")).toHaveCount(0);

  await page.screenshot({
    path: testInfo.outputPath("multi-locale-generic-guide.png"),
    fullPage: true,
  });
});

async function getProjectByName(
  request: APIRequestContext,
  name: string,
): Promise<ProjectResponse> {
  const response = await request.get("/api/projects");
  expect(response.ok()).toBeTruthy();

  const projects = (await response.json()) as ProjectResponse[];
  const project = projects.find((candidate) => candidate.name === name);
  if (!project) {
    throw new Error(`Expected seeded project ${name} to exist`);
  }
  return project;
}
