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
- Interactive selection using `fzf`
- Displaying the resulting command
- Optional command execution

Later versions can add repository management, configuration, shell integration, and more advanced variable behavior.

## Current State

Phases 1 and 2 are implemented: `nav` parses flags, loads `.cheat` files from a
directory, and can list what it found. Interactive search (Phase 3) is next.

```text
$ nav --list
git,branch: Show the current branch name
    git rev-parse --abbrev-ref HEAD
...
```

### Usage

```text
nav [flags]

  -h, --help          show this help text and exit
  -V, --version       print the nav version and exit
      --path <dir>    directory to load .cheat files from (default "cheats")
      --list          print every cheat that was loaded
```

Exit codes: `0` success, `1` error, `2` incorrect usage.

### Cheatsheet File Format

Cheatsheets are plain-text `*.cheat` files. There are four kinds of line:

```text
; A comment for whoever reads the file. Ignored by nav.
% git, branch

# Show the current branch name
git rev-parse --abbrev-ref HEAD

# Delete a local branch
git branch -d <branch>
```

- `%` — a comma-separated tag list. Applies to every cheat below it until the
  next `%` line.
- `#` — the description of the command on the next line. Consecutive `#` lines
  are joined with a space.
- `;` — a comment, ignored. Blank lines are ignored too, except that they end a
  block.
- Anything else is a command.

Every command needs its own `#` description directly above it, and a `%` line
must come before the first command in a file. Violations are reported as
`file:line: message`; valid cheats in the same file are still loaded.

`<variable>` placeholders are recognised as part of the command text for now —
prompting for their values is Phase 4.

### Project Layout

```text
main.go                      thin wrapper around cli.Main
internal/cli/                flag parsing, help/version, exit codes
internal/cheat/              the Cheat model, the parser, directory loading
cheats/                      example cheatsheets
```

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

- [ ] Search cheats by title/tag
- [ ] Search cheats by description
- [ ] Search cheats by command text
- [ ] Display matching cheats interactively
- [ ] Select a cheat from the search results
- [ ] Display the selected cheat
- [ ] Make it so Command-c (or Control-c on windows) copies the command into copy buffer

### Phase 4 — Variables

- [ ] Support variables using `<variable>` syntax
- [ ] Detect variables used by a command
- [ ] Prompt the user for variable values
- [ ] Replace variables in the command
- [ ] Support predefined variable values
- [ ] Allow selecting a predefined value with `fzf`
- [ ] Handle multiple variables in one command

### Phase 5 — Execute Commands

- [ ] Display the completed command before execution
- [ ] Ask for confirmation before execution
- [ ] Execute the completed command
- [ ] Return the command's exit status
- [ ] Display command output
- [ ] Handle command failures cleanly

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
- [ ] Support richer `fzf` configuration
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
3. Select a cheat with `fzf`.
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
