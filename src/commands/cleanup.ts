import { spawnSync } from "child_process";
import { isInsideGitRepo } from "../utils/git.js";

export function cleanupCommand(args: string[]) {
  if (!isInsideGitRepo()) {
    console.error("Not inside a git repository.");
    process.exit(1);
  }

  const result = spawnSync("wt-go", ["cleanup", ...args], {
    stdio: "inherit",
  });

  process.exit(result.status ?? 0);
}
