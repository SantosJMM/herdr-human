package gitinfo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/gitinfo"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBranchReadsTheCurrentBranch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/feat/ux\n")
	if got := gitinfo.Branch(root); got != "feat/ux" {
		t.Errorf("Branch = %q, want feat/ux", got)
	}
}

func TestBranchFindsTheRepositoryFromASubdirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	sub := filepath.Join(root, "internal", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitinfo.Branch(sub); got != "main" {
		t.Errorf("Branch = %q, want main", got)
	}
}

func TestBranchOfADetachedHeadIsTheShortHash(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "0123456789abcdef0123456789abcdef01234567\n")
	if got := gitinfo.Branch(root); got != "0123456" {
		t.Errorf("Branch = %q, want the short hash", got)
	}
}

func TestBranchFollowsAWorktreePointer(t *testing.T) {
	t.Parallel()

	main := t.TempDir()
	gitdir := filepath.Join(main, ".git", "worktrees", "wt")
	write(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/wt-branch\n")

	tree := t.TempDir()
	write(t, filepath.Join(tree, ".git"), "gitdir: "+gitdir+"\n")
	if got := gitinfo.Branch(tree); got != "wt-branch" {
		t.Errorf("absolute gitdir: Branch = %q, want wt-branch", got)
	}

	rel := t.TempDir()
	write(t, filepath.Join(rel, "meta", "HEAD"), "ref: refs/heads/rel-branch\n")
	write(t, filepath.Join(rel, "tree", ".git"), "gitdir: ../meta\n")
	if got := gitinfo.Branch(filepath.Join(rel, "tree")); got != "rel-branch" {
		t.Errorf("relative gitdir: Branch = %q, want rel-branch", got)
	}
}

func TestBranchIsEmptyWhenThereIsNothingToShow(t *testing.T) {
	t.Parallel()

	empty := t.TempDir()
	broken := t.TempDir()
	write(t, filepath.Join(broken, ".git", "HEAD"), "garbage that is not a ref\n")
	badPointer := t.TempDir()
	write(t, filepath.Join(badPointer, ".git"), "not a pointer\n")

	for name, dir := range map[string]string{
		"no repository": empty, "garbage HEAD": broken, "bad pointer": badPointer,
		"empty path": "", "missing directory": filepath.Join(empty, "nope"),
	} {
		if got := gitinfo.Branch(dir); got != "" {
			t.Errorf("%s: Branch = %q, want empty", name, got)
		}
	}
}

func TestBranchIgnoresAnAbsurdlyLargeHead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/"+strings.Repeat("x", 1<<20))
	if got := gitinfo.Branch(root); len(got) > 256 {
		t.Errorf("Branch returned %d bytes; HEAD must be read with a limit", len(got))
	}
}
