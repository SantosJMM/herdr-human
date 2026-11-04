package cli_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/complete"
)

func TestCompletionPrintsTheScriptOfEachShell(t *testing.T) {
	t.Parallel()

	for _, shell := range []string{"bash", "zsh", "powershell"} {
		out := run(t, fixture(), "completion", shell)
		if out.code != 0 || !strings.Contains(out.stdout, "__complete --shell "+shell) || out.stderr != "" {
			t.Errorf("%s: code=%d stderr=%q stdout=%q", shell, out.code, out.stderr, out.stdout)
		}
	}
}

func TestCompletionUsageErrors(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"no shell":      {"completion"},
		"unknown shell": {"completion", "fish"},
		"extra":         {"completion", "bash", "zsh"},
	} {
		out := run(t, fixture(), args...)
		if out.code != 2 || out.stderr == "" {
			t.Errorf("%s: code=%d stderr=%q, want a usage error", name, out.code, out.stderr)
		}
	}
	if out := run(t, fixture(), "completion", "fish"); !strings.Contains(out.stderr, "bash") {
		t.Errorf("the error should list the supported shells: %q", out.stderr)
	}
}

func lineOf(line string) []string {
	return []string{"__complete", "--shell", "bash", "--line", line}
}

func TestCompleteSuggestsSubcommandsAndFlagsWithoutAskingHerdr(t *testing.T) {
	t.Parallel()

	f := fixture()
	out := run(t, f, lineOf("hum a")...)
	if out.code != 0 || out.stdout != "ask\nattention\nalias\n" || out.stderr != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
	if out := run(t, f, lineOf("hum ls --fr")...); out.stdout != "--fresh\n" {
		t.Errorf("flags: %q", out.stdout)
	}
	if len(f.Calls()) != 0 {
		t.Errorf("herdr was called for something that needs no data: %v", f.Calls())
	}
}

func TestCompleteWorktreeVerbsAndWhereTheAgentGoes(t *testing.T) {
	t.Parallel()

	if out := run(t, fixture(), lineOf("hum worktree ")...); out.stdout != "ls\nnew\nrm\n" {
		t.Errorf("verbs: %q", out.stdout)
	}
	// "rm" y "ls" llevan el agente en primer lugar; "new" detrás de la rama.
	if out := run(t, fixture(), lineOf("hum worktree rm ")...); !strings.Contains(out.stdout, "importer\n") {
		t.Errorf("rm should offer agents:\n%s", out.stdout)
	}
	if out := run(t, fixture(), lineOf("hum worktree new feat/x ")...); !strings.Contains(out.stdout, "importer\n") {
		t.Errorf("new should offer agents after the branch:\n%s", out.stdout)
	}
	if out := run(t, fixture(), lineOf("hum worktree new ")...); out.stdout != "" {
		t.Errorf("the branch is free text and must not be suggested:\n%s", out.stdout)
	}
}

func TestCompleteOffersLocalAgentsLive(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), lineOf("hum id ")...)
	for _, want := range []string{"importer\n", "local/wS:p1\n", "local/wS:p4\n"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out.stdout)
		}
	}
	if strings.Contains(out.stdout, "Pane ID work") {
		t.Errorf("free-form titles must not be offered:\n%s", out.stdout)
	}
}

func TestCompleteOffersRemoteAgentsFromTheCacheWithoutTouchingTheNetwork(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	runEnv(t, f, env, "ls") // llena la caché
	remote := remoteCalls(f, "odoous")

	f.Hang("odoous", []string{"api", "snapshot"}) // si se consultara, bloquearía
	clk.advance(10 * time.Minute)                 // caducada para usarla, válida para sugerir
	start := time.Now()
	out := runEnv(t, f, env, lineOf("hum say odoous/")...)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("completion took %v: it must not wait for a remote machine", elapsed)
	}
	if remoteCalls(f, "odoous") != remote {
		t.Errorf("a remote machine was queried during completion")
	}
	if !strings.Contains(out.stdout, "odoous/w1:p2\n") {
		t.Errorf("the cached remote agent should be offered:\n%s", out.stdout)
	}
}

