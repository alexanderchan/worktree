package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	wt "github.com/alexanderchan/wt/internal"
)

func main() {
	repoRoot, err := wt.GetRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: not inside a git repository")
		os.Exit(1)
	}

	currentPath, _ := wt.GetCurrentPath()

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
	if err != nil || selected == nil {
		os.Exit(0)
	}

	_ = wt.RecordUsage(repoRoot, selected.Branch)

	if selected.IsWorktree {
		fmt.Printf("Opening shell in: %s\n", selected.Path)

		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
		if err := os.Chdir(selected.Path); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if err := syscall.Exec(shell, []string{shell}, os.Environ()); err != nil {
			fmt.Fprintln(os.Stderr, "Error exec shell:", err)
			os.Exit(1)
		}
	} else {
		// Recent branch — just git checkout in current directory
		cmd := exec.Command("git", "checkout", selected.Branch)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	}
}
