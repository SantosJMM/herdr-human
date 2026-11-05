package cli_test

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/complete"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/palette"
)

// Los comandos de herdr del catálogo se comparan con el herdr instalado; los de
// hum, con la propia CLI de hum. Si alguien renombra un comando o una opción de
// hum, la paleta fallaría al elegirlo: esto lo detecta antes.
func TestHumCommandsInTheCatalogExistInTheCLI(t *testing.T) {
	t.Parallel()
	entries, err := palette.Native()
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range entries {
		if e.Kind != palette.KindHum {
			continue
		}
		found++
		cmd := e.Command[0]
		if !slices.Contains(complete.Subcommands(), cmd) {
			t.Errorf("%s runs `hum %s`, which is not a hum command", e.ID, cmd)
			continue
		}
		known := map[string]bool{}
		for _, f := range complete.Flags(cmd) {
			known[f.Name] = true
		}
		for _, a := range e.Arguments {
			flag := a.Flag
			if a.Source == palette.SourceLiteral && strings.HasPrefix(a.Value, "--") && a.Value != "--" {
				flag = a.Value
			}
			if flag != "" && !known[flag] {
				t.Errorf("%s passes %s to `hum %s`, which does not take it", e.ID, flag, cmd)
			}
		}
	}
	if found == 0 {
		t.Fatal("no hum commands in the catalog: the check is not looking at anything")
	}
}

func TestPaletteRunExecutesAHumCommandInThisProcess(t *testing.T) {
	t.Parallel()
	// `alias ls` no necesita herdr: sale bien sin ninguna llamada.
	fake := herdrtest.New()
	out := runPalette(t, fake, cli.Env{}, "run", "--hum", "alias", "ls")
	if out.code != 0 || len(fake.Calls()) != 0 {
		t.Errorf("code=%d calls=%v stderr=%q", out.code, fake.Calls(), out.stderr)
	}
}

func TestPaletteRunNotifiesWhenAHumCommandFails(t *testing.T) {
	t.Parallel()
	fake := herdrtest.New()
	fake.Reply("", append(append([]string{}, notify...), `hum nope failed: unknown command "nope"`), okReply)
	out := runPalette(t, fake, cli.Env{}, "run", "--hum", "nope")
	if out.code != 2 {
		t.Errorf("an unknown hum command is a usage error: %+v", out)
	}
	if calls := fake.Calls(); len(calls) != 1 || calls[0].Args[0] != "notification" {
		t.Errorf("the failure is shown as a herdr notification, got %v", calls)
	}
}

func TestPaletteRunDoesNotNotifyWhenWaitEndsBlockedOrTimedOut(t *testing.T) {
	t.Parallel()
	// `hum wait` sale con 3 (el agente espera a la persona) o 4 (plazo vencido) y ya
	// lo contó por su cuenta: no es un fallo que avisar otra vez.
	for name, reply := range map[string]string{"blocked": waitBlockedReply, "timeout": waitTimeoutReply} {
		fake := fixture()
		fake.Reply("", []string{"agent", "wait", "wS:p1"}, reply)
		out := runPalette(t, fake, cli.Env{}, "run", "--hum", "wait", "importer")
		if out.code != 3 && out.code != 4 {
			t.Fatalf("%s: setup: want exit 3 or 4, got %+v", name, out)
		}
		for _, c := range fake.Calls() {
			if c.Args[0] == "notification" {
				t.Errorf("%s: no notification for a settled wait, got %v", name, c.Args)
			}
		}
	}
}

func TestPaletteRunRetriesAHumCommandWhileThePopupIsClosing(t *testing.T) {
	t.Parallel()
	// `hum new` abre un tab: mientras el popup sigue abierto herdr contesta ui_busy.
	fake := &busyThenOK{busy: 2}
	code := cli.Run(context.Background(), cli.Env{
		Runner: fake, Stdout: io.Discard, Stderr: io.Discard,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}, []string{"palette", "run", "--hum", "go", "w1:p2"})
	if code != 0 || fake.focusCalls != 3 {
		t.Errorf("code=%d focus calls=%d: ui_busy is retried until it works", code, fake.focusCalls)
	}
}

func TestPaletteRunNeverOpensAPaletteInsideThePalette(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"run", "--hum", "palette"}, {"run", "--hum", "palette", "run", "--hum", "alias", "ls"}, {"run", "--hum"}} {
		if out := runPalette(t, herdrtest.New(), cli.Env{}, args...); out.code != 2 {
			t.Errorf("%v: want usage error (2), got %+v", args, out)
		}
	}
}

// busyThenOK contesta ui_busy a las primeras llamadas de enfocar y luego bien. El
// resto de llamadas (la lista de agentes) las contesta como un herdr normal.
type busyThenOK struct {
	busy       int
	focusCalls int
}

func (b *busyThenOK) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	switch {
	case len(args) >= 2 && args[0] == "agent" && args[1] == "list":
		return []byte(`{"id":"x","result":{"agents":[{"agent":"codex","agent_status":"idle","pane_id":"w1:p2","tab_id":"w1:t2","workspace_id":"w1"}]}}`), nil
	case len(args) >= 2 && args[0] == "workspace" && args[1] == "list":
		return []byte(`{"id":"x","result":{"workspaces":[{"workspace_id":"w1","label":"one"}]}}`), nil
	case len(args) >= 2 && args[0] == "machine" && args[1] == "list":
		return nil, nil
	case len(args) >= 2 && args[0] == "api" && args[1] == "snapshot":
		return []byte(`{"error":{"code":"unknown_command","message":"no snapshot"},"id":"x"}`), nil
	case len(args) >= 2 && args[0] == "agent" && args[1] == "focus":
		b.focusCalls++
		if b.focusCalls <= b.busy {
			return nil, realUIBusy()
		}
	}
	return []byte(okReply), nil
}
