import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { VitePWA } from "vite-plugin-pwa";
import path from "path";

// VITE_ALLOWED_HOSTS is a comma-separated list of hostnames the dev server will
// accept. Set this when running Vite behind a reverse proxy on a custom domain
// (e.g. `VITE_ALLOWED_HOSTS=dev.example.com,staging.example.com`). Empty /
// unset means "allow localhost only" — Vite's safe default.
const allowedHosts = (process.env.VITE_ALLOWED_HOSTS ?? "")
  .split(",")
  .map((h) => h.trim())
  .filter(Boolean);
const devPort = Number(process.env.VITE_DEV_PORT ?? "5173");
const apiProxyTarget =
  process.env.VITE_API_PROXY_TARGET ?? "http://localhost:8080";
const wsProxyTarget = apiProxyTarget.replace(/^http/, "ws");

export default defineConfig({
  root: import.meta.dirname,
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      // Silent shell updates: Deployik deploys often, so new versions must
      // activate on next launch without a prompt to maintain.
      registerType: "autoUpdate",
      // Custom worker (src/sw.ts): same Workbox precache + nav fallback as
      // generateSW, plus push/notificationclick handlers for Web Push.
      strategies: "injectManifest",
      srcDir: "src",
      filename: "sw.ts",
      includeAssets: [
        "favicon.svg",
        "favicon.ico",
        "apple-touch-icon-180x180.png",
      ],
      manifest: {
        name: "Deployik",
        short_name: "Deployik",
        description: "Self-hosted deployment platform for the Lovinka VPS",
        theme_color: "#0c111d",
        background_color: "#0c111d",
        display: "standalone",
        start_url: "/",
        icons: [
          { src: "pwa-64x64.png", sizes: "64x64", type: "image/png" },
          { src: "pwa-192x192.png", sizes: "192x192", type: "image/png" },
          { src: "pwa-512x512.png", sizes: "512x512", type: "image/png" },
          {
            src: "maskable-icon-512x512.png",
            sizes: "512x512",
            type: "image/png",
            purpose: "maskable",
          },
        ],
      },
      // Local dev never fights a service-worker cache.
      devOptions: { enabled: false },
    }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  server: {
    port: devPort,
    host: "0.0.0.0",
    ...(allowedHosts.length > 0 && { allowedHosts }),
    proxy: {
      "/api": apiProxyTarget,
      "/ws": {
        target: wsProxyTarget,
        ws: true,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
