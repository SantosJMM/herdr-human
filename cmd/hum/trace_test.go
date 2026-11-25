package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Ejecuta el main real en otro proceso: incluye newTracer, cli.Run y el cierre
// del archivo antes de os.Exit. Todas las rutas apuntan a datos sintéticos.
func TestHumTraceEntryPoint(t *testing.T) {
	if os.Getenv("HUM_TEST_ENTRYPOINT") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"hum"}, os.Args[i+1:]...)
			main()
		}
	}
	t.Fatal("missing argument separator")
}

func TestTraceModesThroughMain(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"LOCALAPPDATA", "APPDATA", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "HERDR_PLUGIN_CONFIG_DIR"} {
		t.Setenv(key, root)
	}
	t.Setenv("HUM_TRACE", "")
	t.Setenv("HUM_TEST_ENTRYPOINT", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := func(wantCode int, args ...string) string {
		t.Helper()
		cmd := exec.Command(exe, append([]string{"-test.run=^TestHumTraceEntryPoint$", "--"}, args...)...)
		out, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if code != wantCode {
			t.Fatalf("%v: exit=%d want=%d: %s", args, code, wantCode, out)
		}
		return string(out)
	}
	// --timeout without --wait fails before looking up or messaging any agent.
	args := []string{"say", "synthetic-agent", "SYNTHETIC-SECRET two words", "--timeout", "1"}
	log := filepath.Join(cacheDir(), "trace.log")
	check := func(content bool) {
		t.Helper()
		run(2, args...)
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if strings.Contains(text, "SYNTHETIC-SECRET") != content || strings.Contains(text, "synthetic-agent") != content {
			t.Fatalf("content=%v: unexpected trace: %s", content, text)
		}
		for _, mark := range []string{"main", "environment ready", "exit"} {
			if !regexp.MustCompile(`pid=\d+ say[^\n]* \+\d+\.\dms ` + mark + `\n`).MatchString(text) {
				t.Errorf("missing timing mark %q: %s", mark, text)
			}
		}
		if err := os.Remove(log); err != nil { // Solo el log sintético de esta prueba.
			t.Fatal(err)
		}
	}
	run(0, "trace", "on")
	check(false)
	if out := run(0, "trace", "on", "--content"); !strings.Contains(out, "WARNING") {
		t.Fatalf("missing content warning: %s", out)
	}
	check(true)
	run(0, "trace", "on") // Una activación normal revoca el consentimiento anterior.
	check(false)
	run(0, "trace", "off")
	run(2, args...)
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("trace off wrote a log: %v", err)
	}
	// La variable antigua también debe ser segura, incluso con ruta propia.
	for _, value := range []string{"1", filepath.Join(root, "env-trace.log")} {
		t.Setenv("HUM_TRACE", value)
		log = filepath.Join(cacheDir(), "trace.log")
		if value != "1" {
			log = value
		}
		check(false)
	}
}

func TestTraceLabelNeverCopiesFreeArgumentsByDefault(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "hum"},
		{[]string{"SYNTHETIC-SECRET"}, "hum"},
		{[]string{"ask", "private-agent", "private-message"}, "ask"},
		{[]string{"palette", "run", "--hum", "say", "private-message"}, "palette"},
		{[]string{"palette", "open", "private-argument"}, "palette open"},
		{[]string{"new", "codex", "private-folder"}, "new"},
	} {
		if got := traceLabel(tc.args, false); got != tc.want {
			t.Errorf("%v: label=%q want=%q", tc.args, got, tc.want)
		}
		if got := traceLabel(tc.args, true); got != strings.Join(tc.args, " ") {
			t.Errorf("explicit content mode lost arguments: %q", got)
		}
	}
}
