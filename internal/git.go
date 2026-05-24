package internal

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type WorktreeInfo struct {
	Path      string
	Head      string // short hash
	Branch    string
	IsMain    bool
	IsLocked  bool
	IsPrunable bool
}

func GetRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// GetCurrentPath returns the absolute path of the worktree the user is
// currently inside, using the working directory rather than git's idea of
// the repo root. This correctly identifies which worktree is active.
func GetCurrentPath() (string, error) {
	return os.Getwd()
}

func GetWorktrees() ([]WorktreeInfo, error) {
	out, err := exec.Command("git", "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, err
	}

	var trees []WorktreeInfo
	var cur WorktreeInfo
	isFirst := true

	flush := func() {
		if cur.Path != "" {
			cur.IsMain = isFirst
			isFirst = false
			trees = append(trees, cur)
			cur = WorktreeInfo{}
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			h := strings.TrimPrefix(line, "HEAD ")
			if len(h) > 7 {
				h = h[:7]
			}
			cur.Head = h
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Branch = "(detached)"
		case strings.HasPrefix(line, "locked"):
			cur.IsLocked = true
		case strings.HasPrefix(line, "prunable"):
			cur.IsPrunable = true
		}
	}
	flush() // handle missing trailing newline

	return trees, nil
}

// looksLikeHash returns true if s looks like a git commit SHA (hex, 7–40 chars).
func looksLikeHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// WorktreeActivityTime returns the most recent of:
//   - HEAD commit time
//   - worktree directory mtime (changes when top-level files are added/removed)
//   - the per-worktree gitdir index mtime (touched by git add / commit / stash)
//
// Falls back through whatever signals are available; returns (zero, false) only
// when nothing usable is found. Using the index mtime catches activity in
// worktrees where the user is editing files but hasn't committed yet — pure
// HEAD-commit-time made those look stale.
func WorktreeActivityTime(path string) (time.Time, bool) {
	var best time.Time
	bump := func(t time.Time) {
		if t.After(best) {
			best = t
		}
	}
	if t, ok := LastCommitTime(path); ok {
		bump(t)
	}
	if info, err := os.Stat(path); err == nil {
		bump(info.ModTime())
	}
	if gitdir, ok := resolveGitDir(path); ok {
		if info, err := os.Stat(filepath.Join(gitdir, "index")); err == nil {
			bump(info.ModTime())
		}
	}
	if best.IsZero() {
		return time.Time{}, false
	}
	return best, true
}

// resolveGitDir returns the actual gitdir for a worktree path. The worktree's
// `.git` is a file containing `gitdir: <path>` pointing into the main repo's
// `.git/worktrees/<name>/` directory; the main worktree has a real `.git` dir.
func resolveGitDir(worktreePath string) (string, bool) {
	gitPath := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return gitPath, true
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	dir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(worktreePath, dir)
	}
	return dir, true
}

// LastCommitTime returns the commit timestamp of HEAD in the given worktree path.
func LastCommitTime(path string) (time.Time, bool) {
	out, err := exec.Command("git", "-C", path, "log", "-1", "--format=%ct").Output()
	if err != nil {
		return time.Time{}, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return time.Time{}, false
	}
	ts, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(ts, 0), true
}

// GetRecentBranches returns up to `limit` unique branch names from the git
// reflog checkout history, skipping detached-HEAD SHA entries.
func GetRecentBranches(limit int) ([]string, error) {
	// Fetch more raw entries than we need; reflog counts entries not unique branches.
	out, err := exec.Command("git", "reflog", "--format=%gs", "-n", "1000").Output()
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var branches []string

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "checkout: moving") {
			continue
		}
		idx := strings.LastIndex(line, " to ")
		if idx < 0 {
			continue
		}
		branch := strings.TrimSpace(line[idx+4:])
		if branch == "" || seen[branch] || looksLikeHash(branch) {
			continue
		}
		seen[branch] = true
		branches = append(branches, branch)
		if len(branches) >= limit {
			break
		}
	}

	return branches, nil
}
