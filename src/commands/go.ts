import { spawnSync } from "child_process";
import { isInsideGitRepo } from "../utils/git.js";

export async function goCommand(_params: { branch?: string }) {
  if (!isInsideGitRepo()) {
    console.error("Not inside a git repository.");
    process.exit(1);
  }

  // wt-go renders the TUI on /dev/tty, so stdout is clean for the path.
  const result = spawnSync("wt-go", [], {
    stdio: ["inherit", "pipe", "inherit"],
  });

  const path = result.stdout?.toString().trim();
  if (path) {
    process.stdout.write(path + "\n");
  }

  process.exit(result.status ?? 0);
}
