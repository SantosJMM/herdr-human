package cli_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func openPaneArgs(entrypoint string) []string {
	return []string{"plugin", "pane", "open", "--plugin", "herdr-human", "--entrypoint", entrypoint, "--focus"}
}

func TestPopupOpensTheSamePaneAsItsName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"overview", "machines", "pick", "attention"} {
		fake := herdrtest.New()
		fake.Reply("", openPaneArgs(name), okReply)
		var stderr bytes.Buffer
		code := cli.Run(context.Background(), cli.Env{Runner: fake, Stdout: io.Discard, Stderr: &stderr}, []string{"popup", name})
		if code != 0 || len(fake.Calls()) != 1 {
			t.Errorf("%s: code=%d calls=%v stderr=%q", name, code, fake.Calls(), stderr.String())
		}
	}
}

func TestPopupRetriesWhileThePreviousPopupIsClosing(t *testing.T) {
	t.Parallel()
	fake := &flakyRunner{failures: 2, err: realUIBusy()}
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdout: io.Discard, Stderr: io.Discard,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}, []string{"popup", "pick"})
	if code != 0 || fake.calls != 3 {
		t.Errorf("code=%d calls=%d", code, fake.calls)
	}
}

func TestPopupUsageErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"popup"}, {"popup", "nope"}, {"popup", "pick", "extra"}, {"popup", "show"}, {"popup", "show", "say"}, {"popup", "show", "ls", "x"},
	} {
		var stderr bytes.Buffer
		code := cli.Run(context.Background(), cli.Env{Runner: herdrtest.New(), Stdout: io.Discard, Stderr: &stderr}, args)
		if code != 2 {
			t.Errorf("%v: want usage error (2), got %d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestPopupShowWithoutATerminalJustPrintsAndExits(t *testing.T) {
	t.Parallel()
	out := run(t, fixture(), "popup", "show", "ls", "--local")
	// "--local" no es un argumento válido de popup show: es un error de uso.
	if out.code != 2 {
		t.Fatalf("extra arguments are refused: %+v", out)
	}
	out = run(t, fixture(), "popup", "show", "ls")
	if out.code != 0 || !strings.Contains(out.stdout, "Pane ID work") || strings.Contains(out.stdout, "Press any key") {
		t.Errorf("without a terminal there is nobody to wait for: %+v", out)
	}
}

func TestPopupShowWaitsForAKeyOnATerminal(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	scr := &screen{}
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(context.Background(), cli.Env{
			Runner: fixture(), Stdin: pr, Stdout: scr, Stderr: io.Discard, Terminal: true, StdinTerminal: true,
			Getenv: envWith(map[string]string{"NO_COLOR": "1"}), Width: 100, Height: 30,
		}, []string{"popup", "show", "ls"})
	}()

	scr.waitFor(t, "Pane ID work")
	scr.waitFor(t, "Press any key to close")
	select {
	case code := <-done:
		t.Fatalf("the popup closed before a key was pressed (code %d): the output could not be read", code)
	case <-time.After(150 * time.Millisecond):
	}
	_, _ = pw.Write([]byte("x"))
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("code=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a key press must close the popup")
	}
}
