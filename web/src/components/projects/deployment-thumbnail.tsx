import { useEffect, useState } from "react";
import { ImageIcon, Loader2, RefreshCw } from "lucide-react";

import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

export interface DeploymentThumbnailProps {
  deploymentId: string | null | undefined;
  hasScreenshot: boolean;
  isCapturing?: boolean;
  alt?: string;
  className?: string;
  /**
   * Visual size preset. `sm` is the dashboard list thumbnail; `lg` is the
   * project-overview hero. Both render the same 16:10 aspect ratio so a
   * 1280×800 capture can be downscaled by the browser without distortion.
   */
  size?: "sm" | "lg";
  /**
   * When provided, renders a small refresh button on the thumbnail. The
   * callback is responsible for triggering the capture and any user-visible
   * feedback (toast, refetch, etc.); this component just provides the
   * affordance and prevents click-through to a parent navigation handler.
   */
  onRefresh?: () => void;
  /** Disables the refresh button while a refresh is in flight. */
  refreshing?: boolean;
}

const SIZE_CLASSES: Record<NonNullable<DeploymentThumbnailProps["size"]>, string> = {
  sm: "w-24 sm:w-28 md:w-32",
  lg: "w-full",
};

/**
 * Renders the homepage screenshot for a deployment with three visual states:
 * a `<img>` with fade-in once loaded, an animated capturing skeleton, and a
 * static empty placeholder. Used by the dashboard project rows and the
 * per-environment hero on ProjectOverview.
 */
export function DeploymentThumbnail({
  deploymentId,
  hasScreenshot,
  isCapturing,
  alt,
  className,
  size = "sm",
  onRefresh,
  refreshing,
}: DeploymentThumbnailProps) {
  const [loaded, setLoaded] = useState(false);
  const [errored, setErrored] = useState(false);
  const src = deploymentId && hasScreenshot ? api.getDeploymentScreenshotUrl(deploymentId) : null;

  // Reset load/error state whenever the underlying source changes (e.g. a
  // capture finishes and a fresh URL becomes available). Without this, the
  // previous "errored" flag would suppress the new image.
  useEffect(() => {
    setLoaded(false);
    setErrored(false);
  }, [src]);

  const frame = cn(
    "relative overflow-hidden rounded-md border bg-muted aspect-[16/10]",
    SIZE_CLASSES[size],
    className,
  );

  if (src && !errored) {
    return (
      <div className={cn("group", frame)}>
        {!loaded && <ThumbnailSkeleton capturing={false} />}
        <img
          src={src}
          alt={alt ?? "Deployment preview"}
          loading="lazy"
          onLoad={() => setLoaded(true)}
          onError={() => setErrored(true)}
          className={cn(
            "h-full w-full object-cover object-top transition-opacity duration-300",
            loaded ? "opacity-100" : "opacity-0",
          )}
        />
        {onRefresh && <RefreshButton onRefresh={onRefresh} refreshing={refreshing} />}
      </div>
    );
  }

  return (
    <div className={cn("group", frame)}>
      {isCapturing ? <ThumbnailSkeleton capturing /> : <ThumbnailPlaceholder />}
      {onRefresh && <RefreshButton onRefresh={onRefresh} refreshing={refreshing} />}
    </div>
  );
}

function RefreshButton({
  onRefresh,
  refreshing,
}: {
  onRefresh: () => void;
  refreshing?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        if (!refreshing) onRefresh();
      }}
      disabled={refreshing}
      title={refreshing ? "Capturing…" : "Refresh preview"}
      aria-label="Refresh preview"
      className={cn(
        "absolute right-1 top-1 z-10 flex h-6 w-6 items-center justify-center rounded-md border border-white/15 bg-black/60 text-white backdrop-blur-sm transition-opacity hover:bg-black/80",
        refreshing
          ? "opacity-100"
          : "opacity-0 group-hover:opacity-100 focus-visible:opacity-100",
      )}
    >
      <RefreshCw className={cn("h-3 w-3", refreshing && "animate-spin")} />
    </button>
  );
}

function ThumbnailSkeleton({ capturing }: { capturing: boolean }) {
  return (
    <div className="absolute inset-0 flex items-center justify-center bg-muted">
      <div className="absolute inset-0 animate-pulse bg-gradient-to-br from-muted via-muted/60 to-muted" />
      {capturing && (
        <div className="relative z-10 flex items-center gap-1.5 text-[10px] font-medium text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          <span>Capturing…</span>
        </div>
      )}
    </div>
  );
}

function ThumbnailPlaceholder() {
  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 text-muted-foreground/60">
      <ImageIcon className="h-5 w-5" />
      <span className="text-[10px]">No preview yet</span>
    </div>
  );
}
