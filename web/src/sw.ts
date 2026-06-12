/// <reference lib="webworker" />
// Custom service worker (vite-plugin-pwa injectManifest mode).
//
// Precache behavior is identical to the previous generateSW setup: the built
// shell is precached and navigations fall back to the cached index.html,
// while /api and /ws are NEVER touched by the worker — a deploy dashboard
// must not serve stale data. The custom worker exists only because push and
// notificationclick handlers can't be expressed in generateSW config.
import { clientsClaim } from "workbox-core";
import {
  cleanupOutdatedCaches,
  createHandlerBoundToURL,
  precacheAndRoute,
} from "workbox-precaching";
import { NavigationRoute, registerRoute } from "workbox-routing";

declare let self: ServiceWorkerGlobalScope;

// Silent updates: activate immediately on next launch, no prompt.
self.skipWaiting();
clientsClaim();

cleanupOutdatedCaches();
precacheAndRoute(self.__WB_MANIFEST);

registerRoute(
  new NavigationRoute(createHandlerBoundToURL("index.html"), {
    denylist: [/^\/api\//, /^\/ws\//],
  }),
);

interface PushMessage {
  title?: string;
  body?: string;
  url?: string;
  tag?: string;
}

self.addEventListener("push", (event) => {
  if (!event.data) return;
  let msg: PushMessage;
  try {
    msg = event.data.json();
  } catch {
    return;
  }
  event.waitUntil(
    self.registration.showNotification(msg.title ?? "Deployik", {
      body: msg.body ?? "",
      icon: "/pwa-192x192.png",
      badge: "/pwa-192x192.png",
      // Same tag (e.g. one per deployment) replaces instead of stacking.
      tag: msg.tag,
      data: { url: msg.url ?? "/" },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url: string = event.notification.data?.url ?? "/";
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({
        type: "window",
        includeUncontrolled: true,
      });
      // Reuse an open Deployik window when there is one (the installed PWA),
      // otherwise open a new one.
      for (const client of windows) {
        await client.focus();
        if ("navigate" in client) {
          await client.navigate(url);
        }
        return;
      }
      await self.clients.openWindow(url);
    })(),
  );
});
