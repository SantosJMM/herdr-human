package cli_test

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

var (
	eraseRe   = regexp.MustCompile(`\x1b\[(\d+)A\r\x1b\[J`)
	elapsedRe = regexp.MustCompile(`\d+(\.\d)?m?s`)
)

// frames separa la salida en los dibujos sucesivos del árbol y devuelve también
// cuántas líneas se pidió subir antes de cada redibujo.
func frames(out string) (parts []string, ups []int) {
	idx := eraseRe.FindAllStringSubmatchIndex(out, -1)
	last := 0
	for _, m := range idx {
		parts = append(parts, out[last:m[0]])
		n, _ := strconv.Atoi(out[m[2]:m[3]])
		ups = append(ups, n)
		last = m[1]
	}
	return append(parts, out[last:]), ups
}

func hangOdoous() *herdrtest.Fake {
	f := fixture()
	f.Hang("odoous", []string{"api", "snapshot"})
	return f
}

func termEnv(height int) cli.Env {
	return cli.Env{Terminal: true, Width: 80, Height: height}
}

func normal(s string) string { return elapsedRe.ReplaceAllString(s, "T") }

func TestLsRedrawsInPlaceWhileMachinesAnswer(t *testing.T) {
	t.Parallel()

	out := runEnv(t, hangOdoous(), termEnv(24), "ls", "--color", "never", "--server-timeout", "700ms")
	if out.code != 0 {
		t.Fatalf("code=%d stderr=%q", out.code, out.stderr)
	}
	parts, ups := frames(out.stdout)
	if len(parts) < 2 {
		t.Fatalf("expected at least two frames, got %d:\n%q", len(parts), out.stdout)
	}
	first, final := parts[0], parts[len(parts)-1]
	if !strings.Contains(first, "importer") || !strings.Contains(first, "connecting") {
		t.Errorf("the first frame should already list the local agents and show the pending machine:\n%s", first)
	}
	if strings.Contains(final, "connecting") || !strings.Contains(final, "✖ timeout") {
		t.Errorf("the final frame should have settled:\n%s", final)
	}
	for i, n := range ups {
		if want := strings.Count(parts[i], "\n"); n != want {
			t.Errorf("redraw %d moved up %d lines but the previous frame had %d", i+1, n, want)
		}
	}
}

func TestTheFinalFrameIsTheSameAsWithoutRedrawing(t *testing.T) {
	t.Parallel()

	progressive := runEnv(t, hangOdoous(), termEnv(24), "ls", "--color", "never", "--server-timeout", "700ms")
	parts, _ := frames(progressive.stdout)
	plain := runEnv(t, hangOdoous(), termEnv(0), "ls", "--color", "never", "--server-timeout", "700ms")
	if normal(parts[len(parts)-1]) != normal(plain.stdout) {
		t.Errorf("final frame differs from the non-progressive output:\n--- progressive\n%s\n--- plain\n%s", parts[len(parts)-1], plain.stdout)
	}
	if strings.Contains(plain.stdout, "\x1b[") {
		t.Errorf("without a known height there must be no cursor sequences: %q", plain.stdout)
	}
}

func TestInstantAnswersAreDrawnOnce(t *testing.T) {
	t.Parallel()

	out := runEnv(t, fixture(), termEnv(24), "ls", "--color", "never")
	if eraseRe.MatchString(out.stdout) {
		t.Errorf("nothing was pending, so there is nothing to redraw: %q", out.stdout)
	}
	if strings.Contains(out.stdout, "connecting") {
		t.Errorf("no machine should be shown as connecting:\n%s", out.stdout)
	}
	plain := runEnv(t, fixture(), termEnv(0), "ls", "--color", "never")
	if normal(out.stdout) != normal(plain.stdout) {
		t.Errorf("output differs:\n%s\n---\n%s", out.stdout, plain.stdout)
	}
}

func TestATreeTallerThanTheTerminalIsPrintedOnceAtTheEnd(t *testing.T) {
	t.Parallel()

	out := runEnv(t, hangOdoous(), termEnv(4), "ls", "--color", "never", "--server-timeout", "700ms")
	if eraseRe.MatchString(out.stdout) || strings.Contains(out.stdout, "connecting") {
		t.Errorf("a frame that does not fit cannot be redrawn, so it must not be drawn at all:\n%q", out.stdout)
	}
	for _, want := range []string{"importer", "✖ timeout"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("the final output lacks %q:\n%s", want, out.stdout)
		}
	}
}

func TestPipesAndJSONNeverGetCursorSequences(t *testing.T) {
	t.Parallel()

	pipe := runEnv(t, hangOdoous(), cli.Env{Height: 24, Width: 80}, "ls", "--color", "never", "--server-timeout", "700ms")
	if strings.Contains(pipe.stdout, "\x1b[") || strings.Contains(pipe.stdout, "connecting") {
		t.Errorf("a pipe must get plain output: %q", pipe.stdout)
	}
	js := runEnv(t, fixture(), termEnv(24), "ls", "--json")
	if strings.Contains(js.stdout, "\x1b[") || !strings.HasPrefix(strings.TrimSpace(js.stdout), "{") {
		t.Errorf("--json must stay JSON even on a terminal: %q", js.stdout)
	}
}

func TestALocalFailureStopsAtOnceAndLeavesNoTreeOnScreen(t *testing.T) {
	t.Parallel()

	f := fixture()
	f.Fail("", []string{"agent", "list"}, errors.New("socket closed"))
	out := runEnv(t, f, termEnv(24), "ls", "--color", "never")
	if out.code != 1 || !strings.Contains(out.stderr, "socket closed") {
		t.Errorf("code=%d stderr=%q, want the local failure", out.code, out.stderr)
	}
	parts, _ := frames(out.stdout)
	if strings.Contains(parts[len(parts)-1], "Local") {
		t.Errorf("a failed listing must not leave a tree on screen:\n%q", out.stdout)
	}
}

func TestFailedMachinesAreStillWarnedAboutAtTheEnd(t *testing.T) {
	t.Parallel()

	out := runEnv(t, hangOdoous(), termEnv(24), "ls", "--color", "never", "--server-timeout", "700ms")
	if !strings.Contains(out.stderr, `warning: machine "odoous"`) {
		t.Errorf("stderr = %q, want the usual warning", out.stderr)
	}
}

func TestCachedMachinesAreNotShownAsConnecting(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	runEnv(t, f, env, "ls") // llena la caché sin terminal

	f.Hang("odoous", []string{"api", "snapshot"}) // si se consultara, bloquearía
	clk.advance(2 * time.Second)
	env.Terminal, env.Width, env.Height = true, 80, 24
	out := runEnv(t, f, env, "ls", "--color", "never", "--server-timeout", "700ms")
	if strings.Contains(out.stdout, "connecting") || !strings.Contains(out.stdout, "cached 2s") {
		t.Errorf("a cached machine should appear at once, marked as cached:\n%s", out.stdout)
	}
}

func TestVerboseStillReportsTimingsWhenRedrawing(t *testing.T) {
	t.Parallel()

	out := runEnv(t, fixture(), termEnv(24), "ls", "--color", "never", "--verbose")
	if !strings.Contains(out.stderr, "timings: local") {
		t.Errorf("stderr = %q, want the timings line", out.stderr)
	}
}
