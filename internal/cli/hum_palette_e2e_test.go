package cli_test

import (
	"slices"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

const oneAgent = `{"id":"x","result":{"agents":[{"pane_id":"w1:p9","agent":"claude","agent_status":"idle","terminal_title_stripped":"work"}]}}`

func TestPaletteEndToEndHumCommandGoesToTheDetachedProcessAsHum(t *testing.T) {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	fake.Reply("", []string{"agent", "list"}, oneAgent)
	r := startPaletteUI(t, fake, nil)

	_, _ = r.in.Write([]byte("agent: rename"))
	r.scr.waitFor(t, "Agent: Rename")
	_, _ = r.in.Write([]byte("\r"))
	r.scr.waitFor(t, "work") // la lista de agentes, pedida a herdr solo ahora
	_, _ = r.in.Write([]byte("\r"))
	r.scr.waitFor(t, "new name")
	_, _ = r.in.Write([]byte("docs\r"))

	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	if got := <-r.detach; !slices.Equal(got, []string{"palette", "run", "--hum", "name", "w1:p9", "docs"}) {
		t.Errorf("a hum command runs as hum, not as a herdr command: %v", got)
	}
}

func TestPaletteEndToEndNewAgentSkipsTheOptionalName(t *testing.T) {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	r := startPaletteUI(t, fake, nil)

	_, _ = r.in.Write([]byte("agent: new"))
	r.scr.waitFor(t, "Agent: New")
	_, _ = r.in.Write([]byte("\r"))
	r.scr.waitFor(t, "agent kind")
	_, _ = r.in.Write([]byte("codex")) // la lista de tipos sale de hum, sin consultar a herdr
	r.scr.waitFor(t, "codex")
	_, _ = r.in.Write([]byte("\r"))
	r.scr.waitFor(t, "folder")
	_, _ = r.in.Write([]byte("\r")) // la carpeta parte de la del pane de origen
	r.scr.waitFor(t, "name (optional)")
	_, _ = r.in.Write([]byte("\r")) // y el nombre se deja vacío

	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	if got := <-r.detach; !slices.Equal(got, []string{"palette", "run", "--hum", "new", "codex", `C:\proj`}) {
		t.Errorf("an empty optional name adds no --name: %v", got)
	}
}
