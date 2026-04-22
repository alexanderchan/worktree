package main

import (
	"fmt"
	"os"

	wt "github.com/alexanderchan/wt/internal"
)

// runRecord persists a frecency hit for the given branch. Called by the wtfzf
// shell function after fzf makes a selection (since the Go binary doesn't own
// the selection in that flow).
func runRecord(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "wt-go record: usage: wt-go record <branch>")
		os.Exit(2)
	}
	branch := args[0]
	repoRoot, err := wt.GetRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wt-go record: not inside a git repository")
		os.Exit(1)
	}
	if err := wt.RecordUsage(repoRoot, branch); err != nil {
		fmt.Fprintln(os.Stderr, "wt-go record:", err)
		os.Exit(1)
	}
}
