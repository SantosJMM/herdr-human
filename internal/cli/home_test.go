package cli_test

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func TestHomeNeedsATerminal(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{Runner: herdrtest.New(), Stdout: &stdout, Stderr: &stderr}, []string{"home"})
	if code != 1 || !strings.Contains(stderr.String(), "the home screen needs an interactive terminal") {
		t.Errorf("code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "hint:") || stdout.Len() != 0 {
		t.Errorf("explain how to open it, print nothing on stdout: %q %q", stderr.String(), stdout.String())
	}
}

func TestHomeRejectsUnknownArguments(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"home", "now"}, {"home", "--embedded", "x"}, {"home", "open", "x"}} {
		var stderr bytes.Buffer
		code := cli.Run(context.Background(), cli.Env{Runner: herdrtest.New(), Stdout: io.Discard, Stderr: &stderr}, args)
		if code != 2 || !strings.Contains(stderr.String(), "usage: hum home") {
			t.Errorf("%v: code=%d stderr=%q", args, code, stderr.String())
		}
	}
}

func TestHomeOpenAsksHerdrToOpenItsPopup(t *testing.T) {
	t.Parallel()
	// Es lo que ejecuta el atajo: abre el pane `home` del plugin y enfoca.
	want := []string{"plugin", "pane", "open", "--plugin", "herdr-human", "--entrypoint", "home", "--focus"}
	fake := herdrtest.New()
	fake.Reply("", want, okReply)
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{Runner: fake, Stdout: io.Discard, Stderr: &stderr,
		Getenv: envWith(map[string]string{"HERDR_PLUGIN_ID": "herdr-human"})}, []string{"home", "open"})
	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if calls := fake.Calls(); len(calls) != 1 || !slices.Equal(calls[0].Args, want) {
		t.Errorf("calls = %+v, want %v", calls, want)
	}
}

// startHome lanza `hum home --embedded` con el programa real y un herdr falso.
func startHomeCLI(t *testing.T, fake *herdrtest.Fake) (in io.Writer, scr *screen, done chan int) {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	scr = &screen{}
	done = make(chan int, 1)
	env := cli.Env{
		Runner: fake, Stdin: pr, Stdout: scr, Stderr: io.Discard, Terminal: true, StdinTerminal: true,
		Getenv: envWith(map[string]string{"NO_COLOR": "1"}), GOOS: "windows", Width: 100, Height: 30,
	}
	go func() { done <- cli.Run(context.Background(), env, []string{"home", "--embedded"}) }()
	return pw, scr, done
}

func waitExit(t *testing.T, done chan int) int {
	t.Helper()
	select {
	case code := <-done:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("the home did not exit")
		return -1
	}
}

func TestHomeEndToEndShowsTheTabsAndTheAgentsAndClosesWithEsc(t *testing.T) {
	t.Parallel()
	in, scr, done := startHomeCLI(t, fixture())

	scr.waitFor(t, "[1 Agents]")
	scr.waitFor(t, "2 Attention")
	scr.waitFor(t, "importer") // los agentes salen de herdr, como en `hum pick`
	_, _ = in.Write([]byte("\x1b"))
	if code := waitExit(t, done); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestHomeGoToAgentFocusesItAfterClosing(t *testing.T) {
	t.Parallel()
	fake := fixture()
	fake.Reply("", []string{"agent", "focus", "wS:p1"}, okReply)
	in, scr, done := startHomeCLI(t, fake)

	scr.waitFor(t, "importer")
	// Cada tecla por separado, esperando a que se dibuje: así no se juntan en una.
	_, _ = in.Write([]byte("/"))
	time.Sleep(50 * time.Millisecond)
	_, _ = in.Write([]byte("importer")) // deja un solo agente
	scr.waitFor(t, "1 of ")
	_, _ = in.Write([]byte("\r")) // Enter en la búsqueda abre el menú de acciones del agente
	scr.waitFor(t, "Go to it")
	_, _ = in.Write([]byte("g"))
	if code := waitExit(t, done); code != 0 {
		t.Fatalf("code=%d", code)
	}
	var focused bool
	for _, c := range fake.Calls() {
		focused = focused || slices.Equal(c.Args, []string{"agent", "focus", "wS:p1"})
	}
	if !focused {
		t.Errorf("the chosen agent must be focused; calls: %+v", fake.Calls())
	}
}
