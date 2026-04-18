import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RefreshCcw } from "lucide-react";

import { Button } from "@/components/ui/button";

interface Props {
  children: ReactNode;
  fallback?: (error: Error, reset: () => void) => ReactNode;
  /** Human-readable scope name used in the default fallback, e.g. "Project" or "Workspace". */
  scope?: string;
}

interface State {
  error: Error | null;
}

/**
 * Wraps a subtree and catches render/effect errors so they don't blank the
 * entire SPA. Rendered once per major layout so one misbehaving page stays
 * contained to the affected content area.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // Surface in the console so operators still see the stack even after the
    // fallback UI renders. Send to an observability backend here if you add one.
    console.error("ErrorBoundary caught error:", error, info.componentStack);
  }

  reset = () => {
    this.setState({ error: null });
  };

  render() {
    if (this.state.error) {
      if (this.props.fallback) {
        return this.props.fallback(this.state.error, this.reset);
      }
      const scope = this.props.scope ?? "page";
      return (
        <div className="flex min-h-[320px] items-center justify-center p-6">
          <div className="w-full max-w-md rounded-lg border border-destructive/30 bg-destructive/5 p-6 text-center">
            <div className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-destructive/15">
              <AlertTriangle className="h-5 w-5 text-destructive" />
            </div>
            <h2 className="text-base font-semibold">
              Something went wrong loading this {scope}.
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              {this.state.error.message || "An unexpected error occurred."}
            </p>
            <div className="mt-4 flex items-center justify-center gap-2">
              <Button variant="outline" size="sm" onClick={this.reset}>
                <RefreshCcw className="mr-1.5 h-3.5 w-3.5" />
                Try again
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => window.location.reload()}
              >
                Reload page
              </Button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
