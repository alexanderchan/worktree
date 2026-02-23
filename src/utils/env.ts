import { readdirSync, existsSync, copyFileSync } from "fs";
import { join, basename } from "path";

/**
 * Find all .env.* files in a directory, excluding examples/samples.
 * Excludes patterns like: .env.example, .env.sample, .env.ample, .env.local.example, etc.
 */
export function findEnvFiles(dir: string): string[] {
  if (!existsSync(dir)) return [];

  return readdirSync(dir).filter((file) => {
    if (!file.startsWith(".env")) return false;
    // Exclude example/sample files
    const lower = file.toLowerCase();
    return !lower.includes("ample") && !lower.includes("example");
  });
}

export interface EnvCopyPlan {
  /** Source path in main worktree */
  src: string;
  /** Destination path in target worktree */
  dest: string;
  /** Whether dest already exists */
  exists: boolean;
}

export function planEnvCopy(params: {
  mainPath: string;
  worktreePath: string;
}): EnvCopyPlan[] {
  const { mainPath, worktreePath } = params;
  const envFiles = findEnvFiles(mainPath);

  return envFiles.map((file) => ({
    src: join(mainPath, file),
    dest: join(worktreePath, file),
    exists: existsSync(join(worktreePath, file)),
  }));
}

export function copyEnvFiles(params: {
  plans: EnvCopyPlan[];
  overwrite?: boolean;
}): { copied: string[]; skipped: string[] } {
  const { plans, overwrite = false } = params;
  const copied: string[] = [];
  const skipped: string[] = [];

  for (const plan of plans) {
    if (plan.exists && !overwrite) {
      skipped.push(basename(plan.dest));
    } else {
      copyFileSync(plan.src, plan.dest);
      copied.push(basename(plan.dest));
    }
  }

  return { copied, skipped };
}
