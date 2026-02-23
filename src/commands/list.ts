import { getWorktrees, getCurrentWorktreePath, isInsideGitRepo } from "../utils/git.js";

export function listCommand() {
  if (!isInsideGitRepo()) {
    console.error("Not inside a git repository.");
    process.exit(1);
  }

  const worktrees = getWorktrees();
  const currentPath = getCurrentWorktreePath();

  console.log(`\nWorktrees for this repo:\n`);

  for (const wt of worktrees) {
    const isCurrent = wt.path === currentPath;
    const tag = wt.isMain ? " [main]" : "";
    const locked = wt.isLocked ? " [locked]" : "";
    const cursor = isCurrent ? "▶" : " ";
    const shortCommit = wt.commit.slice(0, 7);

    console.log(`  ${cursor} ${wt.branch}${tag}${locked}`);
    console.log(`      ${wt.path}  (${shortCommit})`);
  }

  console.log();
}
