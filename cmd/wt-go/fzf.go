package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	wt "github.com/alexanderchan/wt/internal"
)

// runFzfList prints a frecency-sorted, ANSI-colored, tab-separated list of
// worktrees (and optionally recent branches) for consumption by fzf.
//
// Output format per line:
//
//	<colored display>\t<path>\t<branch>\t<1=worktree|0=branch>
//
// The shell wrapper pipes this into fzf --ansi --with-nth=1 and extracts
// fields 2-4 from the selected line to cd / checkout.
func runFzfList(args []string) {
	fs := flag.NewFlagSet("wt-go fzf-list", flag.ExitOnError)
	showBranches := fs.Bool("branches", false, "include recent branches from git reflog")
	_ = fs.Parse(args)

	repoRoot, err := wt.GetRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wt: not inside a git repository")
		os.Exit(1)
	}

	currentPath, _ := wt.GetCurrentPath()

	worktrees, err := wt.GetWorktrees()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wt: error getting worktrees:", err)
		os.Exit(1)
	}

	mainPath := ""
	if len(worktrees) > 0 {
		mainPath = worktrees[0].Path
	}

	// Force TrueColor so lipgloss doesn't strip ANSI when stdout is a pipe.
	r := lipgloss.NewRenderer(os.Stdout)
	r.SetColorProfile(termenv.TrueColor)
	lipgloss.SetDefaultRenderer(r)

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

		item := wt.Item{
			Branch:      tree.Branch,
			Path:        tree.Path,
			DisplayPath: displayPath,
			IsWorktree:  true,
			IsCurrent:   tree.Path == currentPath,
			IsMain:      tree.IsMain,
			Head:        tree.Head,
			ReflogPos:   -1,
		}
		if t, ok := wt.LastCommitTime(tree.Path); ok {
			item.ActivityTime = &t
		}
		items = append(items, item)
	}

	reflogLen := 0
	if *showBranches {
		recentBranches, _ := wt.GetRecentBranches(10)
		reflogLen = len(recentBranches)
		for i, branch := range recentBranches {
			if !worktreeByBranch[branch] {
				items = append(items, wt.Item{
					Branch:    branch,
					ReflogPos: i,
				})
			}
		}
	}

	usage := wt.GetUsage(repoRoot)
	items = wt.ScoreItems(items, usage, reflogLen)

	for _, it := range items {
		display := renderFzfRow(it)
		isWT := "0"
		if it.IsWorktree {
			isWT = "1"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", display, it.Path, it.Branch, isWT)
	}
}

var (
	fzfBranchStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#71BEF2"))
	fzfCurrentStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A8CC8C"))
	fzfRecentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#DBAB79"))
	fzfDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	fzfHashStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#C678DD"))
	fzfStaleSty     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8FA3"))
)

func renderFzfRow(it wt.Item) string {
	var icon, branch string
	switch {
	case it.IsCurrent:
		icon = fzfCurrentStyle.Render("▶")
		branch = fzfCurrentStyle.Render(it.Branch)
	case !it.IsWorktree:
		icon = fzfRecentStyle.Render("⎇")
		branch = fzfRecentStyle.Render(it.Branch)
	case wt.IsStale(it):
		icon = fzfStaleSty.Render("●")
		branch = fzfStaleSty.Render(it.Branch)
	default:
		icon = fzfDimStyle.Render("●")
		branch = fzfBranchStyle.Render(it.Branch)
	}

	age := wt.AgeStyle(it.ActivityTime).Render(fmt.Sprintf("[%-3s]", wt.AgeShort(it.ActivityTime)))

	head := ""
	if it.Head != "" {
		head = "  " + fzfHashStyle.Render(it.Head)
	}

	count := ""
	if it.UseCount > 0 {
		count = "  " + fzfDimStyle.Render(fmt.Sprintf("%d×", it.UseCount))
	}

	return icon + " " + branch + "  " + age + head + count
}
