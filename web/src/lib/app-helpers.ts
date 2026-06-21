import type {
  AppCombinedStatus,
  AppRelease,
  MemberLiveStatus,
} from "@/types/api";

type StatusMeta = { label: string; badgeClass: string; dotClass: string };

export const APP_STATUS_META = {
  healthy: { label: "Healthy", badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100", dotClass: "bg-emerald-400" },
  deploying: { label: "Deploying", badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100", dotClass: "bg-amber-400" },
  degraded: { label: "Degraded", badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100", dotClass: "bg-orange-400" },
  down: { label: "Down", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  none: { label: "No deploys", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
} satisfies Record<AppCombinedStatus, StatusMeta>;

export const MEMBER_STATUS_META = {
  healthy: { label: "Healthy", badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100", dotClass: "bg-emerald-400" },
  deploying: { label: "Deploying", badgeClass: "border-amber-400/25 bg-amber-400/12 text-amber-100", dotClass: "bg-amber-400" },
  degraded: { label: "Degraded", badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100", dotClass: "bg-orange-400" },
  failed: { label: "Failed", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  down: { label: "Down", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  none: { label: "No deploys", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
  unknown: { label: "Unknown", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
} satisfies Record<MemberLiveStatus, StatusMeta>;

export const RELEASE_STATUS_META = {
  succeeded: { label: "Succeeded", badgeClass: "border-emerald-400/25 bg-emerald-400/12 text-emerald-100", dotClass: "bg-emerald-400" },
  failed: { label: "Failed", badgeClass: "border-rose-400/25 bg-rose-400/12 text-rose-100", dotClass: "bg-rose-400" },
  rolled_back: { label: "Rolled back", badgeClass: "border-orange-400/25 bg-orange-400/12 text-orange-100", dotClass: "bg-orange-400" },
  pending: { label: "Pending", badgeClass: "border-white/10 bg-white/5 text-slate-200", dotClass: "bg-slate-500" },
} satisfies Record<AppRelease["status"], StatusMeta>;

export const ACTIVE_MEMBER_STATUSES = new Set<MemberLiveStatus>(["deploying"]);
