package cli_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

const (
	newTab     = `{"id":"x","result":{"type":"tab_created","root_pane":{"pane_id":"wS:pA","tab_id":"wS:tA","workspace_id":"wS"},"tab":{"tab_id":"wS:tA","workspace_id":"wS"}}}`
	newStarted = `{"id":"x","result":{"type":"agent_started","agent":{"pane_id":"wS:pA"},"argv":["claude"]}}`
)

// newFake contesta a una creación completa en la máquina dada.
func newFake(machine, dir, label, kind, name string, extra ...string) *herdrtest.Fake {
	f := herdrtest.New()
	tab := []string{"tab", "create"}
	if dir != "" {
		tab = append(tab, "--cwd", dir)
	}
	tab = append(tab, "--label", label, "--focus")
	f.Reply(machine, tab, newTab)
	f.Reply(machine, append([]string{"agent", "start", name, "--kind", kind, "--pane", "wS:pA"}, extra...), newStarted)
	f.Reply(machine, []string{"pane", "rename", "wS:pA", name}, okReply)
	return f
}

func envAt(wd string) cli.Env {
	home := filepath.Join(wd, "home")
	return cli.Env{
		Getwd:  func() (string, error) { return wd, nil },
		Getenv: getenvOf(map[string]string{"HOME": home, "USERPROFILE": home}),
	}
}

func TestNewCreatesTheAgentAndPrintsItsRef(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	f := newFake("", wd, "docs", "claude", "docs")
	out := runEnv(t, f, envAt(wd), "new", "claude", "--name", "docs")
	if out.code != 0 || out.stdout != "local/wS:pA\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
	for _, want := range []string{"tab created (wS:tA)", "claude is ready", `named "docs"`} {
		if !strings.Contains(out.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, out.stderr)
		}
	}
	if strings.Contains(out.stdout, "ready") {
		t.Errorf("progress must stay out of stdout so the ref can be captured: %q", out.stdout)
	}
}

func TestNewWithoutANameUsesTheKindAndDoesNotRenameThePane(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	f := herdrtest.New()
	f.Reply("", []string{"tab", "create", "--cwd", wd, "--label", "codex", "--focus"}, newTab)
	f.Reply("", []string{"agent", "start", "codex", "--kind", "codex", "--pane", "wS:pA"}, newStarted)
	out := runEnv(t, f, envAt(wd), "new", "codex")
	if out.code != 0 || len(f.Calls()) != 2 || strings.Contains(out.stderr, "named") {
		t.Errorf("code=%d calls=%d stderr=%q", out.code, len(f.Calls()), out.stderr)
	}
}

func TestNewResolvesTheFolder(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	abs := t.TempDir()
	home := filepath.Join(wd, "home")
	for name, c := range map[string]struct{ arg, want string }{
		"absolute": {abs, abs},
		"relative": {"sub", filepath.Join(wd, "sub")},
		"tilde":    {"~", home},
		"tilde/":   {"~" + string(filepath.Separator) + "proj", filepath.Join(home, "proj")},
	} {
		f := herdrtest.New()
		f.Reply("", []string{"tab", "create", "--cwd", c.want, "--label", "claude", "--focus"}, newTab)
		f.Reply("", []string{"agent", "start", "claude", "--kind", "claude", "--pane", "wS:pA"}, newStarted)
		if out := runEnv(t, f, envAt(wd), "new", "claude", c.arg); out.code != 0 {
			t.Errorf("%s: code=%d stderr=%q (want --cwd %s)", name, out.code, out.stderr, c.want)
		}
	}
}

func TestNewOnARemoteMachineViaFlagOrPrefixKeepsTheFolderAsGiven(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"prefix": {"new", "odoous/claude", "/opt/proj", "--name", "docs"},
		"flag":   {"new", "claude", "/opt/proj", "--machine", "odoous", "--name", "docs"},
	} {
		f := newFake("odoous", "/opt/proj", "docs", "claude", "docs")
		out := runEnv(t, f, envAt(t.TempDir()), args...)
		if out.code != 0 || out.stdout != "odoous/wS:pA\n" {
			t.Errorf("%s: code=%d stdout=%q stderr=%q", name, out.code, out.stdout, out.stderr)
		}
		for _, c := range f.Calls() {
			if c.Machine != "odoous" {
				t.Errorf("%s: call %v went to %q", name, c.Args, c.Machine)
			}
		}
	}
}

func TestNewOnARemoteMachineWithoutAFolderLeavesItToHerdr(t *testing.T) {
	t.Parallel()

	f := herdrtest.New()
	f.Reply("odoous", []string{"tab", "create", "--label", "claude", "--focus"}, newTab)
	f.Reply("odoous", []string{"agent", "start", "claude", "--kind", "claude", "--pane", "wS:pA"}, newStarted)
	if out := runEnv(t, f, envAt(t.TempDir()), "new", "odoous/claude"); out.code != 0 {
		t.Errorf("code=%d stderr=%q: the local folder must not be sent to a remote machine", out.code, out.stderr)
	}
}

