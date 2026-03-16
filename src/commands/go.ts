import { spawnSync } from "child_process";
import { isInsideGitRepo } from "../utils/git.js";

export async function goCommand(_params: { branch?: string }) {
  if (!isInsideGitRepo()) {
    console.error("Not inside a git repository.");
    process.exit(1);
  }

  const result = spawnSync("wt-go", [], { stdio: "inherit" });
  process.exit(result.status ?? 0);
}
