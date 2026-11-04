package cli_test

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/ui"
)

var (
	ansiRe       = regexp.MustCompile("\x1b\\[[0-9;]*m")
	timingRe     = regexp.MustCompile(`\b\d+(\.\d)?(ms|s)\b`)
	errTransport = errors.New("ssh: connection refused")
)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func getenvOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// runEnv ejecuta hum con un Env a medida (terminal, ancho, variables…).
func runEnv(t *testing.T, fake *herdrtest.Fake, env cli.Env, args ...string) output {
	t.Helper()

	var stdout, stderr bytes.Buffer
	env.Runner, env.Stdout, env.Stderr = fake, &stdout, &stderr
	code := cli.Run(context.Background(), env, args)
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func lines(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }

// findLine devuelve la primera línea que contiene sub.
func findLine(t *testing.T, s, sub string) string {
	t.Helper()

	for _, l := range lines(s) {
		if strings.Contains(l, sub) {
			return l
		}
	}
	t.Fatalf("no line contains %q in:\n%s", sub, s)
	return ""
}

func TestLsRendersTheTreeLikeTheNativePicker(t *testing.T) {
	t.Parallel()

	out := runEnv(t, fixture(), cli.Env{}, "ls")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}

	want := []*regexp.Regexp{
		regexp.MustCompile(`^Local$`),
		regexp.MustCompile(`^ raw$`),
		regexp.MustCompile(`^ ├─ ◆ wS:p4  Pane ID work +claude +working$`), // el pane enfocado usa ◆
		regexp.MustCompile(`^ └─ ○ wS:p1  importer +codex +idle$`),
		// Las máquinas siguen el orden de `herdr machine list`; el estado de cada
		// una va en su encabezado.
		regexp.MustCompile(`^remoto2 +\d+(\.\d)?(ms|s)$`),
		regexp.MustCompile(`^odoous +\d+(\.\d)?(ms|s)$`),
		regexp.MustCompile(`^ odoo-inc$`),
		regexp.MustCompile(`^ └─ ◆ w1:p2  Configura Odoo 18 +codex +idle$`), // enfocado en su máquina
	}
	got := lines(out.stdout)
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), out.stdout)
	}
	for i, re := range want {
		if !re.MatchString(got[i]) {
			t.Errorf("line %d = %q, want it to match %s", i, got[i], re)
		}
	}
}

func TestLsAlignsTheRightColumnsAcrossGroups(t *testing.T) {
	t.Parallel()

	out := runEnv(t, fixture(), cli.Env{}, "ls")
	var widths []int
	for _, l := range lines(out.stdout) {
		if strings.Contains(l, "─ ") {
			widths = append(widths, ui.VisibleLen(l))
		}
	}
	if len(widths) != 3 {
		t.Fatalf("found %d agent rows, want 3:\n%s", len(widths), out.stdout)
	}
	for _, w := range widths {
		if w != widths[0] {
			t.Errorf("rows have different widths %v, want them flush to the same column", widths)
			break
		}
	}
}

func TestLsFallsBackToASCII(t *testing.T) {
	t.Parallel()

	out := runEnv(t, fixture(), cli.Env{Getenv: getenvOf(map[string]string{"HUM_ASCII": "1"})}, "ls")
	for _, r := range out.stdout {
		if r > 127 {
			t.Fatalf("output has a non-ASCII rune %q:\n%s", r, out.stdout)
		}
	}
	if !strings.Contains(out.stdout, "|- * wS:p4") || !strings.Contains(out.stdout, "`- o wS:p1") {
		t.Errorf("output lacks the ASCII tree:\n%s", out.stdout)
	}
}

func TestLsShowsAMachineFailureInItsHeader(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Fail("remoto2", []string{"agent", "list"}, errTransport)
	fake.Hang("odoous", []string{"agent", "list"})

	out := runEnv(t, fake, cli.Env{}, "ls", "--server-timeout", "50ms")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	if l := findLine(t, out.stdout, "odoous"); !regexp.MustCompile(`^odoous +✖ timeout \d+ms$`).MatchString(l) {
		t.Errorf("odoous header = %q, want a timeout status", l)
	}
	if l := findLine(t, out.stdout, "remoto2"); !regexp.MustCompile(`^remoto2 +✖ failed$`).MatchString(l) {
		t.Errorf("remoto2 header = %q, want a failed status", l)
	}
	if !strings.Contains(out.stderr, "remoto2") {
		t.Errorf("stderr = %q, the warning must still be printed", out.stderr)
	}
}

