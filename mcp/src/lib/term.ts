// Minimal ANSI helpers — no external dependency, no color when stdout isn't a TTY.

const tty = process.stdout.isTTY === true && !process.env.NO_COLOR;

const wrap = (code: string) => (s: string) => (tty ? `\x1b[${code}m${s}\x1b[0m` : s);

export const dim = wrap("2");
export const bold = wrap("1");
export const green = wrap("32");
export const red = wrap("31");
export const yellow = wrap("33");
export const cyan = wrap("36");
export const magenta = wrap("35");

export function box(title: string, body: string[]): string {
  const lines = [bold(magenta(`╭─ ${title} `.padEnd(72, "─") + "╮")), ...body.map((l) => `  ${l}`), bold(magenta("╰" + "─".repeat(71) + "╯"))];
  return lines.join("\n");
}

export function section(num: number, total: number, title: string): string {
  return `\n${bold(cyan(`Step ${num}/${total}`))} ${dim("·")} ${bold(title)}\n${dim("─".repeat(60))}`;
}

export function ok(msg: string): string { return `${green("✓")} ${msg}`; }
export function fail(msg: string): string { return `${red("✗")} ${msg}`; }
export function info(msg: string): string { return `${cyan("→")} ${msg}`; }
export function warn(msg: string): string { return `${yellow("!")} ${msg}`; }
