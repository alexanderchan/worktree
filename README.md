# wt — Global Worktree CLI

A global CLI for managing git worktrees: list them, jump into one, and set up env files + run repo-specific setup scripts.

## Installation

```bash
cd /Users/alexanderchan/dev/myprojects/skills/worktree
bun install
npm run build
npm link
```

`wt` is then available globally in your shell.

## Commands

### `wt list` / `wt ls`

List all worktrees in the current git repo. The current worktree is marked with `▶`.

```
  ▶ main [main]
      /Users/you/projects/my-repo  (af67ab3)
    feature-branch
      /Users/you/projects/my-repo/.claude/worktrees/feature-branch  (af67ab3)
```

---

### `wt go [branch]`

Interactive worktree picker that spawns a shell in the selected worktree.

```bash
wt go                  # interactive picker
wt go feature-branch   # jump directly by branch name or partial match
```

Before opening the shell, `wt go` checks whether the target worktree needs setup (missing `.env.*` files, or a setup script exists in the repo). If so, it offers to run `wt setup --yes` first.

---

### `wt setup`

Run from inside a worktree to set it up. Shows a preview of what will happen, asks for confirmation, then:

1. Copies `.env.*` files from the main worktree (see [Env file handling](#env-file-handling))
2. Symlinks `node_modules` directories from the main worktree (see [node_modules linking](#node_modules-linking))
3. Runs the repo's setup script (see [Setup script discovery](#setup-script-discovery))

```bash
wt setup               # preview + confirm
wt setup --yes         # skip confirmation
wt setup --overwrite   # also overwrite existing .env.* files
```

**Example output:**

```
┌  Worktree setup
│
●  Main:     /Users/you/projects/my-repo
●  Worktree: /Users/you/projects/my-repo/.claude/worktrees/feature-branch

Planned actions:

  Env files:
    - copy      .env.local
    - skip      .env  (already exists)

  node_modules (symlink):
    - symlink   node_modules
    - symlink   client/node_modules

  Setup script: setup-worktree.sh
    runner: bash

│
◆  Copied   .env.local
│
◆  Linked   node_modules
◆  Linked   client/node_modules
│
◇  Running setup-worktree.sh...
  ...script output...
│
◆  setup-worktree.sh completed.
│
└  Worktree is ready.
```

## How it works

### Env file handling

`wt setup` globs all `.env.*` files from the root of the main worktree and copies them to the current worktree. Files are excluded if their name contains `ample` or `example` (case-insensitive), which covers patterns like:

- `.env.example` — excluded
- `.env.sample` — excluded
- `.env.local.example` — excluded
- `.env.local` — copied
- `.env.docker-test` — copied

Existing files are skipped unless you pass `--overwrite`.

### node_modules linking

`wt setup` scans for `package.json` files in the main worktree at the root and one level deep (e.g. `client/package.json`, `frontend/package.json`). For each one found, it symlinks the adjacent `node_modules` directory into the worktree — so install doesn't need to be re-run for each branch.

Already-symlinked or existing `node_modules` directories are skipped.

### Setup script discovery

`wt setup` looks for a repo-specific setup script in this order:

1. `scripts/setup-worktree.ts` — runs with `bun run` (falls back to `npx tsx`)
2. `scripts/setup-worktree.sh` — runs with `bash`

If neither exists, the script step is skipped silently. The script runs with its `cwd` set to the worktree being set up.

---

## Development

### Project structure

```
src/
  index.ts              # CLI entry point (commander setup)
  commands/
    list.ts             # wt list
    go.ts               # wt go
    setup.ts            # wt setup
  utils/
    git.ts              # git worktree parsing helpers
    env.ts              # .env.* file discovery and copy logic
dist/
  wt.js                 # bundled output (committed, used by npm link)
```

### Dev workflow

Run directly with Bun during development — no build step needed:

```bash
bun run src/index.ts list
bun run src/index.ts go
bun run src/index.ts setup --yes
```

Or use the `dev` script:

```bash
npm run dev -- setup --yes
```

### Building

```bash
npm run build          # bundles to dist/wt.js (Node.js target)
npm run build:compile  # compiles to a standalone binary at dist/wt (no Node required)
```

The `build` script uses `bun build --target=node`, so the output runs on Node.js without Bun installed. The `build:compile` output embeds the Bun runtime and needs no runtime at all.

### Linking / re-linking

After rebuilding, the global `wt` command picks up changes automatically since `npm link` points directly at `dist/wt.js`.

If you need to re-link (e.g. after moving the repo):

```bash
npm link
```

### Adding a new command

1. Create `src/commands/my-command.ts` and export an async function
2. Import and register it in `src/index.ts` with `program.command(...)`
3. Run `npm run build`

### Key dependencies

| Package | Purpose |
|---|---|
| `@commander-js/extra-typings` | CLI argument parsing with full TypeScript inference |
| `@clack/prompts` | Interactive prompts, spinners, and styled terminal output |
