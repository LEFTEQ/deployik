import { defineConfig } from "@playwright/test";

const apiPort = 18080;
const webPort = 15173;
const apiURL = `http://localhost:${apiPort}`;
const webURL = `http://localhost:${webPort}`;

export default defineConfig({
  testDir: "./e2e",
  outputDir: "./test-results",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: webURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: [
    {
      command: [
        `PORT=${apiPort}`,
        "DEV_MODE=true",
        "JWT_SECRET=e2e-jwt-secret",
        "ENCRYPTION_KEY=e2e-encryption-key",
        "DATABASE_PATH=data/deployik-e2e.db",
        "PROXY_HTML_DIR=data/e2e-proxy-html",
        "NGINX_CONF_DIR=data/e2e-nginx-conf",
        "PROXY_CERTS_DIR=data/e2e-certs",
        "SCREENSHOT_DIR=data/e2e-screenshots",
        `FRONTEND_URL=${webURL}`,
        `ALLOWED_ORIGINS=${webURL}`,
        "go run ./cmd/server/",
      ].join(" "),
      cwd: "..",
      url: `${apiURL}/api/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
    {
      command: `VITE_DEV_PORT=${webPort} VITE_API_PROXY_TARGET=${apiURL} bun run dev --host 0.0.0.0`,
      url: webURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
  ],
  projects: [
    { name: "setup", testMatch: /.*\.setup\.ts/ },
    {
      name: "chromium",
      testIgnore: /.*\.setup\.ts/,
      use: {
        storageState: "playwright/.auth/admin.json",
      },
      dependencies: ["setup"],
    },
  ],
});