func TestLsColorOnlyAddsEscapeCodes(t *testing.T) {
	t.Parallel()

	plain := runEnv(t, fixture(), cli.Env{}, "ls")
	colored := runEnv(t, fixture(), cli.Env{}, "ls", "--color", "always")
	if !strings.Contains(colored.stdout, "\x1b[") {
		t.Fatalf("--color always printed no escape codes:\n%q", colored.stdout)
	}
	norm := func(s string) string { return timingRe.ReplaceAllString(stripANSI(s), "T") }
	if norm(colored.stdout) != norm(plain.stdout) {
		t.Errorf("colored output differs from plain once stripped:\n%s\nvs\n%s", stripANSI(colored.stdout), plain.stdout)
	}
	if strings.Contains(plain.stdout, "\x1b") {
		t.Errorf("plain output has escape codes: %q", plain.stdout)
	}
}

func TestLsColorDecisionFollowsTheEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		env   cli.Env
		args  []string
		color bool
	}{
		{"terminal colors by default", cli.Env{Terminal: true, GOOS: "windows"}, nil, true},
		{"pipe is plain", cli.Env{GOOS: "windows"}, nil, false},
		{"NO_COLOR", cli.Env{Terminal: true, GOOS: "windows", Getenv: getenvOf(map[string]string{"NO_COLOR": "1"})}, nil, false},
		{"--color never", cli.Env{Terminal: true, GOOS: "windows"}, []string{"--color", "never"}, false},
		{"--color always on a pipe", cli.Env{}, []string{"--color", "always"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := runEnv(t, fixture(), tt.env, append([]string{"ls"}, tt.args...)...)
			if got := strings.Contains(out.stdout, "\x1b["); got != tt.color {
				t.Errorf("color = %v, want %v:\n%q", got, tt.color, out.stdout)
			}
		})
	}
}

func TestLsUsesTheThemeColors(t *testing.T) {
	t.Parallel()

	theme := ui.LoadTheme(strings.NewReader(""))
	out := runEnv(t, fixture(), cli.Env{Terminal: true, GOOS: "windows", Theme: theme}, "ls")
	// working = amarillo de catppuccin (#f9e2af).
	if !strings.Contains(out.stdout, "\x1b[38;2;249;226;175mworking") {
		t.Errorf("working status is not painted with the theme yellow:\n%q", out.stdout)
	}
}

func TestLsFillsTheTerminalWidthAndTruncatesNames(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "list"}, `{"id":"x","result":{"agents":[`+
		`{"agent":"codex","agent_status":"idle","focused":false,"pane_id":"wS:p1","tab_id":"wS:t1","workspace_id":"wS",`+
		`"terminal_title_stripped":"Una tarea con un título muy largo que no cabe en la pantalla"}]}}`)
	fake.Reply("", []string{"workspace", "list"}, localWorkspaces)
	fake.Reply("", []string{"api", "snapshot"}, `{"error":{"code":"unknown_command","message":"no snapshot"},"id":"x"}`)

	noColor := getenvOf(map[string]string{"NO_COLOR": "1"})
	out := runEnv(t, fake, cli.Env{Terminal: true, Width: 50, Getenv: noColor}, "ls", "--local")
	row := findLine(t, out.stdout, "wS:p1")
	if ui.VisibleLen(row) != 50 {
		t.Errorf("row is %d cells wide, want exactly 50: %q", ui.VisibleLen(row), row)
	}
	if !strings.Contains(row, "…") || !strings.HasSuffix(row, "codex  idle") {
		t.Errorf("row = %q, want a truncated name and the right columns intact", row)
	}
}