func TestNewPassesNoFocusAndTimeout(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	f := herdrtest.New()
	f.Reply("", []string{"tab", "create", "--cwd", wd, "--label", "claude", "--no-focus"}, newTab)
	f.Reply("", []string{"agent", "start", "claude", "--kind", "claude", "--pane", "wS:pA", "--timeout", "90000"}, newStarted)
	if out := runEnv(t, f, envAt(wd), "new", "claude", "--no-focus", "--timeout", "90000"); out.code != 0 {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestNewSaysWhichStepFailedAndWhatIsLeft(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	f := newFake("odoous", "/opt/proj", "docs", "claude", "docs")
	f.Fail("odoous", []string{"agent", "start", "docs", "--kind", "claude", "--pane", "wS:pA"}, errors.New("agent never became ready"))
	out := runEnv(t, f, envAt(wd), "new", "claude", "/opt/proj", "--machine", "odoous", "--name", "docs")
	if out.code != 1 || out.stdout != "" {
		t.Fatalf("code=%d stdout=%q", out.code, out.stdout)
	}
	for _, want := range []string{
		"the tab wS:tA was created but starting the agent failed",
		"agent never became ready",
		"hint: the tab was left open; close it with: herdr --machine odoous tab close wS:tA",
	} {
		if !strings.Contains(out.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, out.stderr)
		}
	}
	if strings.Contains(out.stderr, "named") {
		t.Errorf("a failed run must not claim the later steps:\n%s", out.stderr)
	}
}

func TestNewSaysHowToNameAPaneThatWasNotRenamed(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	f := newFake("", wd, "docs", "claude", "docs")
	f.Fail("", []string{"pane", "rename", "wS:pA", "docs"}, errors.New("boom"))
	out := runEnv(t, f, envAt(wd), "new", "claude", "--name", "docs")
	if out.code != 1 || !strings.Contains(out.stderr, "hint: the agent is running; name it with: hum name local/wS:pA docs") {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestNewUsageErrorsHappenBeforeAnythingIsCreated(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"no arguments":        {"new"},
		"unknown kind":        {"new", "frobnicator"},
		"invalid name":        {"new", "claude", "--name", "Bad Name"},
		"too many arguments":  {"new", "claude", "/a", "/b"},
		"conflicting machine": {"new", "odoous/claude", "--machine", "remoto2"},
		"bad timeout":         {"new", "claude", "--timeout", "-5"},
	} {
		f := herdrtest.New()
		out := runEnv(t, f, envAt(t.TempDir()), args...)
		if out.code != 2 || out.stderr == "" {
			t.Errorf("%s: code=%d stderr=%q, want a usage error", name, out.code, out.stderr)
		}
		if len(f.Calls()) != 0 {
			t.Errorf("%s: herdr was called: %v", name, f.Calls())
		}
	}
}

func TestNewAsksForWhatIsMissingOnATerminal(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	f := herdrtest.New()
	f.Reply("", []string{"tab", "create", "--cwd", wd, "--label", "mydocs", "--focus"}, newTab)
	f.Reply("", []string{"agent", "start", "mydocs", "--kind", "claude", "--pane", "wS:pA"}, newStarted)
	f.Reply("", []string{"pane", "rename", "wS:pA", "mydocs"}, okReply)

	env := envAt(wd)
	env.Terminal, env.StdinTerminal = true, true
	env.Stdin = strings.NewReader("2\n\nmydocs\n")
	out := runEnv(t, f, env, "new")
	if out.code != 0 || out.stdout != "local/wS:pA\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
	for _, want := range []string{"Kind", "Folder [" + wd + "]", "Name (optional)"} {
		if !strings.Contains(out.stderr, want) {
			t.Errorf("stderr lacks the %q question:\n%s", want, out.stderr)
		}
	}
}

func TestNewAcceptsTheKindByNameAtThePrompt(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	f := herdrtest.New()
	f.Reply("", []string{"tab", "create", "--cwd", wd, "--label", "codex", "--focus"}, newTab)
	f.Reply("", []string{"agent", "start", "codex", "--kind", "codex", "--pane", "wS:pA"}, newStarted)

	env := envAt(wd)
	env.Terminal, env.StdinTerminal = true, true
	env.Stdin = strings.NewReader("codex\n\n\n")
	if out := runEnv(t, f, env, "new"); out.code != 0 {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestNewCancelsWhenTheKindAnswerIsEmptyOrInvalid(t *testing.T) {
	t.Parallel()

	for _, answer := range []string{"\n", "", "99\n", "frobnicator\n"} {
		f := herdrtest.New()
		env := envAt(t.TempDir())
		env.Terminal, env.StdinTerminal = true, true
		env.Stdin = strings.NewReader(answer)
		out := runEnv(t, f, env, "new")
		if out.code == 0 || len(f.Calls()) != 0 {
			t.Errorf("answer %q: code=%d calls=%v, want a refusal without creating anything", answer, out.code, f.Calls())
		}
	}
}

func TestHelpMentionsNew(t *testing.T) {
	t.Parallel()

	if out := run(t, fixture(), "help"); !strings.Contains(out.stdout, "hum new") {
		t.Errorf("help lacks hum new:\n%s", out.stdout)
	}
}
