package cli_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
)

func TestPickNeedsAnInteractiveTerminal(t *testing.T) {
	t.Parallel()

	for name, env := range map[string]cli.Env{
		"pipes":              {},
		"stdout only":        {Terminal: true},
		"stdin only":         {StdinTerminal: true},
		"terminal, no stdin": {Terminal: true, StdinTerminal: true},
	} {
		fake := fixture()
		out := runEnv(t, fake, env, "pick")
		if out.code != 1 || !strings.Contains(out.stderr, "needs an interactive terminal") ||
			!strings.Contains(out.stderr, `hint: use "hum ls"`) {
			t.Errorf("%s: code=%d stderr=%q, want a clear refusal with a hint", name, out.code, out.stderr)
		}
		if len(fake.Calls()) != 0 {
			t.Errorf("%s: herdr was queried before refusing: %v", name, fake.Calls())
		}
	}
}

func TestPickRejectsExtraArguments(t *testing.T) {
	t.Parallel()

	if out := run(t, fixture(), "pick", "something"); out.code != 2 {
		t.Errorf("code = %d, want 2 (stderr %q)", out.code, out.stderr)
	}
}

func TestHelpMentionsPick(t *testing.T) {
	t.Parallel()

	if out := run(t, fixture(), "help"); !strings.Contains(out.stdout, "hum pick") {
		t.Errorf("help lacks hum pick:\n%s", out.stdout)
	}
}

func TestNoArgumentsPrintsHelpWithoutATerminal(t *testing.T) {
	t.Parallel()

	for name, env := range map[string]cli.Env{
		"pipes":       {},
		"stdout only": {Terminal: true},
		"stdin only":  {StdinTerminal: true},
	} {
		fake := fixture()
		out := runEnv(t, fake, env)
		if out.code != 0 || !strings.Contains(out.stdout, "Usage:") || out.stderr != "" {
			t.Errorf("%s: code=%d stdout=%q stderr=%q, want the help on stdout and exit 0", name, out.code, out.stdout, out.stderr)
		}
		if len(fake.Calls()) != 0 {
			t.Errorf("%s: herdr was queried to print the help: %v", name, fake.Calls())
		}
	}
}

func TestNoArgumentsOnATerminalOpensTheSelector(t *testing.T) {
	t.Parallel()

	// El selector real espera teclas: se le da una entrada que nunca escribe y se
	// cancela el contexto cuando ya consultó herdr.
	stdin, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	fake := fixture()
	var stdout bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan int, 1)
	go func() {
		env := cli.Env{Runner: fake, Stdin: stdin, Stdout: &stdout, Stderr: io.Discard, Terminal: true, StdinTerminal: true}
		done <- cli.Run(ctx, env, nil)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for len(fake.Calls()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the selector never queried herdr")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("code = %d, want 0 after cancelling the selector", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the selector did not stop when the context was cancelled")
	}
}
