import { execSync } from "child_process";

export interface Worktree {
  path: string;
  branch: string;
  commit: string;
  isMain: boolean;
  isLocked: boolean;
}

export function getWorktrees(): Worktree[] {
  const output = execSync("git worktree list --porcelain", {
    encoding: "utf-8",
  });

  const worktrees: Worktree[] = [];
  const blocks = output.trim().split("\n\n");

  for (const block of blocks) {
    const lines = block.trim().split("\n");
    const path = lines.find((l) => l.startsWith("worktree "))?.slice(9) ?? "";
    const branch =
      lines
        .find((l) => l.startsWith("branch "))
        ?.slice(7)
        .replace("refs/heads/", "") ?? "(detached)";
    const commit = lines.find((l) => l.startsWith("HEAD "))?.slice(5) ?? "";
    const isMain = worktrees.length === 0;
    const isLocked = lines.some((l) => l.startsWith("locked"));

    if (path) worktrees.push({ path, branch, commit, isMain, isLocked });
  }

  return worktrees;
}

export function getMainWorktree(): Worktree {
  const worktrees = getWorktrees();
  return worktrees[0];
}

export function getCurrentWorktreePath(): string {
  return execSync("git rev-parse --show-toplevel", { encoding: "utf-8" }).trim();
}

export function isInsideWorktree(): boolean {
  try {
    const current = getCurrentWorktreePath();
    const main = getMainWorktree();
    return current !== main.path;
  } catch {
    return false;
  }
}

export function isInsideGitRepo(): boolean {
  try {
    execSync("git rev-parse --show-toplevel", { encoding: "utf-8", stdio: "pipe" });
    return true;
  } catch {
    return false;
  }
}