func TestCompleteRemoteNamesNeedACache(t *testing.T) {
	t.Parallel()

	env, _ := cachedEnv(t) // directorio vacío: nada guardado
	out := runEnv(t, fixture(), env, lineOf("hum say odoous/")...)
	if out.code != 0 || out.stdout != "" || out.stderr != "" {
		t.Errorf("code=%d stdout=%q stderr=%q, want silence", out.code, out.stdout, out.stderr)
	}
}

func TestCompleteMachineNamesComeFromTheSavedList(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), lineOf("hum ls --machine ")...)
	for _, want := range []string{"local\n", "odoous\n", "remoto2\n"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out.stdout)
		}
	}
}

func TestCompleteIsAlwaysSilentAndSuccessful(t *testing.T) {
	t.Parallel()

	f := fixture()
	f.Fail("", []string{"agent", "list"}, errors.New("socket closed"))
	f.Fail("", []string{"machine", "list"}, errors.New("socket closed"))
	for name, args := range map[string][]string{
		"local failure":  lineOf("hum id "),
		"machines fail":  lineOf("hum ls --machine "),
		"bogus flag":     {"__complete", "--bogus"},
		"no line":        {"__complete"},
		"unknown shell":  {"__complete", "--shell", "fish", "--line", "hum "},
		"unknown subcmd": lineOf("hum frobnicate "),
	} {
		out := run(t, f, args...)
		if out.code != 0 || out.stderr != "" {
			t.Errorf("%s: code=%d stderr=%q, completion must never fail or print errors", name, out.code, out.stderr)
		}
	}
}

func TestCompleteFormatsPerShell(t *testing.T) {
	t.Parallel()

	zsh := run(t, fixture(), "__complete", "--shell", "zsh", "--line", "hum id loc")
	if !strings.Contains(zsh.stdout, `local/wS\:p1:`) {
		t.Errorf("zsh must escape the colon and add a description:\n%s", zsh.stdout)
	}
	ps := run(t, fixture(), "__complete", "--shell", "powershell", "--line", "hum id loc")
	if !strings.Contains(ps.stdout, "local/wS:p1\tcodex idle") {
		t.Errorf("powershell separates the description with a tab:\n%q", ps.stdout)
	}
	files := run(t, fixture(), "__complete", "--shell", "bash", "--line", "hum new claude ")
	if files.stdout != "@files\n" {
		t.Errorf("a folder position asks the shell for files: %q", files.stdout)
	}
}

// Cada opción que el autocompletado sugiere debe existir de verdad en el
// comando; si no, se estaría sugiriendo algo que falla.
func TestEverySuggestedFlagExistsInItsCommand(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"--machine": "odoous", "--server-timeout": "1s", "--color": "never", "--timeout": "1",
		"--lines": "1", "--interval": "1s", "--until": "idle", "--name": "docs", "--base": "main", "--label": "x",
	}
	for _, cmd := range complete.Subcommands() {
		for _, fl := range complete.Flags(cmd) {
			args := []string{cmd, fl.Name}
			if cmd == "worktree" {
				// Cada verbo tiene sus propias opciones: se prueba la opción con el
				// verbo que la define.
				verb := map[string]string{"--base": "new", "--label": "new", "--no-focus": "new", "--force": "rm", "--yes": "rm"}[fl.Name]
				if verb == "" {
					verb = "ls"
				}
				args = []string{cmd, verb, fl.Name}
			}
			if fl.Value {
				v, ok := values[fl.Name]
				if !ok {
					t.Fatalf("the test has no sample value for %s", fl.Name)
				}
				args = append(args, v)
			}
			// Dos argumentos de más para que el comando falle tras leer las opciones
			// y no llegue a ejecutarse (attention --watch no terminaría nunca).
			args = append(args, "zzz1", "zzz2")
			out := run(t, fixture(), args...)
			if strings.Contains(out.stderr, "not defined") {
				t.Errorf("hum %s %s: the completion suggests a flag the command does not have:\n%s", cmd, fl.Name, out.stderr)
			}
		}
	}
}
