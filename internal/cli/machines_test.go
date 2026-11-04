package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

var machineListJSON = []string{"machine", "list", "--json"}

// profilesJSON arma `machine list --json` con odoous y remoto1 en el estado dado.
func profilesJSON(odoous, remoto1 bool) string {
	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	return `[` +
		`{"id":"id1","label":"remoto2","target":"srv2","session":"default","enabled":false,"selected":false},` +
		`{"id":"id2","label":"remoto1","target":"srv","session":"default","enabled":` + b(remoto1) + `,"selected":false},` +
		`{"id":"id3","label":"WSL Ubuntu","target":"ssh://u@127.0.0.1:2222","session":"default","enabled":false,"selected":false},` +
		`{"id":"id4","label":"odoous","target":"servidor-odoous","session":"default","enabled":` + b(odoous) + `,"selected":false}]`
}

func runIn(t *testing.T, fake *herdrtest.Fake, stdin string, args ...string) output {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake,
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
	}, args)
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func mutations(f *herdrtest.Fake, verb string) int {
	n := 0
	for _, c := range f.Calls() {
		if len(c.Args) >= 2 && c.Args[0] == "machine" && c.Args[1] == verb {
			n++
		}
	}
	return n
}

func TestMachinesListHuman(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	out := runIn(t, fake, "", "machines")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, want := range []string{"ENABLED", "DISABLED", "servidor-odoous", "default"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, out.stdout)
		}
	}
	order := []string{"odoous", "remoto1", "remoto2", "WSL Ubuntu"}
	last := -1
	for _, label := range order {
		i := strings.Index(out.stdout, label)
		if i <= last {
			t.Errorf("%q is out of alphabetical order:\n%s", label, out.stdout)
		}
		last = i
	}
}

func TestMachinesListJSON(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	out := runIn(t, fake, "", "machines", "--json")
	var got []map[string]any
	if err := json.Unmarshal([]byte(out.stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.stdout)
	}
	if len(got) != 4 || got[0]["label"] != "odoous" {
		t.Errorf("got %v, want 4 profiles sorted with odoous first", got)
	}
}

func TestMachinesChangeState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		id         string
		verb       string
		before     string
		after      string
		wantStdout string
	}{
		{"disable", []string{"machines", "disable", "odoous"}, "id4", "disable",
			profilesJSON(true, false), profilesJSON(false, false), "disabled odoous"},
		{"enable", []string{"machines", "enable", "remoto1"}, "id2", "enable",
			profilesJSON(true, false), profilesJSON(true, true), "enabled remoto1"},
		{"toggle an enabled one", []string{"machines", "toggle", "odoous"}, "id4", "disable",
			profilesJSON(true, false), profilesJSON(false, false), "disabled odoous"},
		{"toggle a disabled one", []string{"machines", "toggle", "remoto1"}, "id2", "enable",
			profilesJSON(true, false), profilesJSON(true, true), "enabled remoto1"},
		{"unique prefix", []string{"machines", "disable", "odo"}, "id4", "disable",
			profilesJSON(true, false), profilesJSON(false, false), "disabled odoous"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := herdrtest.New()
			// Lectura para elegir, relectura previa y lectura de confirmación.
			fake.Sequence("", machineListJSON, tt.before, tt.before, tt.after)
			fake.Reply("", []string{"machine", tt.verb, tt.id}, "ok")

			out := runIn(t, fake, "", tt.args...)
			if out.code != 0 || !strings.Contains(out.stdout, tt.wantStdout) {
				t.Errorf("code=%d stdout=%q stderr=%q, want %q", out.code, out.stdout, out.stderr, tt.wantStdout)
			}
			if n := mutations(fake, tt.verb); n != 1 {
				t.Errorf("%s called %d times, want 1", tt.verb, n)
			}
		})
	}
}

func TestMachinesAlreadyInTheWantedState(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	out := runIn(t, fake, "", "machines", "enable", "odoous")
	if out.code != 0 || !strings.Contains(out.stdout, "already enabled") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
	if n := mutations(fake, "enable") + mutations(fake, "disable"); n != 0 {
		t.Errorf("made %d mutations, want 0", n)
	}
}

func TestMachinesUnknownName(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	out := runIn(t, fake, "", "machines", "disable", "nope")
	if out.code != 1 || !strings.Contains(out.stderr, `no saved machine matches "nope"`) {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestMachinesUsageErrors(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"machines", "enable"},
		{"machines", "disable", "a", "b"},
		{"machines", "frobnicate"},
		{"machines", "--nope"},
	} {
		fake := herdrtest.New()
		fake.Reply("", machineListJSON, profilesJSON(true, false))
		if out := runIn(t, fake, "", args...); out.code != 2 || out.stderr == "" {
			t.Errorf("hum %v: code=%d stderr=%q, want a usage error", args, out.code, out.stderr)
		}
	}
}

func TestMachinesMenuTogglesBySelectedNumber(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	// Lista inicial, relectura previa, confirmación y lista refrescada.
	fake.Sequence("", machineListJSON,
		profilesJSON(true, false), profilesJSON(true, false), profilesJSON(false, false), profilesJSON(false, false))
	fake.Reply("", []string{"machine", "disable", "id4"}, "ok")

	out := runIn(t, fake, "1\nq\n", "machines", "menu")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, want := range []string{
		"Herdr machines", "[1] ENABLED", "odoous", "not a live connection check",
		"remote sessions and processes running", "Requested disable: odoous", "Q: quit",
	} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, out.stdout)
		}
	}
	if n := mutations(fake, "disable"); n != 1 {
		t.Errorf("disable called %d times, want 1", n)
	}
}

func TestMachinesMenuRejectsAnInvalidChoice(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	out := runIn(t, fake, "9\nabc\nq\n", "machines", "menu")
	if out.code != 0 || strings.Count(out.stdout, "Choose a number from the current list, R, or Q.") != 2 {
		t.Errorf("code=%d, want the invalid-choice notice twice:\n%s", out.code, out.stdout)
	}
	if n := mutations(fake, "enable") + mutations(fake, "disable"); n != 0 {
		t.Errorf("made %d mutations, want 0", n)
	}
}

func TestMachinesMenuReportsAProfileThatChangedElsewhere(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	// La persona ve odoous encendida, pero la relectura la encuentra apagada.
	fake.Sequence("", machineListJSON, profilesJSON(true, false), profilesJSON(false, false))

	out := runIn(t, fake, "1\nq\n", "machines", "menu")
	if out.code != 0 || !strings.Contains(out.stdout, "Action not confirmed") {
		t.Errorf("code=%d, want an unconfirmed-action notice:\n%s", out.code, out.stdout)
	}
	if n := mutations(fake, "enable") + mutations(fake, "disable"); n != 0 {
		t.Errorf("made %d mutations, want 0", n)
	}
}

func TestMachinesMenuQuitsOnEndOfInput(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", machineListJSON, profilesJSON(true, false))

	if out := runIn(t, fake, "r\n", "machines", "menu"); out.code != 0 {
		t.Errorf("code = %d, want 0 when input ends", out.code)
	}
	if out := run(t, fake, "machines", "menu"); out.code != 0 {
		t.Errorf("code = %d, want 0 without stdin", out.code)
	}
}
