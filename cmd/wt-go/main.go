// wt-go dispatches to the interactive worktree picker (default) or to
// subcommands like `cleanup`. The shell wrapper `wt` forwards user commands
// that aren't handled in TypeScript (`list`, `setup`) to this binary.
package main

import (
	"fmt"
	"os"
	"strings"
)

func usage() {
	fmt.Fprintln(os.Stderr, `wt-go — worktree picker & cleanup

Usage:
  wt-go                       run the interactive worktree picker (default)
  wt-go cleanup [flags]       prune stale worktrees (dry-run by default)
  wt-go -h | --help           show this help

Run "wt-go cleanup --help" for cleanup flags.`)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runPicker(nil)
		return
	}
	switch args[0] {
	case "cleanup":
		runCleanup(args[1:])
	case "go", "pick":
		runPicker(args[1:])
	case "-h", "--help", "help":
		usage()
	default:
		// Treat unknown leading tokens (like --branches) as picker flags.
		if strings.HasPrefix(args[0], "-") {
			runPicker(args)
			return
		}
		fmt.Fprintln(os.Stderr, "wt-go: unknown command:", args[0])
		usage()
		os.Exit(2)
	}
}
