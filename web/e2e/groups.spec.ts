import { expect, test, type APIRequestContext } from "@playwright/test";

interface ProjectResponse {
  id: string;
  name: string;
  organization_id: string;
  organization_name?: string;
}

interface GroupResponse {
  id: string;
  name: string;
  is_default: boolean;
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

async function getDefaultGroupId(request: APIRequestContext): Promise<string> {
  const res = await request.get("/api/groups");
  expect(res.ok()).toBeTruthy();
  const groups = (await res.json()) as GroupResponse[];
  const group = groups.find((candidate) => candidate.is_default);
  if (!group) {
    throw new Error("seed default group not found");
  }
  return group.id;
}

test.describe("project groups", () => {
  test("creates, renames, invites into, and deletes a group", async ({
    browser,
    page,
    request,
  }, testInfo) => {
    const suffix = Date.now().toString(36);
    const groupName = `QA Group ${suffix}`;
    const renamedGroupName = `QA Renamed ${suffix}`;
    const inviteUsername = `teammate-${suffix}`;
    const defaultGroupId = await getDefaultGroupId(request);

    await page.goto("/");
    await page.getByRole("button", { name: "Create group" }).click();
    await page.getByRole("dialog").getByLabel("Group name").fill(groupName);
    await page.getByRole("dialog").getByLabel("Move static-site").click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Create group" })
      .click();

    await expect(page.getByRole("tab", { name: groupName })).toBeVisible();
    await expect
      .poll(async () => {
        const project = await getProjectByName(request, "static-site");
        return project.organization_name;
      })
      .toBe(groupName);

    await page.getByRole("tab", { name: groupName }).click();
    await page.getByRole("button", { name: `Manage ${groupName}` }).click();
    await page.getByRole("dialog").getByLabel("Group name").fill(renamedGroupName);
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Save changes" })
      .click();

    await expect(page.getByRole("tab", { name: renamedGroupName })).toBeVisible();

    await page.getByRole("tab", { name: renamedGroupName }).click();
    await page.getByRole("button", { name: `Manage ${renamedGroupName}` }).click();
    await page.getByRole("tab", { name: "Members" }).click();
    await page.getByRole("dialog").getByLabel("GitHub username").fill(inviteUsername);
    await page.getByRole("dialog").getByRole("button", { name: "Invite" }).click();
    await expect(page.getByRole("dialog")).toContainText(inviteUsername);
    await page.keyboard.press("Escape");

    const baseURL = testInfo.project.use.baseURL as string;
    const teammateContext = await browser.newContext({ baseURL });
    const teammatePage = await teammateContext.newPage();
    const login = await teammatePage.request.post("/api/auth/dev-login", {
      data: { username: inviteUsername },
    });
    expect(login.ok()).toBeTruthy();

    await teammatePage.goto("/");
    await teammatePage.getByText(inviteUsername).click();
    await teammatePage.getByRole("menuitem", { name: /Group invitations/ }).click();
    await teammatePage
      .getByRole("dialog")
      .getByRole("button", { name: "Accept" })
      .click();
    await expect
      .poll(async () => {
        const groupsRes = await teammatePage.request.get("/api/groups");
        expect(groupsRes.ok()).toBeTruthy();
        const groups = (await groupsRes.json()) as GroupResponse[];
        return groups.some((group) => group.name === renamedGroupName);
      })
      .toBe(true);
    await teammatePage.goto("/");
    await expect(
      teammatePage.getByRole("tab", { name: renamedGroupName }),
    ).toBeVisible();
    await teammateContext.close();

    await page.getByRole("button", { name: `Manage ${renamedGroupName}` }).click();
    await page.getByRole("tab", { name: "Danger" }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Delete group" })
      .click();

    await expect(page.getByRole("tab", { name: renamedGroupName })).toHaveCount(0);
    await expect
      .poll(async () => {
        const project = await getProjectByName(request, "static-site");
        return project.organization_id;
      })
      .toBe(defaultGroupId);
  });
});
