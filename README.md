# nav

`nav` is a small Go command-line application inspired by [navi](https://github.com/denisidoro/navi).

The goal is to build a **simplified, easy-to-understand cheatsheet tool** in Go that lets you keep useful command-line snippets in local files, search for them interactively, fill in simple variables, and optionally execute the resulting command.

This project will intentionally start much smaller than navi. Features will be added incrementally so the codebase remains easy to understand and provides a good way to learn Go.

## Initial Goals

The first version should focus on:

- Local cheatsheet files
- Simple tags/categories
- Searching and selecting cheats
- Simple `<variable>` substitution
- Displaying the resulting command
- Optional command execution

Later versions can add repository management, configuration, shell integration, and more advanced variable behavior.

## Current State

Phases 1 to 5 are implemented, which covers the whole first milestone. Running
`nav` opens an interactive list that filters as you type. Enter picks a cheat,
nav asks for any `<variable>` values its command needs, shows the completed
command, and runs it if you agree. Declining prints the command instead.

```text
Search: docker

  docker,containers  List running containers in the chosen output format
    docker ps --format "<format>"
> docker,containers  List every container, running or not
    docker ps -a
  docker,images  List local images
    docker images

2 of 5 matched   ↑↓ move   ⏎ select   ^Y copy   esc quit
```

Selecting a command with a `<variable>` in it asks for the value, offering any
predefined ones as a filterable list:

```text
Command:
  docker ps --format "<format>"

format:


> table {{.Names}}\t{{.Status}}
  table {{.Names}}\t{{.Image}}\t{{.Ports}}
  json
  {{.Names}}

variable 1 of 1   1 of 4   ↑↓ move   ⏎ accept   esc cancel
```

With every value supplied, nav shows the finished command and asks:

```text
Command:
  # List running containers in the chosen output format
  docker ps --format "table {{.Names}}\t{{.Status}}"

Run it? [y/N]

y run   n or esc do not run, print the command instead
```

### Usage

```text
nav [flags]

  -h, --help          show this help text and exit
  -V, --version       print the nav version and exit
      --path <dir>    directory to load .cheat files from (default "cheats")
  -q, --query <text>  search terms; pre-fills the interactive search box
      --list          print matching cheats and exit, without the interactive list
      --print         print the completed command instead of offering to run it
  -y, --yes           run the completed command without asking
```

Exit codes: `0` success, `1` error, `2` incorrect usage. Once a command has
run, nav exits with *its* status instead, so a `1` or `2` after execution came
from the command rather than from nav.

### Keys in the Interactive List

| Key | Action |
| --- | --- |
| type | filter the list |
| up/down, `Ctrl-P`/`Ctrl-N` | move the highlight |
| page up/down, home/end | move a screenful, or jump to either end |
| Enter | select the highlighted cheat |
| `Ctrl-Y` | copy the highlighted command to the clipboard, as written |
| `Ctrl-W` / `Ctrl-U` | delete the last word / clear the search box |
| Esc or `Ctrl-C` | quit without selecting |

Printable keys all go into the search box, so there is no single-letter quit
key — `q` is a search character.

Copying shells out to `pbcopy` on macOS, `wl-copy`/`xclip`/`xsel` on Linux and
`clip` on Windows. If none is installed, `Ctrl-Y` says so instead of failing.
`Ctrl-Y` copies the command as written, placeholders included; filling those
in happens after a cheat is selected.

The same keys work in the variable prompts, where Enter accepts the value and
Esc abandons the command.

### Variables

A command may contain `<placeholder>` parts. Once a cheat is selected, nav asks
for each one in the order it appears, previewing the command with the values
chosen so far filled in. A variable used twice is asked for once.

A name starts with a letter or underscore and continues with letters, digits,
underscores or hyphens. Requiring that shape keeps ordinary shell syntax from
being mistaken for a variable — `sort < in > out`, `diff <(a) <(b)` and
`[ $x -lt $y ]` all contain a `<` that is not a placeholder.

Predefined values are optional. A variable that has them gets a filterable
list; one that does not gets a text box. Typing a value the list does not
contain is allowed — predefined values are suggestions, not a restriction.

### Running Commands

The finished command is shown in full — wrapped, never truncated, since it is
what you are agreeing to — and runs only if you answer `y`. Anything else
declines, including Enter, `n` and Esc, because running a command that was not
asked for is the one outcome worth going out of the way to avoid. Declining
still prints the command, so nothing is lost.

Commands run with `$SHELL -c`, falling back to `/bin/sh` when `$SHELL` is
unset, so pipes, redirections, quoting and `&&` behave as they would if you
typed the command yourself. Note that `-c` does not read your shell's start-up
files, so aliases and shell functions are **not** available — a cheat that
depends on one will not work.

The command inherits nav's own standard input and output, so it can prompt,
page and colour its output normally; `git rebase -i` and `docker logs -f` work
as expected. nav then exits with the command's status, and reports 130 if you
interrupt it with `Ctrl-C`. While a command runs, nav ignores `Ctrl-C` itself
so the signal reaches the command and nav survives to report what happened.

A command that fails has already explained itself on its own stderr, so nav
adds nothing and simply passes the status on.

Two flags opt out of the question:

```sh
nav --print -q 'docker images'   # never asks, never runs: just print it
nav --yes   -q 'docker images'   # run it without asking
```

`--print` is the one to use when piping nav's output — `nav --print -q docker |
pbcopy` works because the interactive list draws on `/dev/tty` while stdout
stays free for the command text. The two flags contradict each other and nav
rejects them together.

When nav runs a command itself, the command text is echoed to stderr rather
than stdout, so `nav --yes -q ... > out.txt` captures only the command's own
output.

### Searching

Terms are matched case-insensitively against a cheat's tags, description and
command text. Every term must match, in any order, so `docker ps` finds cheats
mentioning both. A term can be restricted to one field:

```sh
nav -q 'tag:git branch'   # 'branch' may match anywhere, 'git' only in tags
nav -q 'cmd:rev-parse'    # match only against the command text
nav -q 'desc:current'     # match only against the description
```

`--list` applies the same search without opening the interactive list, which
is what you want in scripts. When there is no terminal to draw on — nav's
input is a pipe, say — the interactive list degrades to the same listing
rather than failing.

### Cheatsheet File Format

Cheatsheets are plain-text `*.cheat` files. There are five kinds of line:

```text
; A comment for whoever reads the file. Ignored by nav.
% git, branch

# Show the current branch name
git rev-parse --abbrev-ref HEAD

# Delete a local branch
git branch -d <branch>

$ branch:
    main
    develop
```

- `%` — a comma-separated tag list. Applies to every cheat below it until the
  next `%` line.
- `#` — the description of the command on the next line. Consecutive `#` lines
  are joined with a space.
- `;` — a comment, ignored. Blank lines are ignored too, except that they end a
  block.
- `$ name:` — predefined values for the variable `<name>`, one per indented
  line below it. The block ends at the first line that is not indented.
- Anything else is a command.

Every command needs its own `#` description directly above it, and a `%` line
must come before the first command in a file. Violations are reported as
`file:line: message`; valid cheats in the same file are still loaded.

A `$` block applies to every cheat in its `%` tag section, including ones
written above the block, so several commands can share one variable's values.
Declaring the same name twice in a section adds to its values rather than
replacing them.

Because there is one value per line, values need no escaping — a value may
contain commas, pipes, colons and tabs, which matters because format strings
are exactly what this feature tends to be used for. Indentation is only
meaningful directly below a `$` line; elsewhere it is ignored, so an indented
command still works.

### Project Layout

```text
main.go                      thin wrapper around cli.Main
internal/cli/                flag parsing, help/version, exit codes
internal/cheat/              the Cheat model, the parser, directory loading
internal/search/             filtering cheats by a text query
internal/variables/          finding and substituting <placeholder> parts
internal/runner/             running a command and reporting its exit status
internal/ui/                 the interactive screens and their key decoder
internal/term/               raw terminal mode and window size
internal/clipboard/          copying text to the system clipboard
cheats/                      example cheatsheets
```

`nav` has no third-party dependencies. Raw terminal mode is implemented on
top of the standard library's `syscall` package — the job `golang.org/x/term`
would otherwise do — with per-platform `ioctl` constants behind build tags and
a stub that reports "unsupported" on Windows and Plan 9.

The interactive code is deliberately split in two. The screens in
`internal/ui` — the cheat selector and the variable prompt — only read
keypress bytes from an `io.Reader` and write frames to an `io.Writer`, so
filtering, scrolling, copying, prompting and rendering are all unit-testable
without a terminal. `internal/ui/tty.go` is the thin layer that puts a real
terminal into raw mode and hands it those two interfaces.

Both screens implement the same small `view` interface and share one event
loop and one input reader. Sharing the reader is not just tidiness: a terminal
hands over everything typed so far in a single read, so one read can contain
the Enter that finishes the cheat list along with the first characters meant
for the variable prompt. A reader per screen would discard those.

### Building and Testing

```sh
go build -o nav .
go test ./...
```

## TODO

### Phase 1 — Basic Go CLI

- [x] Create the basic `nav` command
- [x] Add command-line argument parsing
- [x] Add `--help`
- [x] Add `--version`
- [x] Add basic error handling
- [x] Add unit tests
- [x] Establish a simple Go project/package structure

### Phase 2 — Cheatsheet Files

- [x] Define a simple cheatsheet file format
- [x] Load cheatsheets from a local directory
- [x] Support multiple cheatsheets
- [x] Parse cheat titles/tags
- [x] Parse descriptions
- [x] Parse executable command lines
- [x] Ignore comments and blank lines
- [x] Report useful parsing errors

### Phase 3 — Search and Selection

- [x] Search cheats by title/tag
- [x] Search cheats by description
- [x] Search cheats by command text
- [x] Display matching cheats interactively
- [x] Select a cheat from the search results
- [x] Display the selected cheat
- [x] Copy the cheat command into the copy buffer — bound to `Ctrl-Y`, not
      `Cmd-C`. A terminal program cannot see `Cmd-C` on macOS: Terminal.app,
      iTerm2, Ghostty and WezTerm all consume `Cmd` combinations for their own
      copy/paste before they reach the process. `Ctrl-Y` works on every
      platform and leaves `Ctrl-C` meaning "quit", which is what people expect.

### Phase 4 — Variables

- [x] Support variables using `<variable>` syntax
- [x] Detect variables used by a command
- [x] Prompt the user for variable values
- [x] Replace variables in the command
- [x] Support for predefined variable values
- [x] Allow selecting a predefined value
- [x] Handle multiple variables in one command

### Phase 5 — Execute Commands

- [x] Display the completed command before execution
- [x] Ask for confirmation before execution
- [x] Execute the completed command
- [x] Return the command's exit status
- [x] Display command output
- [x] Handle command failures cleanly

### Phase 6 — Configuration

- [ ] Define a default cheatsheet directory
- [ ] Allow the cheatsheet directory to be configured
- [ ] Support an environment variable for the cheatsheet path
- [ ] Add a simple configuration file
- [ ] Add configuration for the preferred search command
- [ ] Add configuration for command execution behavior

### Phase 7 — Cheat Repositories

- [ ] Support cheatsheets stored in Git repositories
- [ ] Add a command to list configured repositories
- [ ] Add a command to add a repository
- [ ] Add a command to remove a repository
- [ ] Clone configured repositories
- [ ] Update repositories
- [ ] Search across all configured repositories

### Phase 8 — Shell Integration

- [ ] Provide a shell-friendly command mode
- [ ] Allow the selected command to be returned to the shell
- [ ] Add Bash integration
- [ ] Add Zsh integration
- [ ] Investigate a Ctrl-R style shell widget
- [ ] Investigate tmux integration

### Phase 9 — Advanced Features

- [ ] Support multiline commands/snippets
- [ ] Support variable dependencies
- [ ] Support aliases
- [ ] Support command previews
- [ ] Support multiple cheatsheet formats
- [ ] Add import/integration with external cheatsheet sources such as tldr or cheat.sh

## Design Principles

- Keep the implementation substantially simpler than navi.
- Prefer the Go standard library when practical.
- Add third-party dependencies only when they provide clear value.
- Keep packages small and focused.
- Write tests as features are added.
- Make the code easy for a Java developer learning Go to understand.
- Build one feature at a time rather than designing the entire application up front.

## Suggested First Milestone

The first useful version of `nav` should be able to do this:

1. Read local `.cheat` files.
2. Search the available cheats.
3. Select a cheat.
4. Show the selected command.
5. Ask the user for simple `<variable>` values.
6. Display the completed command.
7. Ask whether to execute it.
8. Execute it and display the result.

Example:

```text
$ nav

Search cheats: docker ps

> Docker - List containers

Command:
docker ps --format "<format>"

format:
  table {{.Names}}	{{.Status}}
  json

Run command? [y/N] y
```

## Reference

This project is inspired by [navi](https://github.com/denisidoro/navi), an interactive command-line cheatsheet tool. Navi supports local cheatsheets, tags, variables, dynamic variable suggestions, `fzf`/`skim` integration, Git-based cheatsheet repositories, configuration, shell widgets, tmux integration, and other features. This project will implement a deliberately smaller subset first.
