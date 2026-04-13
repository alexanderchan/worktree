#!/usr/bin/env node
import { Command } from "@commander-js/extra-typings";
import { listCommand } from "./commands/list.js";
import { goCommand } from "./commands/go.js";
import { initCommand } from "./commands/init.js";
import { setupCommand } from "./commands/setup.js";

const program = new Command()
  .name("wt")
  .description("Global worktree CLI — list, navigate, and set up git worktrees")
  .version("1.0.0");

// ── list ─────────────────────────────────────────────────────────────────────
program
  .command("list")
  .alias("ls")
  .description("List all worktrees in the current repo")
  .action(() => {
    listCommand();
  });

// ── init ────────────────────────────────────────────────────────────────────
program
  .command("init")
  .description(
    "Print shell integration code — add eval \"$(wt init)\" to your .zshrc"
  )
  .action(() => {
    initCommand();
  });

// ── go ───────────────────────────────────────────────────────────────────────
program
  .command("go")
  .argument("[branch]", "Branch name or partial path to switch to directly")
  .description(
    "Interactive worktree picker — spawns a shell in the chosen worktree"
  )
  .action(async (branch) => {
    await goCommand({ branch });
  });

// ── setup ────────────────────────────────────────────────────────────────────
program
  .command("setup")
  .description(
    "Set up the current worktree: copies .env.* files from main and runs the repo's setup script"
  )
  .option("-y, --yes", "Skip confirmation prompts")
  .option("--overwrite", "Overwrite existing .env.* files")
  .action(async (opts) => {
    await setupCommand({ yes: opts.yes, overwrite: opts.overwrite });
  });

program.parse();
