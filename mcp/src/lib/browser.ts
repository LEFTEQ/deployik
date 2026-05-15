import { spawn } from "node:child_process";
import { platform } from "node:os";

/** Best-effort: open a URL in the user's default browser. Returns true on success. */
export function openInBrowser(url: string): boolean {
  let cmd: string;
  let args: string[];
  switch (platform()) {
    case "darwin":
      cmd = "open"; args = [url]; break;
    case "win32":
      cmd = "cmd"; args = ["/c", "start", "", url]; break;
    default:
      cmd = "xdg-open"; args = [url]; break;
  }
  try {
    const child = spawn(cmd, args, { stdio: "ignore", detached: true });
    child.on("error", () => {});
    child.unref();
    return true;
  } catch {
    return false;
  }
}
