export interface PollUntilOptions {
  intervalMs: number;
  timeoutMs: number;
  /** Optional initial delay before the first probe. */
  initialDelayMs?: number;
}

export async function pollUntil<T>(
  probe: () => Promise<T>,
  predicate: (value: T) => boolean,
  opts: PollUntilOptions,
): Promise<{ done: boolean; value: T; elapsedMs: number }> {
  const start = Date.now();
  if (opts.initialDelayMs && opts.initialDelayMs > 0) {
    await sleep(opts.initialDelayMs);
  }
  let last: T = await probe();
  if (predicate(last)) return { done: true, value: last, elapsedMs: Date.now() - start };
  while (Date.now() - start < opts.timeoutMs) {
    await sleep(opts.intervalMs);
    last = await probe();
    if (predicate(last)) return { done: true, value: last, elapsedMs: Date.now() - start };
  }
  return { done: false, value: last, elapsedMs: Date.now() - start };
}

export function sleep(ms: number): Promise<void> {
  return new Promise((res) => setTimeout(res, ms));
}
