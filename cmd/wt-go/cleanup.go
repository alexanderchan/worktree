package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"

	wt "github.com/alexanderchan/wt/internal"
)

// Cleanup output styles (non-TUI — rendered to stderr/stdout directly).
var (
	cleanupBold   = lipgloss.NewStyle().Bold(true)
	cleanupCyan   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	cleanupGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A8CC8C"))
	cleanupYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5C07B"))
	cleanupRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75"))
	cleanupDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type WorktreeRowInfo struct {
	Path     string
	Branch   string
	MTime    time.Time
	Missing  bool // path doesn't exist on disk
	Prunable bool
}

func ageDays(now time.Time, mtime time.Time, missing bool) string {
	if missing {
		return "missing"
	}
	days := int(now.Sub(mtime).Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

// runCleanup implements `wt-go cleanup`. Scans all non-main worktrees, groups
// them into keeping/stale by directory mtime, and optionally removes stale
// trees with `git worktree remove`.
func runCleanup(args []string) {
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	days := fs.Int("days", 7, "age threshold in days (dirs older than this are stale)")
	doDelete := fs.Bool("delete", false, "actually remove stale worktrees (default: dry-run)")
	force := fs.Bool("force", false, "pass --force to `git worktree remove` (required for dirty trees)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, cleanupCyan.Render("wt-go cleanup")+" — prune stale git worktrees")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, cleanupBold.Render("Usage:"))
		fmt.Fprintln(os.Stderr, "  wt-go cleanup [--days N] [--delete] [--force]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, cleanupBold.Render("Flags:"))
		fmt.Fprintln(os.Stderr, "  --days N   Age threshold in days (default: 7)")
		fmt.Fprintln(os.Stderr, "  --delete   Actually remove stale worktrees (default: dry-run)")
		fmt.Fprintln(os.Stderr, "  --force    Pass --force to `git worktree remove` (dirty trees)")
	}
	_ = fs.Parse(args)

	worktrees, err := wt.GetWorktrees()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	now := time.Now()
	cutoff := now.Add(-time.Duration(*days) * 24 * time.Hour)

	mode := "dry-run"
	if *doDelete {
		mode = "delete"
	}
	bar := cleanupCyan.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(bar)
	fmt.Println(cleanupCyan.Render(fmt.Sprintf("  Worktree cleanup — mode: %s, age > %dd", mode, *days)))
	fmt.Println(bar)
	fmt.Println()

	var keeping, stale []WorktreeRowInfo
	for _, tree := range worktrees {
		if tree.IsMain {
			continue
		}
		info := WorktreeRowInfo{Path: tree.Path, Branch: tree.Branch, Prunable: tree.IsPrunable}
		if st, err := os.Stat(tree.Path); err == nil {
			info.MTime = st.ModTime()
		} else {
			info.Missing = true
		}
		if info.Missing || info.MTime.Before(cutoff) {
			stale = append(stale, info)
		} else {
			keeping = append(keeping, info)
		}
	}

	// Sort each bucket: newest first for keeping, oldest first for stale.
	sort.Slice(keeping, func(i, j int) bool { return keeping[i].MTime.After(keeping[j].MTime) })
	sort.Slice(stale, func(i, j int) bool {
		if stale[i].Missing != stale[j].Missing {
			return stale[i].Missing
		}
		return stale[i].MTime.Before(stale[j].MTime)
	})

	printBucket := func(label string, rows []WorktreeRowInfo, styleAge lipgloss.Style) {
		fmt.Println(cleanupBold.Render(fmt.Sprintf("%s (%d):", label, len(rows))))
		if len(rows) == 0 {
			fmt.Println(cleanupDim.Render("  (none)"))
		}
		for _, r := range rows {
			age := fmt.Sprintf("[%s]", ageDays(now, r.MTime, r.Missing))
			name := r.Branch
			if name == "" || name == "(detached)" {
				name = r.Path
			}
			if r.Prunable {
				name += cleanupDim.Render(" (prunable)")
			}
			fmt.Printf("  %s %s\n", styleAge.Render(fmt.Sprintf("%-7s", age)), name)
		}
		fmt.Println()
	}

	printBucket("Keeping", keeping, cleanupDim)
	printBucket("Stale", stale, cleanupYellow)

	if len(stale) == 0 {
		fmt.Println(cleanupGreen.Render("Nothing to remove."))
		return
	}

	if !*doDelete {
		fmt.Println(cleanupCyan.Render(fmt.Sprintf("Dry-run. Re-run with --delete to remove %d stale worktree(s).", len(stale))))
		return
	}

	var failed []WorktreeRowInfo
	for _, r := range stale {
		label := r.Branch
		if label == "" || label == "(detached)" {
			label = r.Path
		}
		fmt.Println(cleanupBold.Render("Removing ") + label)
		removeArgs := []string{"worktree", "remove"}
		if *force {
			removeArgs = append(removeArgs, "--force")
		}
		removeArgs = append(removeArgs, r.Path)
		cmd := exec.Command("git", removeArgs...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			failed = append(failed, r)
		}
	}

	// Always prune metadata at the end.
	if err := exec.Command("git", "worktree", "prune").Run(); err == nil {
		fmt.Println(cleanupGreen.Render("Done. Pruned metadata."))
	}

	if len(failed) > 0 {
		fmt.Println()
		fmt.Println(cleanupYellow.Render(fmt.Sprintf("Skipped %d (dirty or locked). Re-run with --delete --force to force:", len(failed))))
		for _, r := range failed {
			name := r.Branch
			if name == "" || name == "(detached)" {
				name = r.Path
			}
			fmt.Println("  " + cleanupRed.Render(name))
		}
		os.Exit(1)
	}
}
