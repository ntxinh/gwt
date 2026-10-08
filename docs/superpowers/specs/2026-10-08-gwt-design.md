# gwt — Git Worktree Manager TUI — Design

## 1. Overview & Scope

A fast, focused Terminal User Interface for managing Git worktrees. Eliminates the need to remember verbose `git worktree` commands and enables seamless directory switching via shell integration.

**Out of scope:** general Git operations (commits, rebasing, diff viewing). Strictly worktree lifecycle management.

## 2. Environment & Tech Stack

- **Language:** Go 1.21+ (installed toolchain: 1.27.1)
- **TUI:** `charmbracelet/bubbletea` (model), `charmbracelet/bubbles` (list, textinput), `charmbracelet/lipgloss` (styling)
- **Target:** Fedora Linux (Wayland), Fish shell

## 3. Repository Layout

```
gwt/
├── cmd/gwt-bin/
│   ├── main.go      — model, states, key handling, Update/View
│   ├── git.go       — `git worktree list --porcelain` parsing, add/remove/prune wrappers
│   └── git_test.go  — porcelain parser tests (the only real logic)
├── gwt.fish         — canonical source of the fish function; copied to ~/.config/fish/functions/
├── go.mod           — module gwt, package main in cmd/gwt-bin → `go install ./cmd/gwt-bin` yields binary `gwt-bin`
└── README.md        — install + usage
```

Binary is deliberately `gwt-bin` (not `gwt`) so the fish function named `gwt` cannot recurse into itself.

## 4. I/O Architecture

A child process cannot change the parent shell's cwd, so:

- **UI rendering:** Bubble Tea program writes to `os.Stderr` (`tea.NewProgram(model, tea.WithOutput(os.Stderr))`).
- **Path output:** on "Jump", the absolute worktree path goes to `os.Stdout` followed by `\n`, exit 0.
- **Quit/cancel:** exit 0, nothing on stdout → fish wrapper does nothing.
- **Startup failure** (not inside a git repo, `git` missing): error text to stderr, exit 1, empty stdout.

## 5. Fish Shell Integration

`gwt.fish` (verbatim spec, installed to `~/.config/fish/functions/gwt.fish`):

```fish
function gwt --description "Git Worktree TUI Manager"
    set target_dir (gwt-bin)
    if test -n "$target_dir"
        if test -d "$target_dir"
            cd "$target_dir"
        else
            echo "Error: Directory does not exist -> $target_dir"
        end
    end
end
```

## 6. State Machine

`mode` field: `list | add | confirmDelete | confirmPrune`. Single model; mode selects the active view and key handling. A `status` string renders a one-line message bar at the bottom (used for git errors).

### State: List (default)

- Data: parse `git worktree list --porcelain` into `{path, branch, head, bare, detached, locked, prunable}`.
- View: rows of `path — branch — short-HEAD`. Branch cell shows `(detached)` when HEAD is detached, `(bare)` for bare repos; locked worktrees get a `*` suffix on the path. Order is git's order (main worktree first).
- Keys:
  - `j`/`k`, `up`/`down` — navigate
  - `enter` — jump: write selected path to stdout, quit
  - `a` — → Add
  - `d` — → DeleteConfirm (for the selected worktree)
  - `p` — → PruneConfirm
  - `q` / `esc` — quit, no output
  - `r` — reload list *(tiny, included: after external git ops while gwt stays open)*

### State: Add

- View: two text inputs — 1) directory name, 2) branch name.
- Focus: `tab` / `shift+tab` / `up` / `down` cycle the two fields; `esc` cancels to List.
- `enter` (from either field): directory name required — empty dir does nothing (status hint). Resolve path as `<main-worktree-root>/.worktrees/<name>` (root = first `worktree` record in the porcelain output — always an absolute path, so gwt's cwd is irrelevant).
  - Non-empty branch → `git worktree add <path> -b <branch>`
  - Empty branch → `git worktree add <path>` (git auto-names the branch from the basename)
  - Also ensure `.worktrees/` appears in `<git-common-dir>/info/exclude` so worktrees don't dirty repo status (append once if absent; common dir = `git rev-parse --git-common-dir` or the `gitdir:` of the main worktree record).
  - On failure → status line shows git's stderr, stay in Add state so fields aren't lost.
  - On success → reload, → List.

### State: DeleteConfirm

- View: `Force remove worktree <branch-or-name>? (y/N)` — shows the branch name, or the path's basename for detached worktrees.
- `y` → `git worktree remove <path> --force`; reload; → List.
- `n` / `esc` → → List.
- No preemptive guards: git already refuses to remove the main worktree or a dirty/absent path; its stderr lands in the status line. `--force` is what the spec's prompt advertises.

### State: PruneConfirm

- View: `Prune missing/orphaned worktrees? (y/N)`
- `y` → `git worktree prune`; reload; → List.
- `n` / `esc` → → List.

## 7. Error Handling

All `git worktree` invocations return `cmd.CombinedOutput` on failure; the trimmed output is shown in the status line. The program never panics on user/git errors. Startup failure (see §4) is the only fatal path.

## 8. Testing

- `git_test.go`: table-driven parser tests — normal worktrees, bare repo record, `detached`, `locked`, `locked reason`, `prunable`, empty output. No tests for model glue or rendering.
- Manual smoke: `go run ./cmd/gwt-bin` inside a repo — navigate, add, delete, prune, jump — plus fish wrapper `cd` verification.

## 9. Dependencies

`bubbletea`, `bubbles`, `lipgloss` — already-decided pins via `go mod tidy`.
