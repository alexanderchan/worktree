const SHELL_FUNCTION = `
# wt shell integration — enables "wt go" to cd in the current shell
wt() {
  if [ "$1" = "go" ]; then
    shift
    local dir
    dir=$(command wt go "$@")
    if [ $? -eq 0 ] && [ -n "$dir" ]; then
      cd "$dir"
    fi
  else
    command wt "$@"
  fi
}
`.trimStart();

export function initCommand() {
  process.stdout.write(SHELL_FUNCTION);
}
