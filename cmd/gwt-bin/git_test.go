package main

import (
	"strings"
	"testing"
)

const porcelainFixture = `worktree /repo/main
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/main/.worktrees/feature-x
HEAD 89abcdef0123456789abcdef0123456789abcdef
branch refs/heads/feature/x

worktree /repo/detached
HEAD fedcba9876543210fedcba9876543210fedcba98
detached
locked reason line ignored

worktree /repo/gone
HEAD 1111111122222222333333334444444455555555
branch refs/heads/gone
prunable gone

`

const bareFixture = `worktree /repo/bare.git
HEAD 0000000000000000000000000000000000000000
bare

worktree /repo/bare.git/.worktrees/wt1
HEAD aaaaaaaabbbbbbbbccccccccddddddddeeeeeeee
branch refs/heads/wt1

`

func wt(t *testing.T, list []Worktree, i int) Worktree {
	t.Helper()
	if len(list) <= i {
		t.Fatalf("expected worktree %d, got %d entries", i, len(list))
	}
	return list[i]
}

func TestParseWorktrees(t *testing.T) {
	wts := parseWorktrees([]byte(porcelainFixture))
	if len(wts) != 4 {
		t.Fatalf("got %d worktrees: %+v", len(wts), wts)
	}

	main := wt(t, wts, 0)
	if main.Path != "/repo/main" || main.Branch != "refs/heads/main" || main.BranchName() != "main" {
		t.Errorf("main: %+v", main)
	}
	if main.Head != "0123456789abcdef0123456789abcdef01234567" || main.ShortHead() != "0123456" {
		t.Errorf("main head: %+v", main)
	}

	feat := wt(t, wts, 1)
	if feat.BranchName() != "feature/x" {
		t.Errorf("feature branch name: %q", feat.BranchName())
	}

	det := wt(t, wts, 2)
	if !det.Detached || det.BranchName() != "(detached)" || !det.Locked {
		t.Errorf("detached: %+v", det)
	}

	gone := wt(t, wts, 3)
	if !gone.Prunable {
		t.Errorf("prunable: %+v", gone)
	}
}

func TestParseWorktreesBare(t *testing.T) {
	wts := parseWorktrees([]byte(bareFixture))
	if len(wts) != 2 {
		t.Fatalf("got %d", len(wts))
	}
	if !wts[0].Bare || wts[0].BranchName() != "(bare)" {
		t.Errorf("bare: %+v", wts[0])
	}
	if wts[1].BranchName() != "wt1" {
		t.Errorf("linked: %+v", wts[1])
	}
}

func TestParseWorktreesEmpty(t *testing.T) {
	if got := parseWorktrees(nil); len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestBranchName(t *testing.T) {
	for w, want := range map[Worktree]string{
		{Branch: "refs/heads/a"}:     "a",
		{Branch: "refs/heads/a/b/c"}: "a/b/c",
		{Branch: ""}:                 "(detached)",
		{Bare: true}:                 "(bare)",
	} {
		if got := w.BranchName(); got != want {
			t.Errorf("%+v: got %q want %q", w, got, want)
		}
	}
}

func TestGitErrMessage(t *testing.T) {
	err := &gitErr{args: []string{"worktree", "remove", "/x"}, output: "fatal: /x is dirty"}
	if !strings.Contains(err.Error(), "is dirty") {
		t.Errorf("stderr lost: %q", err.Error())
	}
}
