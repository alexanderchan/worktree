import * as p from "@clack/prompts";
import { spawnSync } from "child_process";
import { existsSync } from "fs";
import { join } from "path";
import {
  getWorktrees,
  getCurrentWorktreePath,
  isInsideGitRepo,
} from "../utils/git.js";
import { planEnvCopy } from "../utils/env.js";

export async function goCommand(params: { branch?: string }) {
  if (!isInsideGitRepo()) {
    console.error("Not inside a git repository.");
    process.exit(1);
  }

  const worktrees = getWorktrees();
  const mainWorktree = worktrees[0];
  const currentPath = getCurrentWorktreePath();
  const others = worktrees.filter((wt) => !wt.isMain);

  if (others.length === 0) {
    p.outro("No worktrees found. Create one first with `git worktree add`.");
    process.exit(0);
  }

  let targetPath: string;

  if (params.branch) {
    // Try to find by branch name match
    const match = others.find(
      (wt) =>
        wt.branch === params.branch ||
        wt.branch.includes(params.branch!) ||
        wt.path.endsWith(params.branch!)
    );
    if (!match) {
      p.log.error(
        `No worktree found matching "${params.branch}". Available:\n` +
          others.map((wt) => `  ${wt.branch}`).join("\n")
      );
      process.exit(1);
    }
    targetPath = match.path;
  } else {
    p.intro("Worktree switcher");

    const selected = await p.select({
      message: "Which worktree?",
      options: others.map((wt) => ({
        value: wt.path,
        label: wt.branch,
        hint:
          wt.path === currentPath
            ? "← current"
            : wt.path.replace(mainWorktree.path + "/", ""),
      })),
    });

    if (p.isCancel(selected)) {
      p.cancel("Cancelled.");
      process.exit(0);
    }

    targetPath = selected as string;
  }

  // Check if setup is needed
  const envPlans = planEnvCopy({
    mainPath: mainWorktree.path,
    worktreePath: targetPath,
  });
  const missingEnvFiles = envPlans.filter((p) => !p.exists);

  const setupScriptSh = join(mainWorktree.path, "scripts/setup-worktree.sh");
  const setupScriptTs = join(mainWorktree.path, "scripts/setup-worktree.ts");
  const hasSetupSh = existsSync(setupScriptSh);
  const hasSetupTs = existsSync(setupScriptTs);
  const hasSetupScript = hasSetupSh || hasSetupTs;

  if (missingEnvFiles.length > 0 || hasSetupScript) {
    p.log.warn("This worktree may need setup:");

    if (missingEnvFiles.length > 0) {
      p.log.message(
        `  Env files to copy: ${missingEnvFiles.map((e) => e.src.split("/").pop()).join(", ")}`
      );
    }
    if (hasSetupSh) p.log.message(`  Setup script: scripts/setup-worktree.sh`);
    if (hasSetupTs) p.log.message(`  Setup script: scripts/setup-worktree.ts`);

    const runSetup = await p.confirm({
      message: "Run `wt setup` first?",
      initialValue: true,
    });

    if (p.isCancel(runSetup)) {
      p.cancel("Cancelled.");
      process.exit(0);
    }

    if (runSetup) {
      // Re-invoke the globally installed `wt setup` in the target worktree
      const result = spawnSync("wt", ["setup", "--yes"], {
        cwd: targetPath,
        stdio: "inherit",
        shell: true,
      });
      if (result.status !== 0) {
        p.log.warn("Setup exited with errors, continuing anyway...");
      }
    }
  }

  p.outro(`Opening shell in: ${targetPath}`);

  spawnSync(process.env.SHELL ?? "/bin/bash", [], {
    cwd: targetPath,
    stdio: "inherit",
  });
}
