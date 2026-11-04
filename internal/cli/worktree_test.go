package cli_test

import (
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

// Forma observada con "herdr worktree list" real (herdr 0.9.x).
const worktreeList = `{"id":"x","result":{"type":"worktree_list","source":{"repo_name":"app"},"worktrees":[` +
	`{"branch":"main","is_bare":false,"is_detached":false,"is_linked_worktree":false,"is_prunable":false,"label":"app","path":"/work/app"},` +
	`{"branch":"feat/x","is_bare":false,"is_detached":false,"is_linked_worktree":true,"is_prunable":false,"label":"x","path":"/work/app-x"}]}}`

func worktreeCalls(f *herdrtest.Fake, verb string) int {
	n := 0
	for _, c := range f.Calls() {
		if len(c.Args) > 1 && c.Args[0] == "worktree" && c.Args[1] == verb {
			n++
		}
	}
	return n
}

func TestWorktreeLsUsesTheCurrentFolderByDefault(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"worktree", "list", "--cwd", "/work/app"}, worktreeList)

	env := cli.Env{Getwd: func() (string, error) { return "/work/app", nil }}
	out := runEnv(t, fake, env, "worktree", "ls")
	if out.code != 0 || out.stderr != "" {
		t.Fatalf("code=%d stderr=%q", out.code, out.stderr)
	}
	for _, want := range []string{"main", "/work/app", "feat/x", "/work/app-x"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out.stdout)
		}
	}
}

func TestWorktreeLsOfAnAgentWorkspaceOnARemoteMachine(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("odoous", []string{"worktree", "list", "--workspace", "w1"}, worktreeList)

	out := run(t, fake, "worktree", "ls", "odoous/configura")
	if out.code != 0 || !strings.Contains(out.stdout, "feat/x") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestWorktreeNewCreatesFromTheWorkspaceOfAnAgent(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"worktree", "create", "--workspace", "wS", "--branch", "feat/y", "--base", "main", "--label", "Why", "--no-focus"},
		`{"id":"x","result":{"path":"/work/app-y"}}`)

	out := run(t, fake, "worktree", "new", "feat/y", "importer", "--base", "main", "--label", "Why", "--no-focus")
	if out.code != 0 || !strings.Contains(out.stdout, "feat/y") || !strings.Contains(out.stdout, "/work/app-y") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestWorktreeNeverTrustsARepositoryOnItsOwn(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"worktree", "create", "--workspace", "wS", "--branch", "feat/y"},
		`{"error":{"code":"repository_untrusted","message":"repository is not trusted"},"id":"x"}`)

	out := run(t, fake, "worktree", "new", "feat/y", "importer")
	if out.code != 1 || !strings.Contains(out.stderr, "hint:") || !strings.Contains(out.stderr, "--trust-repository") {
		t.Errorf("code=%d stderr=%q, want a hint about trusting the repository by hand", out.code, out.stderr)
	}
	for _, c := range fake.Calls() {
		for _, a := range c.Args {
			if a == "--trust-repository" {
				t.Errorf("hum passed --trust-repository: %v", c.Args)
			}
		}
	}
}

func TestWorktreeRemoveAsksAndPassesForce(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"worktree", "remove", "--workspace", "wS", "--force"}, okReply)

	env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader("y\n")}
	out := runEnv(t, fake, env, "worktree", "rm", "importer", "--force")
	if out.code != 0 || !strings.Contains(out.stdout, "removed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
	if !strings.Contains(out.stderr, "[y/N]") {
		t.Errorf("no question was asked: %q", out.stderr)
	}
}

func TestWorktreeRemoveNeedsConfirmation(t *testing.T) {
	t.Parallel()

	fake := fixture()
	if out := run(t, fake, "worktree", "rm", "importer"); out.code != 1 || !strings.Contains(out.stderr, "--yes") {
		t.Errorf("without a terminal: code=%d stderr=%q", out.code, out.stderr)
	}
	env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader("n\n")}
	if out := runEnv(t, fake, env, "worktree", "rm", "importer"); out.code != 1 || !strings.Contains(out.stderr, "cancelled") {
		t.Errorf("answering no: code=%d stderr=%q", out.code, out.stderr)
	}
	if n := worktreeCalls(fake, "remove"); n != 0 {
		t.Errorf("a worktree was removed without confirmation (%d calls)", n)
	}
}

func TestWorktreeRemoveWithYes(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("odoous", []string{"worktree", "remove", "--workspace", "w1"}, okReply)

	out := run(t, fake, "worktree", "rm", "--yes", "odoous/configura")
	if out.code != 0 || !strings.Contains(out.stdout, "removed") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestWorktreeUsageErrors(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"no verb":        {"worktree"},
		"unknown verb":   {"worktree", "frobnicate"},
		"new no branch":  {"worktree", "new"},
		"new dash":       {"worktree", "new", "--", "-x", "importer"},
		"new blank":      {"worktree", "new", "  "},
		"new two whos":   {"worktree", "new", "feat/y", "importer", "codex"},
		"rm no who":      {"worktree", "rm", "--yes"},
		"ls two whos":    {"worktree", "ls", "importer", "codex"},
		"new control ch": {"worktree", "new", "feat\x1b[0m"},
	} {
		fake := fixture()
		out := run(t, fake, args...)
		if out.code != 2 || !strings.Contains(out.stderr, "usage: hum worktree") {
			t.Errorf("%s: code=%d stderr=%q, want a usage error", name, out.code, out.stderr)
		}
		for _, verb := range []string{"create", "remove", "list"} {
			if worktreeCalls(fake, verb) != 0 {
				t.Errorf("%s: herdr was called (%s)", name, verb)
			}
		}
	}
}
