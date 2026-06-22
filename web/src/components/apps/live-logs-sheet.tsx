import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Download, Pause, Play, Terminal, WrapText, X } from "lucide-react";

import { api } from "@/lib/api";
import { queryKeys } from "@/lib/queryKeys";
import { cn } from "@/lib/utils";
import { useContainerLogs } from "@/hooks/useContainerLogs";
import { useLogTabsStore, type LogTab } from "@/store/log-tabs";
import { LogConsole } from "@/components/apps/log-console";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

const STATUS_DOT: Record<string, string> = {
  open: "bg-emerald-400",
  connecting: "bg-amber-400",
  error: "bg-rose-400",
  closed: "bg-slate-500",
  idle: "bg-slate-500",
};

export function LiveLogsSheet() {
  const tabs = useLogTabsStore((s) => s.tabs);
  const activeTabId = useLogTabsStore((s) => s.activeTabId);
  const setActiveTab = useLogTabsStore((s) => s.setActiveTab);
  const closeTab = useLogTabsStore((s) => s.closeTab);
  const closeAll = useLogTabsStore((s) => s.closeAll);

  const open = tabs.length > 0;
  const active = tabs.find((t) => t.id === activeTabId) ?? tabs[0] ?? null;

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) closeAll();
      }}
    >
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 p-0 sm:max-w-2xl"
      >
        <SheetHeader className="border-b p-4">
          <SheetTitle className="flex items-center gap-2 text-sm">
            <Terminal className="h-4 w-4" /> Live logs
          </SheetTitle>
          <SheetDescription className="text-xs">
            Realtime container output. Switch environment or branch, or open more
            containers as tabs.
          </SheetDescription>
        </SheetHeader>

        {active ? <Switchers tab={active} /> : null}

        {/* Tab bar */}
        {tabs.length > 0 ? (
          <div className="flex gap-1 overflow-x-auto border-b px-3 pt-2">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                type="button"
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  "flex shrink-0 items-center gap-1.5 rounded-t-md border border-b-0 px-2.5 py-1.5 text-xs",
                  tab.id === active?.id
                    ? "border-emerald-400/30 bg-emerald-400/[0.08] text-foreground"
                    : "border-transparent text-muted-foreground hover:text-foreground",
                )}
              >
                <span className="max-w-[120px] truncate">{tab.projectName}</span>
                <span className="text-[10px] text-muted-foreground">
                  {tab.environment === "preview" ? "dev" : "prod"}
                  {tab.branch ? `/${tab.branch}` : ""}
                </span>
                <span
                  role="button"
                  tabIndex={0}
                  aria-label="Close tab"
                  onClick={(e) => {
                    e.stopPropagation();
                    closeTab(tab.id);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      e.stopPropagation();
                      closeTab(tab.id);
                    }
                  }}
                  className="rounded p-0.5 text-muted-foreground/60 hover:text-foreground"
                >
                  <X className="h-3 w-3" />
                </span>
              </button>
            ))}
          </div>
        ) : null}

        {/* Consoles — all mounted so every tab keeps streaming. */}
        <div className="relative flex-1 overflow-hidden">
          {tabs.map((tab) => (
            <TabLogView
              key={tab.id}
              tab={tab}
              hidden={tab.id !== active?.id}
            />
          ))}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function Switchers({ tab }: { tab: LogTab }) {
  const retarget = useLogTabsStore((s) => s.retarget);
  const { data: instances } = useQuery({
    queryKey: queryKeys.previewInstances(tab.projectId),
    queryFn: () => api.listPreviewInstances(tab.projectId),
    enabled: tab.environment === "preview",
  });

  return (
    <div className="flex flex-wrap items-center gap-3 border-b p-3 text-xs">
      <span className="text-muted-foreground">Env</span>
      <div className="inline-flex rounded-md border p-0.5">
        {(["preview", "production"] as const).map((env) => (
          <button
            key={env}
            type="button"
            onClick={() => retarget(tab.id, { environment: env })}
            className={cn(
              "rounded px-2.5 py-1",
              tab.environment === env
                ? "bg-primary/15 text-primary"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {env === "preview" ? "Development" : "Production"}
          </button>
        ))}
      </div>

      {tab.environment === "preview" ? (
        <>
          <span className="text-muted-foreground">Branch</span>
          <Select
            value={tab.branch ?? ""}
            onValueChange={(v) => retarget(tab.id, { branch: v })}
          >
            <SelectTrigger className="h-7 w-[180px] text-xs">
              <SelectValue placeholder="default branch" />
            </SelectTrigger>
            <SelectContent>
              {(instances ?? []).map((pi) => (
                <SelectItem key={pi.id} value={pi.branch}>
                  {pi.branch}
                  {pi.is_default ? " (default)" : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </>
      ) : null}
    </div>
  );
}

function TabLogView({ tab, hidden }: { tab: LogTab; hidden: boolean }) {
  const { lines, status, clear } = useContainerLogs({
    projectId: tab.projectId,
    environment: tab.environment,
    branch: tab.branch,
  });
  const [paused, setPaused] = useState(false);
  const [wrap, setWrap] = useState(true);

  const download = () => {
    const blob = new Blob([lines.map((l) => l.content).join("\n")], {
      type: "text/plain",
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${tab.projectName}-${tab.environment}${tab.branch ? `-${tab.branch}` : ""}.log`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className={cn("absolute inset-0 flex flex-col", hidden && "hidden")}>
      <div className="flex items-center gap-2 border-b px-3 py-2 text-[11px]">
        <span
          className={cn(
            "h-2 w-2 shrink-0 rounded-full",
            STATUS_DOT[status] ?? "bg-slate-500",
            status === "open" && "animate-pulse",
          )}
        />
        <span className="font-mono text-muted-foreground">
          {status === "open" ? "LIVE" : status}
        </span>
        <div className="ml-auto flex items-center gap-1">
          <ToolBtn onClick={() => setPaused((p) => !p)} label={paused ? "Resume" : "Pause"}>
            {paused ? <Play className="h-3 w-3" /> : <Pause className="h-3 w-3" />}
          </ToolBtn>
          <ToolBtn onClick={() => setWrap((w) => !w)} label="Wrap" active={wrap}>
            <WrapText className="h-3 w-3" />
          </ToolBtn>
          <ToolBtn onClick={clear} label="Clear">
            Clear
          </ToolBtn>
          <ToolBtn onClick={download} label="Download">
            <Download className="h-3 w-3" />
          </ToolBtn>
        </div>
      </div>
      <LogConsole
        lines={lines}
        status={status}
        paused={paused}
        wrap={wrap}
        className="flex-1"
      />
    </div>
  );
}

function ToolBtn({
  onClick,
  label,
  active,
  children,
}: {
  onClick: () => void;
  label: string;
  active?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={label}
      aria-label={label}
      className={cn(
        "inline-flex items-center gap-1 rounded border border-border/60 px-1.5 py-0.5 text-[10px] transition-colors hover:border-primary hover:text-primary",
        active ? "border-primary/50 text-primary" : "text-muted-foreground",
      )}
    >
      {children}
    </button>
  );
}
