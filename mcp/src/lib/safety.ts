export type SafetyTier = "read" | "mutating" | "destructive" | "destructive_production";

export interface SafetyContext {
  toolName: string;
  tier: SafetyTier;
  /** The resource name a `confirm_name` arg must match when set. */
  expectedName?: string;
  /** What this call will do, surfaced in dry-run mode. */
  impact: Record<string, unknown>;
}

export interface SafetyArgs {
  confirm?: boolean;
  confirm_name?: string;
}

export type SafetyResult =
  | { proceed: true }
  | { proceed: false; dryRun: { tool: string; tier: SafetyTier; willDo: Record<string, unknown>; nextCall: Record<string, unknown> } };

export function checkSafety(ctx: SafetyContext, args: SafetyArgs): SafetyResult {
  if (ctx.tier === "read" || ctx.tier === "mutating") {
    return { proceed: true };
  }

  const requiresName = ctx.tier === "destructive_production" || !!ctx.expectedName;
  const required: Record<string, unknown> = { confirm: true };
  if (ctx.tier === "destructive_production") {
    if (!ctx.expectedName) {
      throw new Error(`safety: destructive_production tier requires expectedName for tool ${ctx.toolName}`);
    }
  }
  if (requiresName && ctx.expectedName) {
    required.confirm_name = ctx.expectedName;
  }

  if (args.confirm !== true) {
    return {
      proceed: false,
      dryRun: {
        tool: ctx.toolName,
        tier: ctx.tier,
        willDo: ctx.impact,
        nextCall: required,
      },
    };
  }

  if (requiresName) {
    if (args.confirm_name !== ctx.expectedName) {
      return {
        proceed: false,
        dryRun: {
          tool: ctx.toolName,
          tier: ctx.tier,
          willDo: ctx.impact,
          nextCall: required,
        },
      };
    }
  }

  return { proceed: true };
}
