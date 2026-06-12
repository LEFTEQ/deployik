// Tiny external store for connectivity state. `navigator.onLine` alone lies on
// iOS (stays true on dead Wi-Fi), so ApiClient also reports actual fetch
// failures/successes here.
let offline = typeof navigator !== "undefined" ? !navigator.onLine : false;
const listeners = new Set<() => void>();

function setOffline(value: boolean) {
  if (offline === value) return;
  offline = value;
  listeners.forEach((listener) => listener());
}

export function reportNetworkError() {
  setOffline(true);
}

export function reportNetworkSuccess() {
  setOffline(false);
}

export function subscribeNetworkStatus(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function isOffline(): boolean {
  return offline;
}

if (typeof window !== "undefined") {
  window.addEventListener("online", () => setOffline(false));
  window.addEventListener("offline", () => setOffline(true));
}
