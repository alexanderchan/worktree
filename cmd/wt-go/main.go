package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	wt "github.com/alexanderchan/wt/internal"
)

func main() {
	repoRoot, err := wt.GetRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: not inside a git repository")
		os.Exit(1)
	}

	currentPath, err := wt.GetCurrentPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wt: warning: could not determine current directory: %v\n", err)
	}

	worktrees, err := wt.GetWorktrees()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error getting worktrees:", err)
		os.Exit(1)
	}

	mainPath := ""
	if len(worktrees) > 0 {
		mainPath = worktrees[0].Path
	}

	worktreeByBranch := make(map[string]bool)
	var items []wt.Item

	for _, tree := range worktrees {
		if tree.Branch == "(detached)" {
			continue
		}
		worktreeByBranch[tree.Branch] = true

		displayPath := tree.Path
		if mainPath != "" && strings.HasPrefix(tree.Path, mainPath+"/") {
			displayPath = "." + tree.Path[len(mainPath):]
		}
		if tree.IsPrunable {
			displayPath += " (prunable)"
		}

		items = append(items, wt.Item{
			Branch:      tree.Branch,
			Path:        tree.Path,
			DisplayPath: displayPath,
			IsWorktree:  true,
			IsCurrent:   tree.Path == currentPath,
			IsMain:      tree.IsMain,
			Head:        tree.Head,
			ReflogPos:   -1,
		})
	}

	recentBranches, _ := wt.GetRecentBranches(10)
	for i, branch := range recentBranches {
		if !worktreeByBranch[branch] {
			items = append(items, wt.Item{
				Branch:    branch,
				ReflogPos: i,
			})
		}
	}

	usage := wt.GetUsage(repoRoot)
	items = wt.ScoreItems(items, usage, len(recentBranches))

	selected, err := wt.ShowSelection(items)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wt: error:", err)
		os.Exit(1)
	}
	if selected == nil {
		os.Exit(0)
	}

	if selected.IsWorktree {
		_ = wt.RecordUsage(repoRoot, selected.Branch)
		fmt.Println(selected.Path)
	} else {
		// Recent branch — git checkout in current directory.
		cmd := exec.Command("git", "checkout", selected.Branch)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
		// Record usage only after checkout succeeded.
		_ = wt.RecordUsage(repoRoot, selected.Branch)
	}
}
