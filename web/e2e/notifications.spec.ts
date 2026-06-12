import { expect, test } from "@playwright/test";

test.describe("notification settings", () => {
  test("vapid key endpoint serves the public key", async ({ request }) => {
    const res = await request.get("/api/push/vapid-key");
    expect(res.ok()).toBeTruthy();
    const body = (await res.json()) as { public_key: string };
    expect(body.public_key.length).toBeGreaterThan(20);
  });

  test("page renders device cards and empty device list", async ({ page }) => {
    await page.goto("/account/notifications");
    await expect(page.getByTestId("push-this-device")).toBeVisible();
    await expect(page.getByTestId("push-devices")).toBeVisible();
    await expect(page.getByText("No devices registered yet.")).toBeVisible();
  });

  test("subscription CRUD over the API honors presence-aware PATCH", async ({
    request,
  }) => {
    const created = await request.post("/api/push/subscriptions", {
      data: {
        endpoint: "https://push.example/e2e-device",
        keys: { p256dh: "p", auth: "a" },
        device_label: "E2E device",
      },
    });
    expect(created.status()).toBe(201);
    const sub = (await created.json()) as {
      id: string;
      notify_build_starts: boolean;
    };

    const patched = await request.patch(`/api/push/subscriptions/${sub.id}`, {
      data: { notify_deploy_outcomes: false },
    });
    expect(patched.ok()).toBeTruthy();
    const updated = (await patched.json()) as {
      notify_deploy_outcomes: boolean;
      notify_build_starts: boolean;
    };
    expect(updated.notify_deploy_outcomes).toBe(false);
    expect(updated.notify_build_starts).toBe(true);

    const deleted = await request.delete(`/api/push/subscriptions/${sub.id}`);
    expect(deleted.status()).toBe(204);
  });
});
