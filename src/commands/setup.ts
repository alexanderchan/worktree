import * as p from "@clack/prompts";
import { spawnSync } from "child_process";
import { existsSync } from "fs";
import { join, basename } from "path";
import {
  getCurrentWorktreePath,
  getMainWorktree,
  isInsideGitRepo,
  isInsideWorktree,
} from "../utils/git.js";
import { planEnvCopy, copyEnvFiles } from "../utils/env.js";
import { planNodeModulesLinks, linkNodeModules } from "../utils/node-modules.js";

function detectRunner(scriptPath: string): string[] {
  if (scriptPath.endsWith(".ts")) {
    // Prefer bun, fall back to tsx
    try {
      spawnSync("bun", ["--version"], { stdio: "pipe" });
      return ["bun", "run", scriptPath];
    } catch {
      return ["npx", "tsx", scriptPath];
    }
  }
  return ["bash", scriptPath];
}

export async function setupCommand(params: { yes?: boolean; overwrite?: boolean }) {
  if (!isInsideGitRepo()) {
    console.error("Not inside a git repository.");
    process.exit(1);
  }

  if (!isInsideWorktree()) {
    p.log.error("Run this from inside a worktree, not the main repo.");
    process.exit(1);
  }

  const worktreePath = getCurrentWorktreePath();
  const main = getMainWorktree();

  p.intro("Worktree setup");
  p.log.info(`Main:     ${main.path}`);
  p.log.info(`Worktree: ${worktreePath}`);

  // ── Plans ────────────────────────────────────────────────────────────────────
  const envPlans = planEnvCopy({ mainPath: main.path, worktreePath });
  const nmPlans = planNodeModulesLinks({ mainPath: main.path, worktreePath });

  // ── Setup script detection ──────────────────────────────────────────────────
  const setupScriptTs = join(main.path, "scripts/setup-worktree.ts");
  const setupScriptSh = join(main.path, "scripts/setup-worktree.sh");
  // Prefer .ts if both exist
  const setupScript = existsSync(setupScriptTs)
    ? setupScriptTs
    : existsSync(setupScriptSh)
      ? setupScriptSh
      : null;

  // ── Preview ─────────────────────────────────────────────────────────────────
  const hasWork =
    envPlans.length > 0 ||
    nmPlans.some((p) => p.status === "symlink") ||
    setupScript;

  if (!hasWork) {
    p.outro("Nothing to do — worktree already set up.");
    return;
  }

  console.log("\nPlanned actions:\n");

  // Env files
  if (envPlans.length > 0) {
    console.log("  Env files:");
    for (const plan of envPlans) {
      const file = basename(plan.src);
      if (plan.exists && !params.overwrite) {
        console.log(`    - skip      ${file}  (already exists)`);
      } else if (plan.exists && params.overwrite) {
        console.log(`    - overwrite ${file}`);
      } else {
        console.log(`    - copy      ${file}`);
      }
    }
  } else {
    console.log("  Env files: none found in main repo");
  }

  // node_modules
  if (nmPlans.length > 0) {
    console.log("\n  node_modules (symlink):");
    for (const plan of nmPlans) {
      if (plan.status === "already-exists") {
        console.log(`    - skip      ${plan.rel}  (already exists)`);
      } else {
        console.log(`    - symlink   ${plan.rel}`);
      }
    }
  } else {
    console.log("\n  node_modules: none found in main repo");
  }

  // Setup script
  if (setupScript) {
    const runner = detectRunner(setupScript);
    console.log(`\n  Setup script: ${basename(setupScript)}`);
    console.log(`    runner: ${runner.slice(0, 2).join(" ")}`);
  } else {
    console.log("\n  Setup script: none found (scripts/setup-worktree.sh or .ts)");
  }

  console.log();

  // ── Confirm ─────────────────────────────────────────────────────────────────
  if (!params.yes) {
    const proceed = await p.confirm({
      message: "Proceed with setup?",
      initialValue: true,
    });

    if (p.isCancel(proceed) || !proceed) {
      p.cancel("Cancelled.");
      process.exit(0);
    }
  }

  // ── Execute ─────────────────────────────────────────────────────────────────

  // Env files
  if (envPlans.length > 0) {
    const { copied, skipped } = copyEnvFiles({
      plans: envPlans,
      overwrite: params.overwrite,
    });
    for (const f of copied) p.log.success(`Copied   ${f}`);
    for (const f of skipped) p.log.message(`Skipped  ${f} (already exists, use --overwrite)`);
  }

  // node_modules symlinks
  if (nmPlans.length > 0) {
    const { linked, skipped } = linkNodeModules({ plans: nmPlans });
    for (const rel of linked) p.log.success(`Linked   ${rel}`);
    for (const rel of skipped) p.log.message(`Skipped  ${rel} (already exists)`);
  }

  // Setup script
  if (setupScript) {
    const runner = detectRunner(setupScript);
    p.log.step(`Running ${basename(setupScript)}...`);
    console.log();

    const result = spawnSync(runner[0], runner.slice(1), {
      cwd: worktreePath,
      stdio: "inherit",
    });

    console.log();
    if (result.status === 0) {
      p.log.success(`${basename(setupScript)} completed.`);
    } else {
      p.log.error(`${basename(setupScript)} failed (exit ${result.status}).`);
      process.exit(1);
    }
  }

  p.outro("Worktree is ready.");
}
