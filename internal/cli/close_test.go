package cli_test

import (
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func closeCalls(f *herdrtest.Fake) int {
	n := 0
	for _, c := range f.Calls() {
		if len(c.Args) > 1 && c.Args[1] == "close" {
			n++
		}
	}
	return n
}

func TestCloseTabWithYesSkipsTheQuestion(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"tab", "close", "wS:t1"}, okReply)

	out := run(t, fake, "close", "--tab", "--yes", "importer")
	if out.code != 0 || !strings.Contains(out.stdout, "closed tab wS:t1") || out.stderr != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestCloseWorkspaceOnARemoteMachine(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("odoous", []string{"workspace", "close", "w1"}, okReply)

	out := run(t, fake, "close", "--workspace", "--yes", "odoous/configura")
	if out.code != 0 || !strings.Contains(out.stdout, "closed workspace w1") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestCloseWithoutATerminalRefusesToGuess(t *testing.T) {
	t.Parallel()

	fake := fixture()
	out := run(t, fake, "close", "--tab", "importer")
	if out.code != 1 || !strings.Contains(out.stderr, "--yes") {
		t.Errorf("code=%d stderr=%q, want a refusal that mentions --yes", out.code, out.stderr)
	}
	if closeCalls(fake) != 0 {
		t.Errorf("something was closed without confirmation: %v", fake.Calls())
	}
}

func TestCloseAsksOnATerminalAndClosesOnlyOnYes(t *testing.T) {
	t.Parallel()

	for _, answer := range []string{"y\n", "Y\n", "yes\n"} {
		fake := fixture()
		fake.Reply("", []string{"tab", "close", "wS:t1"}, okReply)
		env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader(answer)}
		out := runEnv(t, fake, env, "close", "--tab", "importer")
		if out.code != 0 || !strings.Contains(out.stdout, "closed tab wS:t1") {
			t.Errorf("answer %q: code=%d stdout=%q stderr=%q", answer, out.code, out.stdout, out.stderr)
		}
		for _, want := range []string{"Close tab wS:t1", "stop", "[y/N]"} {
			if !strings.Contains(out.stderr, want) {
				t.Errorf("answer %q: the question lacks %q: %q", answer, want, out.stderr)
			}
		}
	}
}

func TestCloseCancelsOnAnythingButYes(t *testing.T) {
	t.Parallel()

	for _, answer := range []string{"\n", "n\n", "no\n", "yy\n", "si\n", ""} {
		fake := fixture()
		env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader(answer)}
		out := runEnv(t, fake, env, "close", "--workspace", "importer")
		if out.code != 1 || out.stdout != "" || !strings.Contains(out.stderr, "cancelled") {
			t.Errorf("answer %q: code=%d stdout=%q stderr=%q, want a cancelled failure", answer, out.code, out.stdout, out.stderr)
		}
		if closeCalls(fake) != 0 {
			t.Errorf("answer %q: something was closed: %v", answer, fake.Calls())
		}
	}
}

func TestCloseNeedsExactlyOneKindAndOneAgent(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"no kind":    {"close", "--yes", "importer"},
		"both kinds": {"close", "--tab", "--workspace", "--yes", "importer"},
		"no agent":   {"close", "--tab", "--yes"},
		"two agents": {"close", "--tab", "--yes", "importer", "codex"},
	} {
		fake := fixture()
		out := run(t, fake, args...)
		if out.code != 2 || !strings.Contains(out.stderr, "usage: hum close") {
			t.Errorf("%s: code=%d stderr=%q, want a usage error", name, out.code, out.stderr)
		}
		if closeCalls(fake) != 0 {
			t.Errorf("%s: something was closed", name)
		}
	}
}

func TestCloseExplainsTheWorktreeGroupRefusal(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"workspace", "close", "wS"},
		`{"error":{"code":"workspace_group_close_required","message":"close the group"},"id":"x"}`)

	out := run(t, fake, "close", "--workspace", "--yes", "importer")
	if out.code != 1 || !strings.Contains(out.stderr, "hint:") || !strings.Contains(out.stderr, "worktree") {
		t.Errorf("code=%d stderr=%q, want a hint about linked worktree workspaces", out.code, out.stderr)
	}
}

func TestCloseShowsHerdrErrors(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"tab", "close", "wS:t1"}, `{"error":{"code":"tab_not_found","message":"no tab"},"id":"x"}`)

	out := run(t, fake, "close", "--tab", "--yes", "importer")
	if out.code != 1 || !strings.Contains(out.stderr, "tab_not_found") || out.stdout != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}
