import { api } from "@/lib/api";
import type { PushSubscriptionInfo } from "@/types/api";

// Browser-side Web Push plumbing. iOS supports push only for PWAs installed
// to the home screen (16.4+), and the permission prompt must come from a
// user gesture — enablePushOnThisDevice is called from a button tap.

export function isPushSupported(): boolean {
  return (
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

export function isStandalone(): boolean {
  return (
    window.matchMedia("(display-mode: standalone)").matches ||
    ("standalone" in navigator &&
      (navigator as { standalone?: boolean }).standalone === true)
  );
}

export function isIOS(): boolean {
  return (
    /iPad|iPhone|iPod/.test(navigator.userAgent) ||
    // iPadOS 13+ reports as macOS but has touch points.
    (navigator.userAgent.includes("Mac") && navigator.maxTouchPoints > 1)
  );
}

function deviceLabel(): string {
  const ua = navigator.userAgent;
  let device = "Browser";
  if (/iPhone/.test(ua)) device = "iPhone";
  else if (
    /iPad/.test(ua) ||
    (ua.includes("Mac") && navigator.maxTouchPoints > 1)
  )
    device = "iPad";
  else if (/Android/.test(ua)) device = "Android";
  else if (/Mac/.test(ua)) device = "Mac";
  else if (/Windows/.test(ua)) device = "Windows";
  else if (/Linux/.test(ua)) device = "Linux";
  return isStandalone() ? `${device} (app)` : device;
}

function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padding = "=".repeat((4 - (base64.length % 4)) % 4);
  const normalized = (base64 + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = window.atob(normalized);
  const output = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) {
    output[i] = raw.charCodeAt(i);
  }
  return output;
}

/** Endpoint of this browser's existing push subscription, if any. */
export async function getCurrentEndpoint(): Promise<string | null> {
  if (!isPushSupported()) return null;
  const registration = await navigator.serviceWorker.getRegistration();
  if (!registration) return null;
  const subscription = await registration.pushManager.getSubscription();
  return subscription?.endpoint ?? null;
}

/**
 * Full enable flow: permission prompt -> pushManager.subscribe -> register
 * with the API. Must run inside a user-gesture handler (iOS requirement).
 */
export async function enablePushOnThisDevice(): Promise<PushSubscriptionInfo> {
  if (!isPushSupported()) {
    throw new Error("Push notifications are not supported in this browser");
  }

  const permission = await Notification.requestPermission();
  if (permission !== "granted") {
    throw new Error("Notification permission was not granted");
  }

  const registration = await navigator.serviceWorker.ready;
  const { public_key } = await api.getPushVapidKey();

  let subscription = await registration.pushManager.getSubscription();
  if (!subscription) {
    subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(public_key) as BufferSource,
    });
  }

  const json = subscription.toJSON();
  if (!json.endpoint || !json.keys?.p256dh || !json.keys?.auth) {
    throw new Error("Browser returned an incomplete push subscription");
  }

  return api.subscribePush({
    endpoint: json.endpoint,
    keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
    device_label: deviceLabel(),
  });
}

/** Unsubscribe this browser locally and remove the server-side registration. */
export async function disablePushOnThisDevice(
  subscriptionId: string,
): Promise<void> {
  if (isPushSupported()) {
    const registration = await navigator.serviceWorker.getRegistration();
    const subscription = await registration?.pushManager.getSubscription();
    await subscription?.unsubscribe();
  }
  await api.deletePushSubscription(subscriptionId);
}
