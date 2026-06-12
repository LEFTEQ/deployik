import { useSyncExternalStore } from "react";
import { WifiOff } from "lucide-react";

import { isOffline, subscribeNetworkStatus } from "@/lib/network-status";

/**
 * Slim connectivity banner. The PWA shell opens from cache when offline, but
 * Deployik never caches data — so the user must know requests are failing.
 */
export function OfflineBanner() {
  const offline = useSyncExternalStore(subscribeNetworkStatus, isOffline);
  if (!offline) return null;

  return (
    <div
      data-testid="offline-banner"
      className="fixed inset-x-0 top-0 z-50 bg-warning pt-safe text-warning-foreground"
    >
      <div className="flex items-center justify-center gap-2 py-1.5 text-xs font-medium">
        <WifiOff className="size-3.5" />
        You're offline — live data is unavailable
      </div>
    </div>
  );
}
