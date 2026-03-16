package internal

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
