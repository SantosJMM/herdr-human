package cli_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func TestAmbiguousQueryListsTheCandidatesAndHints(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "id", "codex")
	if out.code != 1 {
		t.Fatalf("code = %d, want 1", out.code)
	}
	for _, want := range []string{
		`hum: "codex" matches 2 agents`,
		"local/wS:p1",
		"odoous/w1:p2",
		`hint: use "machine/pane", e.g. "local/wS:p1"`,
	} {
		if !strings.Contains(out.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, out.stderr)
		}
	}
	if out.stdout != "" {
		t.Errorf("stdout = %q, want it empty", out.stdout)
	}
}

func TestNotFoundHintsAtLs(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "id", "nothing")
	if out.code != 1 || !strings.Contains(out.stderr, `hint: run "hum ls"`) {
		t.Errorf("code=%d stderr=%q, want a hint to run hum ls", out.code, out.stderr)
	}
}

func TestNotFoundMentionsUnreachableMachines(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Fail("remoto2", []string{"agent", "list"}, errors.New("ssh: connection refused"))

	out := run(t, fake, "id", "remoto2/dev")
	if out.code != 1 {
		t.Fatalf("code = %d, want 1 (stderr %q)", out.code, out.stderr)
	}
	if !strings.Contains(out.stderr, "hint: could not reach remoto2") {
		t.Errorf("stderr = %q, want a hint naming the unreachable machine", out.stderr)
	}
}

func TestAmbiguityIsAskedNumberedOnAnInteractiveTerminal(t *testing.T) {
	t.Parallel()

	env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader("2\n")}
	out := runEnv(t, fixture(), env, "id", "codex")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	if !strings.Contains(out.stderr, "Which one? [1-2, Enter to cancel]") {
		t.Errorf("stderr = %q, want the numbered question", out.stderr)
	}
	second := regexp.MustCompile(`2\) +(\S+)`).FindStringSubmatch(out.stderr)
	if second == nil {
		t.Fatalf("stderr has no second option:\n%s", out.stderr)
	}
	ids := map[string]string{"local/wS:p1": "wS:p1\n", "odoous/w1:p2": "w1:p2\n"}
	if out.stdout != ids[second[1]] {
		t.Errorf("stdout = %q, want the id of option 2 (%s)", out.stdout, second[1])
	}
}

func TestInvalidChoiceKeepsTheAmbiguityError(t *testing.T) {
	t.Parallel()

	for _, answer := range []string{"x\n", "0\n", "3\n", "1x\n"} {
		env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader(answer)}
		out := runEnv(t, fixture(), env, "id", "codex")
		if out.code != 1 || out.stdout != "" {
			t.Errorf("answer %q: code=%d stdout=%q, want a failure with no output", answer, out.code, out.stdout)
		}
		if !strings.Contains(out.stderr, `hint: use "machine/pane"`) {
			t.Errorf("answer %q: stderr = %q, want the usual hint", answer, out.stderr)
		}
	}
}

func TestAmbiguityIsNeverAskedWithoutAnInteractiveStdin(t *testing.T) {
	t.Parallel()

	// stdout es una terminal pero stdin es una tubería: no se lee la elección de ahí.
	env := cli.Env{Terminal: true, Stdin: strings.NewReader("1\n")}
	out := runEnv(t, fixture(), env, "id", "codex")
	if out.code != 1 || strings.Contains(out.stderr, "Which one?") {
		t.Errorf("code=%d stderr=%q, want a plain failure without a question", out.code, out.stderr)
	}
}

func TestSayToABlockedAgentExplainsWhatToDo(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "hi"},
		`{"error":{"code":"agent_blocked","message":"agent is waiting for approval"},"id":"x"}`)

	out := run(t, fake, "say", "importer", "hi")
	if out.code != 1 {
		t.Fatalf("code = %d, want 1", out.code)
	}
	if !strings.Contains(out.stderr, "hint: the agent is waiting for you; run: hum go local/wS:p1") {
		t.Errorf("stderr = %q, want the blocked hint", out.stderr)
	}
}

func TestAgentGoneHintsAtLs(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "read", "wS:p1"},
		`{"error":{"code":"agent_not_found","message":"agent target wS:p1 not found"},"id":"x"}`)

	out := run(t, fake, "look", "importer")
	if out.code != 1 || !strings.Contains(out.stderr, `hint: the agent may have been closed; run "hum ls"`) {
		t.Errorf("code=%d stderr=%q, want the agent-gone hint", out.code, out.stderr)
	}
}

func TestInvalidNameSuggestsAValidOne(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "name", "importer", "Has Upper")
	if out.code != 1 {
		t.Fatalf("code = %d, want 1", out.code)
	}
	if !strings.Contains(out.stderr, "hint: for example: hum name local/wS:p1 docs-agent") {
		t.Errorf("stderr = %q, want a name suggestion", out.stderr)
	}
}

func TestOtherErrorsHaveNoHint(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	out := run(t, fake, "id", "x")
	if out.code != 1 || strings.Contains(out.stderr, "hint:") {
		t.Errorf("code=%d stderr=%q, an unexplained failure must not invent a hint", out.code, out.stderr)
	}
}
