# gwt — Git Worktree Manager TUI

Fast TUI for the `git worktree` lifecycle — list, add, force-remove, prune — plus jump-to-directory via a fish wrapper.

## Install

    go install ./cmd/gwt-bin                    # binary lands on PATH as gwt-bin
    cp gwt.fish ~/.config/fish/functions/       # fish wrapper

## Usage

Run `gwt` inside any git repository.

| Key | Action |
|-----|--------|
| `j`/`k`, `↑`/`↓` | navigate |
| `enter` | cd into selected worktree |
| `a` | add worktree at `<repo-root>/.worktrees/<dir>` — branch input optional (empty = git auto-names) |
| `d` | force-remove selected worktree (y/N) |
| `p` | prune missing/orphaned worktrees (y/N) |
| `r` | reload |
| `q` / `esc` | quit |

The UI renders on stderr; the selected path is printed on stdout for the wrapper to `cd`. `.worktrees/` is auto-added to `.git/info/exclude`.
