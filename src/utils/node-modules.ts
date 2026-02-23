import { readdirSync, existsSync, symlinkSync, lstatSync } from "fs";
import { join, relative } from "path";

export interface NodeModulesPlan {
  /** Relative path from repo root, e.g. "node_modules" or "client/node_modules" */
  rel: string;
  src: string;
  dest: string;
  /** "symlink" | "already-exists" | "src-missing" */
  status: "symlink" | "already-exists" | "src-missing";
}

/**
 * Find package.json files in the main worktree at root + one level deep,
 * and plan symlinking their adjacent node_modules into the target worktree.
 */
export function planNodeModulesLinks(params: {
  mainPath: string;
  worktreePath: string;
}): NodeModulesPlan[] {
  const { mainPath, worktreePath } = params;
  const plans: NodeModulesPlan[] = [];
  const seen = new Set<string>();

  function check(dir: string) {
    const rel = relative(mainPath, join(dir, "node_modules"));
    if (seen.has(rel)) return;
    seen.add(rel);

    const src = join(mainPath, rel);
    const dest = join(worktreePath, rel);

    if (!existsSync(src)) return; // no node_modules here

    let status: NodeModulesPlan["status"];
    if (existsSync(dest) || isSymlink(dest)) {
      status = "already-exists";
    } else {
      status = "symlink";
    }

    plans.push({ rel, src, dest, status });
  }

  // Root package.json
  if (existsSync(join(mainPath, "package.json"))) {
    check(mainPath);
  }

  // One level deep
  try {
    for (const entry of readdirSync(mainPath, { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      if (entry.name.startsWith(".") || entry.name === "node_modules") continue;
      const subdir = join(mainPath, entry.name);
      if (existsSync(join(subdir, "package.json"))) {
        check(subdir);
      }
    }
  } catch {
    // ignore read errors
  }

  return plans;
}

function isSymlink(path: string): boolean {
  try {
    return lstatSync(path).isSymbolicLink();
  } catch {
    return false;
  }
}

export function linkNodeModules(params: {
  plans: NodeModulesPlan[];
}): { linked: string[]; skipped: string[] } {
  const linked: string[] = [];
  const skipped: string[] = [];

  for (const plan of params.plans) {
    if (plan.status === "symlink") {
      symlinkSync(plan.src, plan.dest);
      linked.push(plan.rel);
    } else {
      skipped.push(plan.rel);
    }
  }

  return { linked, skipped };
}
