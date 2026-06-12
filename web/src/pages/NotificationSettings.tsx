import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, BellOff, Smartphone, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import {
  disablePushOnThisDevice,
  enablePushOnThisDevice,
  getCurrentEndpoint,
  isIOS,
  isPushSupported,
  isStandalone,
} from "@/lib/push";
import type { PushPreferencesUpdate, PushSubscriptionInfo } from "@/types/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

const EVENT_TOGGLES: {
  key: keyof PushPreferencesUpdate;
  label: string;
  description: string;
}[] = [
  {
    key: "notify_deploy_outcomes",
    label: "Deployment outcomes",
    description: "A deployment went live or failed.",
  },
  {
    key: "notify_build_starts",
    label: "Build starts",
    description: "A git push triggered an auto-build.",
  },
  {
    key: "notify_ssl_issues",
    label: "Domain & SSL problems",
    description: "SSL provisioning for a domain failed.",
  },
];

export function NotificationSettings() {
  const queryClient = useQueryClient();
  const [currentEndpoint, setCurrentEndpoint] = useState<string | null>(null);
  const [enabling, setEnabling] = useState(false);

  const subscriptionsQuery = useQuery({
    queryKey: queryKeys.pushSubscriptions(),
    queryFn: () => api.listPushSubscriptions(),
  });

  useEffect(() => {
    void getCurrentEndpoint().then(setCurrentEndpoint);
  }, []);

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: queryKeys.pushSubscriptions() });

  const updateMutation = useMutation({
    mutationFn: ({ id, prefs }: { id: string; prefs: PushPreferencesUpdate }) =>
      api.updatePushSubscription(id, prefs),
    onSuccess: () => invalidate(),
    onError: (err) => toast.error(err.message),
  });

  const removeMutation = useMutation({
    mutationFn: (sub: PushSubscriptionInfo) =>
      sub.endpoint === currentEndpoint
        ? disablePushOnThisDevice(sub.id)
        : api.deletePushSubscription(sub.id),
    onSuccess: () => {
      invalidate();
      toast.success("Device removed");
    },
    onError: (err) => toast.error(err.message),
  });

  // Must stay inside the tap handler — iOS only shows the permission prompt
  // for a user gesture.
  const handleEnable = async () => {
    setEnabling(true);
    try {
      await enablePushOnThisDevice();
      setCurrentEndpoint(await getCurrentEndpoint());
      await invalidate();
      toast.success("Notifications enabled on this device");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to enable");
    } finally {
      setEnabling(false);
    }
  };

  const subscriptions = subscriptionsQuery.data ?? [];
  const thisDevice = subscriptions.find((s) => s.endpoint === currentEndpoint);
  const supported = isPushSupported();
  const needsInstall = !supported && isIOS() && !isStandalone();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Notifications</h1>
        <p className="text-sm text-muted-foreground">
          Push notifications for your projects: deploys going live, failed
          builds, and SSL problems. Delivered to every device you enable.
        </p>
      </div>

      <Card data-testid="push-this-device">
        <CardHeader>
          <CardTitle>This device</CardTitle>
          <CardDescription>
            {thisDevice
              ? "Notifications are enabled. Choose which events reach this device."
              : "Enable push notifications on the device you're using right now."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {needsInstall ? (
            <div className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm">
              On iPhone, push notifications require installing Deployik first:
              open this site in Safari, tap <strong>Share</strong> →{" "}
              <strong>Add to Home Screen</strong>, then enable notifications
              from the installed app.
            </div>
          ) : !supported ? (
            <p className="text-sm text-muted-foreground">
              This browser does not support push notifications.
            </p>
          ) : !thisDevice ? (
            <Button
              className="h-11 w-full md:h-9 md:w-auto"
              onClick={handleEnable}
              disabled={enabling}
              data-testid="enable-push-button"
            >
              <Bell className="mr-2 h-4 w-4" />
              {enabling ? "Enabling..." : "Enable on this device"}
            </Button>
          ) : (
            <div className="space-y-4">
              {EVENT_TOGGLES.map((toggle) => (
                <div
                  key={toggle.key}
                  className="flex items-center justify-between gap-3"
                >
                  <div className="min-w-0">
                    <Label className="text-sm">{toggle.label}</Label>
                    <p className="text-xs text-muted-foreground">
                      {toggle.description}
                    </p>
                  </div>
                  <Switch
                    checked={thisDevice[toggle.key] as boolean}
                    disabled={updateMutation.isPending}
                    onCheckedChange={(checked) =>
                      updateMutation.mutate({
                        id: thisDevice.id,
                        prefs: { [toggle.key]: checked },
                      })
                    }
                  />
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card data-testid="push-devices">
        <CardHeader>
          <CardTitle>Registered devices</CardTitle>
          <CardDescription>
            Every device receiving push notifications for your account.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {subscriptionsQuery.isLoading ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : subscriptions.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No devices registered yet.
            </p>
          ) : (
            <ul className="divide-y divide-border">
              {subscriptions.map((sub) => (
                <li
                  key={sub.id}
                  className="flex items-center justify-between gap-3 py-3"
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <Smartphone className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="min-w-0 truncate text-sm font-medium">
                          {sub.device_label || "Unknown device"}
                        </span>
                        {sub.endpoint === currentEndpoint ? (
                          <Badge
                            variant="outline"
                            className="border-emerald-500/40 text-emerald-200"
                          >
                            This device
                          </Badge>
                        ) : null}
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Added {new Date(sub.created_at).toLocaleDateString()}
                      </p>
                    </div>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => removeMutation.mutate(sub)}
                    disabled={removeMutation.isPending}
                  >
                    {sub.endpoint === currentEndpoint ? (
                      <BellOff className="mr-1.5 h-3.5 w-3.5" />
                    ) : (
                      <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                    )}
                    Remove
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
