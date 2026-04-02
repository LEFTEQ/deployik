export const AUDIENCE_STATUS_META: Record<
  string,
  { label: string; badgeClass: string; description: string }
> = {
  provisioning: {
    label: "Provisioning",
    badgeClass: "border-sky-400/25 bg-sky-400/12 text-sky-100",
    description: "Deployik is creating or syncing the linked Umami website.",
  },
  ready_to_install: {
    label: "Ready to install",
    badgeClass: "border-primary/25 bg-primary/12 text-primary",
    description:
      "The website exists. Add the tracker to start collecting audience data.",
  },
  waiting_for_data: {
    label: "Waiting for data",
    badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100",
    description:
      "Tracking is configured, but Umami has not seen recent traffic yet.",
  },
  receiving_data: {
    label: "Receiving data",
    badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100",
    description: "Audience analytics is live and receiving traffic.",
  },
  stale: {
    label: "No recent data",
    badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100",
    description:
      "This project has historical traffic, but nothing recent in the selected window.",
  },
  unavailable: {
    label: "Unavailable",
    badgeClass: "border-white/10 bg-white/5 text-slate-200",
    description: "Umami is not configured on this Deployik instance.",
  },
  error: {
    label: "Error",
    badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100",
    description:
      "Deployik could not provision or query the linked analytics website.",
  },
};
