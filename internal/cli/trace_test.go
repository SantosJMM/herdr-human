package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/trace"
)

func runTrace(t *testing.T, cacheDir string, getenv func(string) string, args ...string) output {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Runner: herdrtest.New(), Stdout: &stdout, Stderr: &stderr, CacheDir: cacheDir, Getenv: getenv,
	}, append([]string{"trace"}, args...))
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestTraceOnOffShowClear(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "cache")
	none := envWith(nil)

	if out := runTrace(t, dir, none, "show"); out.code != 0 || !strings.Contains(out.stdout, "trace off") || !strings.Contains(out.stdout, "no log yet") {
		t.Errorf("before anything: %+v", out)
	}
	if out := runTrace(t, dir, none, "on"); out.code != 0 || !strings.Contains(out.stdout, "trace.log") {
		t.Fatalf("on: %+v", out)
	}
	if !trace.Enabled(none, dir) {
		t.Error("`hum trace on` must switch the trace on for other processes (popups) through the marker file")
	}

	log := filepath.Join(dir, "trace.log")
	if err := os.WriteFile(log, []byte("line one\nline two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := runTrace(t, dir, none, "show"); !strings.Contains(out.stdout, "trace on") || !strings.Contains(out.stdout, "line two") {
		t.Errorf("show: %+v", out)
	}
	if out := runTrace(t, dir, none, "off"); out.code != 0 || trace.Enabled(none, dir) {
		t.Errorf("off: %+v", out)
	}
	if _, err := os.Stat(log); err != nil {
		t.Error("turning the trace off keeps the log")
	}
	if out := runTrace(t, dir, none, "clear"); out.code != 0 {
		t.Errorf("clear: %+v", out)
	} else if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Error("clear deletes the log")
	}
	if out := runTrace(t, dir, none, "clear"); out.code != 0 {
		t.Errorf("clearing twice is fine: %+v", out)
	}
}

func TestTraceUsageAndMissingCacheFolder(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{}, {"nope"}, {"on", "extra"}} {
		if out := runTrace(t, t.TempDir(), envWith(nil), args...); out.code != 2 {
			t.Errorf("%v: want usage error (2), got %+v", args, out)
		}
	}
	if out := runTrace(t, "", envWith(nil), "on"); out.code != 1 || !strings.Contains(out.stderr, "no cache folder") {
		t.Errorf("without a cache folder there is nowhere to write: %+v", out)
	}
}

func TestPaletteOpenHandsTheTriggerTimeToThePopupOnlyWhenTracing(t *testing.T) {
	t.Parallel()
	start := time.UnixMilli(1793873711000)
	var log bytes.Buffer
	fake := herdrtest.New()
	fake.Reply("", append(paneOpen, "--env", "HUM_TRACE_T0=1793873711000"), okReply)
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdout: io.Discard, Stderr: io.Discard, Trace: trace.New(&log, "palette open", start),
	}, []string{"palette", "open"})
	if code != 0 {
		t.Fatalf("with the trace on the pane is opened with the trigger time: code=%d calls=%v", code, fake.Calls())
	}
	for _, want := range []string{"open: asking herdr to open the pane", "open: herdr answered"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("the trace lacks %q:\n%s", want, log.String())
		}
	}
}

func TestPaletteTraceRecordsTheFirstFrameAndTheTimeSinceTheTrigger(t *testing.T) {
	var log syncBuffer
	trigger := time.Now().Add(-120 * time.Millisecond)
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	r := startPaletteWith(t, fake, map[string]string{"HUM_TRACE_T0": trace.FormatTrigger(trigger)}, func(env *cli.Env) {
		env.Trace = trace.New(&log, "palette", time.Now())
	})

	_, _ = r.in.Write([]byte("ping"))
	r.scr.waitFor(t, "Ping the thing")
	_, _ = r.in.Write([]byte("\x1b"))
	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d", code)
	}

	text := log.String()
	for _, want := range []string{
		"palette: catalog, cache and settings ready",
		"palette: herdr answered with the plugin list",
		"tui: first view rendered",
		"palette: first frame written to the terminal (",
		"ms since the trigger)",
		"tui: plugin list merged into the view",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the trace lacks %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "first frame written") != 1 || strings.Count(text, "first view rendered") != 1 {
		t.Errorf("each milestone is recorded once, however many frames are drawn:\n%s", text)
	}
}

func TestPaletteWithoutATraceWritesNothingExtra(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	r := startPaletteUI(t, fake, map[string]string{"HUM_TRACE_T0": "1793873711000"})
	r.scr.waitFor(t, "Command palette")
	_, _ = r.in.Write([]byte("\x1b"))
	if code := r.wait(t); code != 0 {
		t.Errorf("the trigger variable alone changes nothing: code=%d", code)
	}
}
