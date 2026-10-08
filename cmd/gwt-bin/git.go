package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Worktree struct {
	Path     string
	Branch   string // full ref e.g. refs/heads/main; "" if detached/bare
	Head     string
	Bare     bool
	Detached bool
	Locked   bool
	Prunable bool
}

func (w Worktree) BranchName() string {
	switch {
	case w.Bare:
		return "(bare)"
	case w.Branch == "":
		return "(detached)"
	default:
		return strings.TrimPrefix(w.Branch, "refs/heads/")
	}
}

func (w Worktree) ShortHead() string {
	if len(w.Head) > 7 {
		return w.Head[:7]
	}
	return w.Head
}

// parseWorktrees parses `git worktree list --porcelain` output: records are
// separated by blank lines; each starts with `worktree <path>` followed by
// `HEAD <sha>`, `branch <ref>`, and bare attributes
// (bare, detached, locked [reason], prunable [reason]).
func parseWorktrees(out []byte) []Worktree {
	var wts []Worktree
	for _, rec := range strings.Split(strings.TrimSpace(string(out)), "\n\n") {
		lines := strings.Split(rec, "\n")
		if !strings.HasPrefix(lines[0], "worktree ") {
			continue
		}
		w := Worktree{Path: strings.TrimPrefix(lines[0], "worktree ")}
		for _, a := range lines[1:] {
			switch {
			case strings.HasPrefix(a, "HEAD "):
				w.Head = strings.TrimPrefix(a, "HEAD ")
			case strings.HasPrefix(a, "branch "):
				w.Branch = strings.TrimPrefix(a, "branch ")
			case a == "bare":
				w.Bare = true
			case a == "detached":
				w.Detached = true
			case a == "locked" || strings.HasPrefix(a, "locked "):
				w.Locked = true
			case a == "prunable" || strings.HasPrefix(a, "prunable "):
				w.Prunable = true
			}
		}
		wts = append(wts, w)
	}
	return wts
}

type gitErr struct {
	args   []string
	output string
}

func (e *gitErr) Error() string {
	s := strings.TrimSpace(e.output)
	if s == "" {
		s = "exit error"
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.args, " "), s)
}

func runGit(args ...string) (string, error) {
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", &gitErr{args: args, output: string(out)}
	}
	return strings.TrimSpace(string(out)), nil
}

func listWorktrees() ([]Worktree, error) {
	out, err := runGit("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees([]byte(out)), nil
}

func addWorktree(path, branch string) error {
	args := []string{"worktree", "add", path}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	_, err := runGit(args...)
	return err
}

func removeWorktree(path string) error {
	_, err := runGit("worktree", "remove", path, "--force")
	return err
}

func pruneWorktrees() error {
	_, err := runGit("worktree", "prune")
	return err
}

// ensureExcluded appends ".worktrees/" to <git-common-dir>/info/exclude once
// so worktrees inside the main checkout don't dirty repo status.
func ensureExcluded() error {
	dir, err := runGit("rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dir) // rev-parse may return a relative path
	if err != nil {
		return err
	}
	p := filepath.Join(abs, "info", "exclude")
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(b), ".worktrees/") {
		return nil
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(b) > 0 && b[len(b)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(".worktrees/\n")
	return err
}
