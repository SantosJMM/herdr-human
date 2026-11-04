package cli_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
)

// El prefijo "maquina/" y --machine deben acotar la búsqueda de la misma forma.
func TestMachineFlagScopesResolutionLikeThePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"prefix", []string{"id", "odoous/codex"}, "w1:p2\n"},
		{"flag", []string{"id", "codex", "--machine", "odoous"}, "w1:p2\n"},
		{"flag before the query", []string{"id", "--machine", "odoous", "codex"}, "w1:p2\n"},
		{"flag is case-insensitive", []string{"id", "codex", "--machine", "ODOOUS"}, "w1:p2\n"},
		{"local through the flag", []string{"id", "importer", "--machine", "local"}, "wS:p1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := fixture()
			out := run(t, fake, tt.args...)
			if out.code != 0 || out.stdout != tt.want {
				t.Fatalf("code=%d stdout=%q, want %q (stderr %q)", out.code, out.stdout, tt.want, out.stderr)
			}
			for _, c := range fake.Calls() {
				if c.Machine == "remoto2" {
					t.Errorf("remoto2 was queried: %v", c.Args)
				}
			}
		})
	}
}

func TestAskAcceptsBothWaysToNameTheMachine(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"prefix": {"ask", "odoous/codex", "hello"},
		"flag":   {"ask", "codex", "--machine", "odoous", "hello"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fake := fixture()
			fake.Reply("odoous", []string{"agent", "prompt", "w1:p2", "hello", "--wait"}, askIdleReply)
			fake.Reply("odoous", []string{"agent", "read", "w1:p2", "--lines", "200"}, "> hello\nhi there\n")

			out := run(t, fake, args...)
			if out.code != 0 || out.stdout != "hi there\n" {
				t.Fatalf("code=%d stdout=%q (stderr %q)", out.code, out.stdout, out.stderr)
			}
			for _, c := range fake.Calls() {
				if c.Machine == "remoto2" {
					t.Errorf("remoto2 was queried")
				}
			}
		})
	}
}

func TestMachineFlagThatExcludesEverythingIsNotFound(t *testing.T) {
	t.Parallel()

	// El agente "importer" es local: --machine odoous no debe encontrarlo.
	out := run(t, fixture(), "id", "importer", "--machine", "odoous")
	if out.code != 1 || !strings.Contains(out.stderr, `no agent matches "importer"`) {
		t.Errorf("code=%d stderr=%q, want not found", out.code, out.stderr)
	}
}

func TestChoicePromptSaysHowToCancel(t *testing.T) {
	t.Parallel()

	env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader("\n")}
	out := runEnv(t, fixture(), env, "id", "codex")
	if !strings.Contains(out.stderr, "Which one? [1-2, Enter to cancel]") {
		t.Errorf("stderr = %q, want the prompt to say how to cancel", out.stderr)
	}
}

func TestEnterQuitAndEndOfInputCancel(t *testing.T) {
	t.Parallel()

	for _, answer := range []string{"q\n", "Q\n", "\n", ""} {
		env := cli.Env{Terminal: true, StdinTerminal: true, Stdin: strings.NewReader(answer)}
		out := runEnv(t, fixture(), env, "id", "codex")
		if out.code != 1 || out.stdout != "" || !strings.Contains(out.stderr, "cancelled") {
			t.Errorf("answer %q: code=%d stdout=%q stderr=%q, want a cancelled failure", answer, out.code, out.stdout, out.stderr)
		}
	}
}

// syncBuffer permite leer stderr mientras otra goroutine escribe.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Ctrl+C mientras se espera la elección debe cancelar al instante, sin esperar
// a que la persona pulse Enter.
func TestInterruptCancelsTheChoicePrompt(t *testing.T) {
	t.Parallel()

	stdin, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })

	stderr := &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan int, 1)
	go func() {
		env := cli.Env{Runner: fixture(), Stdin: stdin, Stdout: io.Discard, Stderr: stderr, Terminal: true, StdinTerminal: true}
		done <- cli.Run(ctx, env, []string{"id", "codex"})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(stderr.String(), "Which one?") {
		if time.Now().After(deadline) {
			t.Fatalf("the prompt never appeared: %q", stderr.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	select {
	case code := <-done:
		if code != 1 || !strings.Contains(stderr.String(), "cancelled") {
			t.Errorf("code=%d stderr=%q, want a cancelled failure", code, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Ctrl+C did not cancel the prompt")
	}
}
