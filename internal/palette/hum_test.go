package palette

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/actions"
)

func humEntries(t *testing.T) map[string]Entry {
	t.Helper()
	entries, err := Native()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Entry{}
	for _, e := range entries {
		if e.Kind == KindHum {
			out[e.ID] = e
		}
	}
	return out
}

func TestHumCommandsComeFromTheCatalogWithTheirOwnSource(t *testing.T) {
	got := humEntries(t)
	for _, id := range []string{"hum.agent.new", "hum.agent.say", "hum.agent.name", "hum.agent.notify"} {
		e, ok := got[id]
		if !ok {
			t.Errorf("%s is missing from the catalog", id)
			continue
		}
		if e.Source != "hum" || e.Description == "" {
			t.Errorf("%s: want source hum and a description, got %+v", id, e)
		}
	}
}

func TestUnknownRunnerIsDroppedLikeAnyOtherBadEntry(t *testing.T) {
	data := `{"schema_version":1,"commands":[
	  {"id":"ok","title":"T","runner":"hum","command":["ls"],"arguments":[]},
	  {"id":"herdr","title":"T","runner":"herdr","command":["pane","close"],"arguments":[]},
	  {"id":"bad","title":"T","runner":"bash","command":["ls"],"arguments":[]}
	]}`
	entries, err := parseCatalog([]byte(data))
	if err != nil || len(entries) != 2 || entries[0].Kind != KindHum || entries[1].Kind != KindNative {
		t.Errorf("got %+v %v", entries, err)
	}
}

func TestAnOptionalValueNeedsAFlagThatLooksLikeOne(t *testing.T) {
	data := `{"schema_version":1,"commands":[
	  {"id":"ok","title":"T","command":["x"],"arguments":[{"source":"input","prompt":"p","flag":"--name"}]},
	  {"id":"required","title":"T","command":["x"],"arguments":[{"source":"input","prompt":"p","flag":"--name","required":true}]},
	  {"id":"notflag","title":"T","command":["x"],"arguments":[{"source":"input","prompt":"p","flag":"name"}]}
	]}`
	entries, _ := parseCatalog([]byte(data))
	if got := ids(entries); !slices.Equal(got, []string{"ok"}) {
		t.Errorf("a required value cannot be omitted and a flag starts with --, got %v", got)
	}
}

func TestOptionalInputOmitsItsFlagWhenLeftEmpty(t *testing.T) {
	e := Entry{Command: []string{"new"}, Arguments: []Argument{
		{Source: SourceLiteral, Value: "claude"},
		{Source: SourceInput, Prompt: "name (optional)", Flag: "--name"},
	}}
	for _, tt := range []struct {
		typed string
		want  []string
	}{
		{"", []string{"new", "claude"}},
		{"   ", []string{"new", "claude"}},
		{"docs", []string{"new", "claude", "--name", "docs"}},
	} {
		got, ok, err := Resolve(context.Background(), e, &Context{}, &scripted{inputs: []string{tt.typed}})
		if err != nil || !ok || !slices.Equal(got, tt.want) {
			t.Errorf("typed %q: got %v (ok=%v err=%v), want %v", tt.typed, got, ok, err, tt.want)
		}
	}
}

func TestAgentKindsAreTheOnesHumNewAccepts(t *testing.T) {
	// Sin herdr: la lista es fija y es la misma de `hum new`, no una copia.
	choices, err := SelectorChoices(context.Background(), nil, "agent_kinds", Origin{}, "")
	if err != nil || len(choices) != len(actions.Kinds) {
		t.Fatalf("got %d choices (%v), want %d", len(choices), err, len(actions.Kinds))
	}
	for i, k := range actions.Kinds {
		if choices[i].Value != k || choices[i].Label != k {
			t.Errorf("choice %d = %+v, want %q", i, choices[i], k)
		}
	}
}

func TestHumCommandsBuildTheExpectedHumLines(t *testing.T) {
	got := humEntries(t)
	vals := &Context{Origin: Origin{PaneID: "w1:p3", Cwd: `C:\proj`}, Lists: &fakeLists{}}
	for _, tt := range []struct {
		id      string
		answers *scripted
		want    []string
	}{
		{"hum.agent.new", &scripted{choices: []string{"claude"}, inputs: []string{`C:\proj`, ""}}, []string{"new", "claude", `C:\proj`}},
		{"hum.agent.new", &scripted{choices: []string{"codex"}, inputs: []string{`D:\x`, "docs"}}, []string{"new", "codex", `D:\x`, "--name", "docs"}},
		{"hum.agent.say", &scripted{choices: []string{"p1"}, inputs: []string{"--force it"}}, []string{"say", "p1", "--", "--force it"}},
		{"hum.agent.name", &scripted{choices: []string{"p1"}, inputs: []string{"docs"}}, []string{"name", "p1", "docs"}},
		{"hum.agent.notify", &scripted{choices: []string{"p2"}}, []string{"wait", "p2", "--notify"}},
	} {
		argv, ok, err := Resolve(context.Background(), got[tt.id], vals, tt.answers)
		if err != nil || !ok || !slices.Equal(argv, tt.want) {
			t.Errorf("%s = %v (ok=%v err=%v), want %v", tt.id, argv, ok, err, tt.want)
		}
	}

	// La carpeta de «Agent: New…» parte de la del pane de origen.
	p := &scripted{choices: []string{"claude"}, inputs: []string{"x", ""}}
	if _, _, err := Resolve(context.Background(), got["hum.agent.new"], vals, p); err != nil || !strings.Contains(strings.Join(p.asked, "|"), `input:folder=C:\proj`) {
		t.Errorf("the folder question starts from the origin folder, asked %v (%v)", p.asked, err)
	}
}

// Los comandos de herdr se vigilan contra el herdr instalado (coverage_test.go);
// los de hum, contra la propia CLI de hum, que es la que los ejecuta.
func TestExcludedAndCoverageIgnoreHumCommands(t *testing.T) {
	inCatalog, err := CatalogCommands()
	if err != nil {
		t.Fatal(err)
	}
	for _, humCommand := range []string{"new", "say", "name", "wait"} {
		if _, found := inCatalog[humCommand]; found {
			t.Errorf("%q is a hum command and must not be checked against herdr", humCommand)
		}
	}
}