func TestLsTrimsNoisyCodexTitles(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "list"}, `{"id":"x","result":{"agents":[`+
		`{"agent":"codex","agent_status":"idle","focused":false,"pane_id":"wS:p1","tab_id":"wS:t1","workspace_id":"wS",`+
		`"terminal_title_stripped":"Prepara Excel | mi-proyecto | Prepara Excel | 5h 100% left | weekly 67% left"}]}}`)
	fake.Reply("", []string{"workspace", "list"}, localWorkspaces)
	fake.Reply("", []string{"api", "snapshot"}, `{"error":{"code":"unknown_command","message":"no snapshot"},"id":"x"}`)

	short := runEnv(t, fake, cli.Env{}, "ls", "--local")
	if row := findLine(t, short.stdout, "wS:p1"); !strings.Contains(row, "Prepara Excel ") || strings.Contains(row, "|") {
		t.Errorf("row = %q, want only the first title segment", row)
	}
	full := runEnv(t, fake, cli.Env{}, "ls", "--local", "--full-names")
	if row := findLine(t, full.stdout, "wS:p1"); !strings.Contains(row, "weekly 67% left") {
		t.Errorf("--full-names row = %q, want the whole title", row)
	}
	asJSON := runEnv(t, fake, cli.Env{}, "ls", "--local", "--json")
	if !strings.Contains(asJSON.stdout, "weekly 67% left") {
		t.Errorf("--json must keep the full title:\n%s", asJSON.stdout)
	}
}

func TestLsJSONIgnoresColorFlags(t *testing.T) {
	t.Parallel()

	out := runEnv(t, fixture(), cli.Env{Terminal: true}, "ls", "--json", "--color", "always")
	if strings.Contains(out.stdout, "\x1b") {
		t.Errorf("--json output has escape codes: %q", out.stdout)
	}
}

func TestInvalidColorValueIsAUsageError(t *testing.T) {
	t.Parallel()

	if out := runEnv(t, fixture(), cli.Env{}, "ls", "--color", "sometimes"); out.code != 2 {
		t.Errorf("code = %d, want 2 (stderr %q)", out.code, out.stderr)
	}
}

func TestAttentionStylesStatusesButKeepsTheText(t *testing.T) {
	t.Parallel()

	plain := runEnv(t, attentionFixture(agentsJSON("blocked", "working", "idle")), cli.Env{}, "attention", "--local")
	colored := runEnv(t, attentionFixture(agentsJSON("blocked", "working", "idle")), cli.Env{}, "attention", "--local", "--color", "always")
	if !strings.Contains(colored.stdout, "\x1b[") {
		t.Fatalf("--color always printed no escape codes:\n%q", colored.stdout)
	}
	if stripANSI(colored.stdout) != plain.stdout {
		t.Errorf("stripped colored output differs:\n%s\nvs\n%s", stripANSI(colored.stdout), plain.stdout)
	}
}

func TestMachinesMenuAlignsItsColumns(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{Runner: fake, Stdin: strings.NewReader("q\n"), Stdout: &stdout, Stderr: &stderr},
		[]string{"machines", "menu"})
	if code != 0 {
		t.Fatalf("code = %d, stderr %q", code, stderr.String())
	}
	var cols []int
	for _, l := range lines(stdout.String()) {
		if strings.HasPrefix(l, "[") {
			cols = append(cols, strings.Index(l, "(session:"))
		}
	}
	if len(cols) != 4 {
		t.Fatalf("found %d machine rows, want 4:\n%s", len(cols), stdout.String())
	}
	for _, c := range cols {
		if c != cols[0] || c < 0 {
			t.Errorf("the session column is at %v, want every row aligned:\n%s", cols, stdout.String())
			break
		}
	}
}

func TestMachinesMenuColorsStatesOnATerminal(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdin: strings.NewReader("q\n"), Stdout: &stdout, Stderr: &stderr, Terminal: true, GOOS: "windows",
	}, []string{"machines", "menu"})
	if code != 0 {
		t.Fatalf("code = %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\x1b[38;2;") && !strings.Contains(stdout.String(), "\x1b[3") {
		t.Errorf("menu has no colored state:\n%q", stdout.String())
	}
	if !strings.Contains(stripANSI(stdout.String()), "[1] ENABLED") && !strings.Contains(stripANSI(stdout.String()), "[1] DISABLED") {
		t.Errorf("stripped menu lost its rows:\n%s", stripANSI(stdout.String()))
	}
}
