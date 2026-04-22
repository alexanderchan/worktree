const SHELL_FUNCTION = `
# wt shell integration — enables "wt go" to cd in the current shell
wt() {
  if [ "$1" = "go" ]; then
    shift
    local dir
    dir=$(command wt go "$@")
    if [ $? -eq 0 ] && [ -n "$dir" ]; then
      cd "$dir"
      if [ "\${WORKTREE_RUN_SETUP_ON_GO:-}" = "true" ]; then
        command wt setup
      fi
    fi
  else
    command wt "$@"
  fi
}

# wtfzf — fzf-powered worktree picker with preview pane (requires fzf)
wtfzf() {
  local sel dir branch is_wt
  sel=$(command wt-go fzf-list "$@" | \\
    fzf --ansi \\
        --with-nth=1 \\
        --delimiter=$'\\t' \\
        --preview='git -C {2} log -1 --stat --color=always 2>/dev/null; echo; git -C {2} status --short 2>/dev/null' \\
        --preview-window='right:50%:wrap') || return 0
  dir=$(printf '%s' "\$sel" | awk -F'\\t' '{print \$2}')
  branch=$(printf '%s' "\$sel" | awk -F'\\t' '{print \$3}')
  is_wt=$(printf '%s' "\$sel" | awk -F'\\t' '{print \$4}')
  if [ "\$is_wt" = "1" ] && [ -n "\$dir" ]; then
    cd "\$dir"
    command wt-go record "\$branch" 2>/dev/null
    if [ "\${WORKTREE_RUN_SETUP_ON_GO:-}" = "true" ]; then
      command wt setup
    fi
  elif [ -n "\$branch" ]; then
    git checkout "\$branch" && command wt-go record "\$branch" 2>/dev/null
  fi
}
`.trimStart();

export function initCommand() {
  process.stdout.write(SHELL_FUNCTION);
}
