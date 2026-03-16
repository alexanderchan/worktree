package internal

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
)

type WorktreeInfo struct {
	Path     string
	Head     string // short hash
	Branch   string
	IsMain   bool
	IsLocked bool
}

func GetRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

func GetCurrentPath() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func GetWorktrees() ([]WorktreeInfo, error) {
	out, err := exec.Command("git", "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, err
	}

	var trees []WorktreeInfo
	var cur WorktreeInfo
	isFirst := true

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if cur.Path != "" {
				cur.IsMain = isFirst
				isFirst = false
				trees = append(trees, cur)
				cur = WorktreeInfo{}
			}
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
		}
	}
	// Handle last block (no trailing newline)
	if cur.Path != "" {
		cur.IsMain = isFirst
		trees = append(trees, cur)
	}

	return trees, nil
}

// GetRecentBranches returns up to `limit` unique branches from git reflog checkout history.
func GetRecentBranches(limit int) ([]string, error) {
	out, err := exec.Command("git", "reflog", "--format=%gs", "-n", "500").Output()
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
		if branch == "" || seen[branch] {
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
