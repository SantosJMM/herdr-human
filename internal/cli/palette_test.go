package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

const (
	busyReply = `{"id":"x","error":{"code":"ui_busy","message":"a popup is open"}}`
	deadReply = `{"id":"x","error":{"code":"not_found","message":"no such action"}}`
)

var (
	paneOpen = []string{"plugin", "pane", "open", "--plugin", "herdr-human", "--entrypoint", "palette", "--focus"}
	invoke   = []string{"plugin", "action", "invoke", "ping", "--plugin", "x.plug"}
	notify   = []string{"notification", "show", "Command palette", "--body"}
)

func envWith(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func runPalette(t *testing.T, fake *herdrtest.Fake, env cli.Env, args ...string) output {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env.Runner, env.Stdout, env.Stderr = fake, &stdout, &stderr
	if env.Sleep == nil {
		env.Sleep = func(context.Context, time.Duration) error { return nil }
	}
	code := cli.Run(context.Background(), env, append([]string{"palette"}, args...))
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestPaletteOpenIsOneHerdrCall(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", paneOpen, okReply)
	out := runPalette(t, fake, cli.Env{}, "open")
	if out.code != 0 {
		t.Fatalf("code=%d stderr=%q", out.code, out.stderr)
	}
	if calls := fake.Calls(); len(calls) != 1 {
		t.Errorf("opening the palette must not query herdr for anything else, got %v", calls)
	}
}

func TestPaletteOpenUsesThePluginIDHerdrGives(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "pane", "open", "--plugin", "custom.id", "--entrypoint", "palette", "--focus"}, okReply)
	out := runPalette(t, fake, cli.Env{Getenv: envWith(map[string]string{"HERDR_PLUGIN_ID": "custom.id"})}, "open")
	if out.code != 0 {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestPaletteOpenReportsHerdrFailures(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", paneOpen, deadReply)
	out := runPalette(t, fake, cli.Env{}, "open")
	if out.code != 1 || !strings.Contains(out.stderr, "no such action") {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestPaletteRunInvokesAnActionAfterAShortWait(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", invoke, okReply)
	var slept []time.Duration
	out := runPalette(t, fake, cli.Env{Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }},
		"run", "--action", "x.plug", "ping")
	if out.code != 0 || len(fake.Calls()) != 1 {
		t.Fatalf("code=%d stderr=%q calls=%v", out.code, out.stderr, fake.Calls())
	}
	if len(slept) != 1 || slept[0] > 100*time.Millisecond {
		t.Errorf("a single short wait so the popup can close, got %v", slept)
	}
}

func TestPaletteRunRetriesWhileThePopupIsStillOpen(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Sequence("", invoke, busyReply, busyReply, okReply)
	out := runPalette(t, fake, cli.Env{}, "run", "--action", "x.plug", "ping")
	if out.code != 0 || len(fake.Calls()) != 3 {
		t.Errorf("ui_busy is retried until it works: code=%d calls=%d stderr=%q", out.code, len(fake.Calls()), out.stderr)
	}
}

func TestPaletteRunGivesUpOnRealErrorsAndNotifies(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", invoke, deadReply)
	fake.Reply("", append(slices.Clone(notify), "x.plug.ping failed: not_found: no such action"), okReply)
	out := runPalette(t, fake, cli.Env{}, "run", "--action", "x.plug", "ping")
	if out.code != 1 {
		t.Errorf("code=%d", out.code)
	}
	calls := fake.Calls()
	if len(calls) != 2 || calls[1].Args[0] != "notification" {
		t.Errorf("a real error is not retried and is shown as a notification, got %v", calls)
	}
}

func TestPaletteRunGivesUpWhenThePopupNeverCloses(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", invoke, busyReply)
	fake.Reply("", append(slices.Clone(notify), "x.plug.ping failed: ui_busy: a popup is open"), okReply)
	out := runPalette(t, fake, cli.Env{}, "run", "--action", "x.plug", "ping")
	if out.code != 1 || len(fake.Calls()) > 50 {
		t.Errorf("retries are bounded: code=%d calls=%d", out.code, len(fake.Calls()))
	}
}

func TestPaletteRunPassesNativeArgumentsVerbatim(t *testing.T) {
	t.Parallel()
	argv := []string{"workspace", "create", "--cwd", "C:/my proj", "--focus"}
	fake := herdrtest.New()
	fake.Reply("", argv, okReply)
	out := runPalette(t, fake, cli.Env{}, append([]string{"run", "--command"}, argv...)...)
	if out.code != 0 || len(fake.Calls()) != 1 {
		t.Errorf("dashes and spaces in the arguments are not reinterpreted: code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestPaletteUsageErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"nope"}, {"open", "extra"}, {"run"}, {"run", "--action", "only-one"}, {"run", "--command"}, {"run", "--wat", "x", "y"},
	} {
		out := runPalette(t, herdrtest.New(), cli.Env{}, args...)
		if out.code != 2 {
			t.Errorf("%v: want usage error (2), got %d stderr=%q", args, out.code, out.stderr)
		}
	}
}

func TestPaletteNeedsATerminal(t *testing.T) {
	t.Parallel()
	out := runPalette(t, herdrtest.New(), cli.Env{})
	if out.code != 1 || !strings.Contains(out.stderr, "interactive terminal") {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

// --- extremo a extremo con una terminal simulada ---

type screen struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *screen) waitFor(t *testing.T, want string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		s.mu.Lock()
		ok := strings.Contains(s.buf.String(), want)
		s.mu.Unlock()
		if ok {
			return
		}
	}
	t.Fatalf("timed out waiting for %q on screen", want)
}

const pluginList = `{"id":"x","result":{"plugins":[` +
	`{"plugin_id":"x.plug","name":"Plug","enabled":true,"actions":[{"id":"ping","title":"Ping the thing","platforms":["windows","linux","macos"]}]},` +
	`{"plugin_id":"herdr-human","name":"Herdr Human","enabled":true,"actions":[{"id":"palette","title":"Open palette"},{"id":"pick","title":"Talk to agents"}]}]}}`

type paletteRun struct {
	in      *io.PipeWriter
	scr     *screen
	done    chan int
	stderr  *bytes.Buffer
	state   string
	detach  chan []string
	started time.Time
}

func startPaletteUI(t *testing.T, fake *herdrtest.Fake, extra map[string]string) *paletteRun {
	return startPaletteWith(t, fake, extra, nil)
}

// startPaletteWith es startPaletteUI con una última oportunidad de ajustar el entorno.
func startPaletteWith(t *testing.T, fake *herdrtest.Fake, extra map[string]string, tune func(*cli.Env)) *paletteRun {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	r := &paletteRun{in: pw, scr: &screen{}, done: make(chan int, 1), stderr: &bytes.Buffer{}, state: t.TempDir(), detach: make(chan []string, 1)}
	vars := map[string]string{
		"NO_COLOR":                  "1",
		"HERDR_PLUGIN_STATE_DIR":    r.state,
		"HERDR_CONFIG_PATH":         filepath.Join(r.state, "config.toml"),
		"HERDR_PLUGIN_CONTEXT_JSON": `{"workspace_id":"w1","tab_id":"w1:t2","focused_pane_id":"w1:p3","focused_pane_cwd":"C:\\proj"}`,
	}
	for k, v := range extra {
		vars[k] = v
	}
	if err := os.WriteFile(vars["HERDR_CONFIG_PATH"], []byte("[[keys.command]]\nkey = \"f2\"\ntype = \"plugin_action\"\ncommand = \"x.plug.ping\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := cli.Env{
		Runner: fake, Stdin: pr, Stdout: r.scr, Stderr: r.stderr, Terminal: true, StdinTerminal: true,
		Getenv: envWith(vars), GOOS: "windows", Width: 100, Height: 30,
		Detach: func(args []string) error { r.detach <- args; return nil },
	}
	if tune != nil {
		tune(&env)
	}
	r.started = time.Now()
	go func() { r.done <- cli.Run(context.Background(), env, []string{"palette"}) }()
	return r
}

func (r *paletteRun) wait(t *testing.T) int {
	t.Helper()
	select {
	case code := <-r.done:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("the palette did not exit")
		return -1
	}
}

func TestPaletteEndToEndPluginAction(t *testing.T) {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	r := startPaletteUI(t, fake, nil)

	// La lista es más larga que la pantalla, así que se filtra primero. Las
	// acciones de plugins llegan después de abrir: el filtro se mantiene.
	_, _ = r.in.Write([]byte("ping"))
	r.scr.waitFor(t, "Ping the thing")
	r.scr.waitFor(t, "f2") // el atajo de config.toml aparece junto a la acción
	_, _ = r.in.Write([]byte("\r"))
	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	if got := <-r.detach; !slices.Equal(got, []string{"palette", "run", "--action", "x.plug", "ping"}) {
		t.Errorf("detached args = %v", got)
	}
	recents, _ := os.ReadFile(filepath.Join(r.state, "recents.json"))
	if !strings.Contains(string(recents), "x.plug.ping") {
		t.Errorf("the choice is remembered: %q", recents)
	}
	cache, err := os.ReadFile(filepath.Join(r.state, "plugins-cache.json"))
	if err != nil {
		t.Fatalf("the plugin actions are cached for the next opening: %v", err)
	}
	if strings.Contains(string(cache), "herdr-human.palette") {
		t.Error("the palette must not list its own open action")
	}
	if !strings.Contains(string(cache), "herdr-human.pick") {
		t.Error("the other actions of this same plugin are offered")
	}
}

func TestPaletteEndToEndNativeCommandUsesTheOriginContext(t *testing.T) {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	r := startPaletteUI(t, fake, nil)

	_, _ = r.in.Write([]byte("tab: close"))
	r.scr.waitFor(t, "Tab: Close current")
	_, _ = r.in.Write([]byte("\r"))
	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	if got := <-r.detach; !slices.Equal(got, []string{"palette", "run", "--command", "tab", "close", "w1:t2"}) {
		t.Errorf("the tab id comes from the popup's context, got %v", got)
	}
	for _, c := range fake.Calls() {
		if c.Args[0] == "pane" || c.Args[0] == "workspace" || c.Args[0] == "tab" {
			t.Errorf("a command with only static context must not query herdr, got %v", c.Args)
		}
	}
}

func TestPaletteEndToEndOpensEvenIfPluginsAreSlowOrDown(t *testing.T) {
	fake := herdrtest.New()
	fake.Fail("", []string{"plugin", "list", "--json"}, errors.New("herdr down"))
	r := startPaletteUI(t, fake, nil)

	_, _ = r.in.Write([]byte("tab: close"))
	r.scr.waitFor(t, "Tab: Close current") // lo embebido se dibuja igual
	r.scr.waitFor(t, "plugins unavailable")
	_, _ = r.in.Write([]byte("\x1b"))
	if code := r.wait(t); code != 0 {
		t.Errorf("closing without choosing is not an error: code=%d", code)
	}
	select {
	case args := <-r.detach:
		t.Errorf("nothing was chosen, nothing should run: %v", args)
	default:
	}
}

// El CLI real de herdr imprime el error ui_busy como JSON por stderr y sale con
// código 1 (no hay sobre en stdout). Este es el error tal como llega de verdad.
func realUIBusy() error {
	return &herdr.ExitError{
		Args:   paneOpen,
		Stderr: `{"error":{"code":"ui_busy","message":"a popup pane is already open"},"id":"cli:plugin"}`,
		Err:    errors.New("exit status 1"),
	}
}

func TestPaletteOpenRetriesWhileThePreviousPopupIsClosing(t *testing.T) {
	t.Parallel()
	fake := &flakyRunner{failures: 3, err: realUIBusy()}
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdout: &stdout, Stderr: &stderr,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}, []string{"palette", "open"})
	if code != 0 || fake.calls != 4 {
		t.Errorf("a popup that is still closing is retried: code=%d calls=%d stderr=%q", code, fake.calls, stderr.String())
	}
}

func TestPaletteOpenStopsInsistingWhenAnotherPopupStaysOpen(t *testing.T) {
	t.Parallel()
	fake := &flakyRunner{failures: 1000, err: realUIBusy()}
	var stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdout: io.Discard, Stderr: &stderr,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}, []string{"palette", "open"})
	if code != 1 || fake.calls > 15 || !strings.Contains(stderr.String(), "ui_busy") {
		t.Errorf("a bounded number of attempts, then the reason: code=%d calls=%d stderr=%q", code, fake.calls, stderr.String())
	}
}

func TestPaletteRunRetriesOnTheRealStderrFormat(t *testing.T) {
	t.Parallel()
	fake := &flakyRunner{failures: 2, err: realUIBusy()}
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdout: io.Discard, Stderr: io.Discard,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}, []string{"palette", "run", "--action", "x.plug", "ping"})
	if code != 0 || fake.calls != 3 {
		t.Errorf("ui_busy on stderr is retried: code=%d calls=%d", code, fake.calls)
	}
}

func TestPaletteOpenDoesNotRetryOtherFailures(t *testing.T) {
	t.Parallel()
	fake := &flakyRunner{failures: 5, err: &herdr.ExitError{Stderr: `{"error":{"code":"not_found"}}`, Err: errors.New("exit status 1")}}
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdout: io.Discard, Stderr: io.Discard,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}, []string{"palette", "open"})
	if code != 1 || fake.calls != 1 {
		t.Errorf("only ui_busy is worth retrying: code=%d calls=%d", code, fake.calls)
	}
}

// flakyRunner falla con err las primeras failures veces y luego responde bien.
type flakyRunner struct {
	failures int
	err      error
	calls    int
}

func (f *flakyRunner) Run(context.Context, string, ...string) ([]byte, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, f.err
	}
	return []byte(okReply), nil
}
