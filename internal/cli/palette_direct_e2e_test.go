package cli_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func directFake() *herdrtest.Fake {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	fake.Reply("", []string{"agent", "list"}, oneAgent)
	return fake
}

func TestPaletteOpenedByAShortcutGoesStraightToTheAgentPicker(t *testing.T) {
	// Es lo que hace el atajo de «Agent: Focus…»: abre el popup con
	// HUM_PALETTE_ENTRY=agent.focus. No se teclea nada para llegar a la lista.
	r := startPaletteUI(t, directFake(), map[string]string{"HUM_PALETTE_ENTRY": "agent.focus"})

	r.scr.waitFor(t, "work") // el agente de la lista de elección
	_, _ = r.in.Write([]byte("\r"))
	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	if got := <-r.detach; !slices.Equal(got, []string{"palette", "run", "--command", "agent", "focus", "w1:p9"}) {
		t.Errorf("the chosen agent is focused through the usual detached run: %v", got)
	}
}

func TestAShortcutOnAHumCommandAsksEverythingItNeedsAndRunsItAsHum(t *testing.T) {
	r := startPaletteUI(t, directFake(), map[string]string{"HUM_PALETTE_ENTRY": "hum.agent.name"})

	r.scr.waitFor(t, "work") // 1.º: a qué agente
	_, _ = r.in.Write([]byte("\r"))
	r.scr.waitFor(t, "new name") // 2.º: qué nombre
	_, _ = r.in.Write([]byte("docs\r"))
	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	if got := <-r.detach; !slices.Equal(got, []string{"palette", "run", "--hum", "name", "w1:p9", "docs"}) {
		t.Errorf("%v", got)
	}
}

func TestAShortcutOnACommandThatNeedsTheOriginPaneWorksFromWhereItWasPressed(t *testing.T) {
	// «Tab: Close current» no pregunta nada, pero con StartWith también entra directo:
	// usa el contexto de origen que herdr da al popup.
	r := startPaletteUI(t, directFake(), map[string]string{"HUM_PALETTE_ENTRY": "tab.close"})
	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	if got := <-r.detach; !slices.Equal(got, []string{"palette", "run", "--command", "tab", "close", "w1:t2"}) {
		t.Errorf("%v", got)
	}
}

func TestAShortcutWhoseEntryIsNotAvailableHereOpensTheListWithANotice(t *testing.T) {
	// Sin pane de origen, «Pane: Close current» no se ofrece: la paleta no ejecuta
	// nada a ciegas, abre la lista y lo dice.
	r := startPaletteUI(t, directFake(), map[string]string{
		"HUM_PALETTE_ENTRY":         "pane.close",
		"HERDR_PLUGIN_CONTEXT_JSON": `{}`,
	})
	r.scr.waitFor(t, "«pane.close» is not available from here")
	_, _ = r.in.Write([]byte("\x1b"))
	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d", code)
	}
	select {
	case args := <-r.detach:
		t.Errorf("nothing must run: %v", args)
	default:
	}
}

func TestWithoutTheVariableThePaletteOpensTheListAsAlways(t *testing.T) {
	r := startPaletteUI(t, directFake(), nil)
	r.scr.waitFor(t, "type to search commands and plugin actions")
	if strings.Contains(r.scr.buf.String(), "is not available from here") {
		t.Error("no notice without the variable")
	}
	_, _ = r.in.Write([]byte("\x1b"))
	r.wait(t)
}
