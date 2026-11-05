package palette

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func ids(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.ID
	}
	return out
}

// --- catálogo ---

func TestNativeCatalogShipsWithTheBinary(t *testing.T) {
	entries, err := Native()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 31 {
		t.Fatalf("want the 31 commands of the original catalog, got %d", len(entries))
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Kind != KindNative || e.Source != "Herdr" || seen[e.ID] {
			t.Fatalf("bad or repeated entry %+v", e)
		}
		seen[e.ID] = true
	}
}

func TestParseCatalogDropsBadEntriesButKeepsTheRest(t *testing.T) {
	data := `{"schema_version":1,"commands":[
	  {"id":"ok","title":"Ok","command":["pane","close"],"arguments":[{"source":"context","key":"pane_id"}]},
	  {"id":"ok","title":"Repeated","command":["x"],"arguments":[]},
	  {"id":"nocmd","title":"No command","command":[],"arguments":[]},
	  {"id":"badkey","title":"Bad key","command":["x"],"arguments":[{"source":"context","key":"nope"}]},
	  {"id":"badsel","title":"Bad selector","command":["x"],"arguments":[{"source":"select","selector":"nope","prompt":"p"}]},
	  {"id":"emptyliteral","title":"Empty","command":["x"],"arguments":[{"source":"literal","value":""}]}
	]}`
	entries, err := parseCatalog([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(entries); !slices.Equal(got, []string{"ok"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParseCatalogRejectsWrongSchemaAndBadJSON(t *testing.T) {
	if _, err := parseCatalog([]byte(`{"schema_version":2,"commands":[]}`)); err == nil {
		t.Error("a different schema_version must be refused")
	}
	if _, err := parseCatalog([]byte(`{`)); err == nil {
		t.Error("invalid JSON must be refused")
	}
}

// --- difuso ---

func TestScorePrefersPrefixAndWordStarts(t *testing.T) {
	prefix, _ := Score("tab close", "tab")
	deeper, _ := Score("close tab", "tab")
	if prefix <= deeper {
		t.Errorf("a prefix match (%d) must beat the same match deeper in the text (%d)", prefix, deeper)
	}
	if _, ok := Score("workspace", "wz"); ok {
		t.Error("a missing letter must not match")
	}
	if s, ok := Score("anything", ""); !ok || s != 0 {
		t.Errorf("an empty query matches everything with score 0, got %d %v", s, ok)
	}
	if _, ok := Score("Pane Close", "pane close"); !ok {
		t.Error("matching ignores case and spaces in the query")
	}
}

func TestFilterKeepsInputOrderOnTies(t *testing.T) {
	items := []string{"beta one", "alpha one", "gamma one"}
	got := Filter(items, "one", func(s string) string { return s })
	if !slices.Equal(got, items) {
		t.Errorf("equal scores must keep the incoming order, got %v", got)
	}
	if all := Filter(items, "  ", func(s string) string { return s }); !slices.Equal(all, items) {
		t.Errorf("a blank query returns everything, got %v", all)
	}
}

// --- plugins ---

func TestPluginEntriesFiltersAndNames(t *testing.T) {
	plugins := []PluginInfo{
		{PluginID: "a.one", Name: "One", Enabled: true, Actions: []PluginAction{
			{ID: "open", Title: "Open"},
			{ID: "unix-only", Title: "Unix", Platforms: []string{"linux"}},
			{ID: "win", Title: "", Platforms: []string{"windows"}},
		}},
		{PluginID: "b.off", Name: "Off", Enabled: false, Actions: []PluginAction{{ID: "x"}}},
		{PluginID: "me", Name: "Me", Enabled: true, Actions: []PluginAction{{ID: "palette"}, {ID: "pick"}}},
		{PluginID: "c.noname", Enabled: true, Actions: []PluginAction{{ID: "go", Title: "Go"}}},
	}
	got := PluginEntries(plugins, "windows", "me.palette")
	if want := []string{"a.one.open", "a.one.win", "me.pick", "c.noname.go"}; !slices.Equal(ids(got), want) {
		t.Fatalf("got %v want %v", ids(got), want)
	}
	if got[1].Title != "win" {
		t.Errorf("an action without a title shows its id, got %q", got[1].Title)
	}
	if got[3].Source != "c.noname" {
		t.Errorf("a plugin without a name shows its id, got %q", got[3].Source)
	}
}

func TestPlatformMapsDarwin(t *testing.T) {
	if Platform("darwin") != "macos" || Platform("windows") != "windows" {
		t.Error("darwin is called macos in the manifests")
	}
}

func TestListPluginsReadsOneCall(t *testing.T) {
	f := herdrtest.New()
	f.Reply("", []string{"plugin", "list", "--json"},
		`{"id":"x","result":{"plugins":[{"plugin_id":"a","name":"A","enabled":true,"actions":[{"id":"o","title":"O","platforms":["windows"]}]}]}}`)
	got, err := ListPlugins(context.Background(), f)
	if err != nil || len(got) != 1 || got[0].Actions[0].ID != "o" {
		t.Fatalf("got %+v %v", got, err)
	}
	if n := len(f.Calls()); n != 1 {
		t.Errorf("names and actions come from a single herdr call, got %d", n)
	}
}

// --- recientes, caché y orden ---

func TestRecentsRankAndPersist(t *testing.T) {
	dir := t.TempDir()
	if LoadRecents(dir).Rank("a") != maxRecents+1 {
		t.Fatal("never-used entries rank after every recent one")
	}
	LoadRecents(dir).Record(dir, "a").Record(dir, "b").Record(dir, "a")
	reloaded := LoadRecents(dir)
	if reloaded.Rank("a") != 0 || reloaded.Rank("b") != 1 || reloaded.Rank("zzz") != maxRecents+1 {
		t.Errorf("most recent first and no duplicates, got a=%d b=%d", reloaded.Rank("a"), reloaded.Rank("b"))
	}
}

func TestRecentsCapAndCorruptFile(t *testing.T) {
	dir := t.TempDir()
	r := Recents{}
	for i := 0; i < maxRecents+10; i++ {
		r = r.Record(dir, string(rune('A'+i%26))+string(rune('a'+i/26)))
	}
	if len(r.ids) != maxRecents {
		t.Errorf("recents are capped at %d, got %d", maxRecents, len(r.ids))
	}
	if err := os.WriteFile(filepath.Join(dir, recentsFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadRecents(dir); len(got.ids) != 0 {
		t.Error("a corrupt store degrades to no recents")
	}
}

func TestMergeOrdersRecentsThenSourceThenTitleAndAddsChords(t *testing.T) {
	a := Entry{ID: "z.a", Title: "Alpha", Source: "Zed"}
	b := Entry{ID: "a.b", Title: "Beta", Source: "Aaa"}
	c := Entry{ID: "a.c", Title: "Charlie", Source: "Aaa"}
	recents := Recents{ids: []string{"z.a"}}
	got := Merge(map[string]string{"a.b": "f2"}, recents, []Entry{c, a}, []Entry{b})
	if want := []string{"z.a", "a.b", "a.c"}; !slices.Equal(ids(got), want) {
		t.Fatalf("got %v want %v", ids(got), want)
	}
	if got[1].Chord != "f2" || got[0].Chord != "" {
		t.Errorf("chords: %q %q", got[0].Chord, got[1].Chord)
	}
}

func TestCacheRoundTripAndRejectsJunk(t *testing.T) {
	dir := t.TempDir()
	SaveCache(dir, []Entry{
		{Kind: KindPlugin, ID: "p.a", PluginID: "p", ActionID: "a", Title: "A", Source: "P"},
		{Kind: KindNative, ID: "native", Title: "N"},
		{Kind: KindPlugin, ID: "bad"},
	})
	got := LoadCache(dir)
	if len(got) != 1 || got[0].ID != "p.a" {
		t.Fatalf("only well-formed plugin entries survive the cache, got %+v", got)
	}
	if LoadCache(t.TempDir()) != nil {
		t.Error("no cache is nil")
	}
}

// --- atajos ---

func TestParseChordsReadsOnlyPluginActions(t *testing.T) {
	toml := `
[keys]
prefix = "ctrl+b"

[[keys.command]]
key = "f1"            # comentario
type = "plugin_action"
command = "x.palette.open"

[[keys.command]]
key = "prefix+t"
type = "popup"
command = "pwsh # not a comment"

[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "x.palette.open"

[[keys.command]]
key = "prefix+shift+l"
type = "plugin_action"
command = "herdr-human.pick-windows"
`
	got := ParseChords(toml)
	if got["x.palette.open"] != "f1" {
		t.Errorf("the first key of an action wins, got %q", got["x.palette.open"])
	}
	if got["herdr-human.pick-windows"] != "prefix+shift+l" || len(got) != 2 {
		t.Errorf("got %v", got)
	}
}

func TestConfigPathPrefersEnvAndMissingMeansNoChords(t *testing.T) {
	if got := ConfigPath(env(map[string]string{"HERDR_CONFIG_PATH": "/x/config.toml"})); got != "/x/config.toml" {
		t.Errorf("HERDR_CONFIG_PATH wins, got %q", got)
	}
	if LoadChords(env(map[string]string{"HERDR_CONFIG_PATH": filepath.Join(t.TempDir(), "none.toml")})) != nil {
		t.Error("an unreadable config means no chords")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[[keys.command]]\nkey = \"f1\"\ntype = \"plugin_action\"\ncommand = \"a.b\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadChords(env(map[string]string{"HERDR_CONFIG_PATH": path})); got["a.b"] != "f1" {
		t.Errorf("got %v", got)
	}
}

// --- origen y contexto ---

func TestOriginFromContextJSONAndFallbacks(t *testing.T) {
	json := `{"workspace_id":"w1","tab_id":"w1:t2","focused_pane_id":"w1:p3","focused_pane_cwd":"C:\\proj"}`
	o := OriginFromEnv(env(map[string]string{"HERDR_PLUGIN_CONTEXT_JSON": json}))
	if o != (Origin{PaneID: "w1:p3", TabID: "w1:t2", WorkspaceID: "w1", Cwd: `C:\proj`}) {
		t.Errorf("got %+v", o)
	}
	fb := OriginFromEnv(env(map[string]string{"HERDR_PLUGIN_CONTEXT_JSON": "not json", "HERDR_PANE_ID": "p9", "HERDR_TAB_ID": "t9", "HERDR_WORKSPACE_ID": "w9"}))
	if fb.PaneID != "p9" || fb.TabID != "t9" || fb.WorkspaceID != "w9" {
		t.Errorf("loose ids are the fallback, got %+v", fb)
	}
}

func TestAvailableHidesCommandsWithoutTheirContext(t *testing.T) {
	entries, _ := Native()
	none := Available(entries, Origin{})
	full := Available(entries, Origin{PaneID: "p", TabID: "t", WorkspaceID: "w", Cwd: "c"})
	if len(full) != len(entries) {
		t.Errorf("with a full origin every command is offered: %d of %d", len(full), len(entries))
	}
	if len(none) >= len(full) {
		t.Errorf("without an origin the context-bound commands disappear: %d vs %d", len(none), len(full))
	}
	for _, e := range none {
		if len(e.RequiredContext()) != 0 {
			t.Errorf("%s needs context and must be hidden", e.ID)
		}
	}
}

type fakeLists struct {
	ws    []Workspace
	tabs  []Tab
	calls int
	err   error
}

func (f *fakeLists) Workspaces(context.Context) ([]Workspace, error) { f.calls++; return f.ws, f.err }
func (f *fakeLists) Tabs(context.Context, string) ([]Tab, error)     { f.calls++; return f.tabs, f.err }
func (f *fakeLists) Agents(context.Context) ([]Agent, error) {
	f.calls++
	return []Agent{{PaneID: "p1", Agent: "claude", Status: "idle", Title: "work"}, {PaneID: "p2"}}, f.err
}

func TestNeighboursWrapAndAreComputedLazily(t *testing.T) {
	lists := &fakeLists{
		ws:   []Workspace{{ID: "w3", Number: 3}, {ID: "w1", Number: 1}, {ID: "w2", Number: 2}},
		tabs: []Tab{{ID: "t1", WorkspaceID: "w3", Number: 1}, {ID: "t2", WorkspaceID: "w3", Number: 2}, {ID: "tx", WorkspaceID: "w9", Number: 1}},
	}
	c := &Context{Origin: Origin{WorkspaceID: "w3", TabID: "t2"}, Lists: lists}
	if lists.calls != 0 {
		t.Fatal("building the context must not query herdr")
	}
	for key, want := range map[string]string{
		"next_workspace_id": "w1", "previous_workspace_id": "w2",
		"next_tab_id": "t1", "previous_tab_id": "t1",
	} {
		got, err := c.Value(context.Background(), key)
		if err != nil || got != want {
			t.Errorf("%s = %q %v, want %q", key, got, err, want)
		}
	}
	if lists.calls != 2 {
		t.Errorf("each list is fetched once and remembered, got %d calls", lists.calls)
	}
	if _, err := (&Context{Origin: Origin{WorkspaceID: "zz"}, Lists: lists}).Value(context.Background(), "next_workspace_id"); err == nil {
		t.Error("an unknown current workspace has no neighbour")
	}
	if v, _ := c.Value(context.Background(), "pane_id"); v != "" {
		t.Errorf("a static key missing from the origin is empty, got %q", v)
	}
}

func TestSingleItemIsItsOwnNeighbour(t *testing.T) {
	c := &Context{Origin: Origin{WorkspaceID: "w1"}, Lists: &fakeLists{ws: []Workspace{{ID: "w1", Number: 1}}}}
	if got, err := c.Value(context.Background(), "next_workspace_id"); err != nil || got != "w1" {
		t.Errorf("got %q %v", got, err)
	}
}

// --- resolución ---

type scripted struct {
	inputs  []string
	choices []string
	cancel  bool
	asked   []string
}

func (s *scripted) Input(_ context.Context, spec InputSpec) (string, bool, error) {
	s.asked = append(s.asked, "input:"+spec.Prompt+"="+spec.Initial)
	if s.cancel {
		return "", false, nil
	}
	v := s.inputs[0]
	s.inputs = s.inputs[1:]
	return v, true, nil
}

func (s *scripted) Choose(_ context.Context, prompt string, choices []Choice) (string, bool, error) {
	s.asked = append(s.asked, "choose:"+prompt+":"+strings.Join(labels(choices), ","))
	if s.cancel {
		return "", false, nil
	}
	v := s.choices[0]
	s.choices = s.choices[1:]
	return v, true, nil
}

func labels(cs []Choice) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Label
	}
	return out
}

func TestResolveBuildsTheCommandInOrder(t *testing.T) {
	e := Entry{Command: []string{"workspace", "create"}, Arguments: []Argument{
		{Source: SourceLiteral, Value: "--cwd"},
		{Source: SourceInput, Prompt: "dir", DefaultContext: "cwd", Required: true},
		{Source: SourceLiteral, Value: "--focus"},
		{Source: SourceContext, Key: "tab_id"},
	}}
	p := &scripted{inputs: []string{"D:\\x"}}
	argv, ok, err := Resolve(context.Background(), e, &Context{Origin: Origin{Cwd: "C:\\here", TabID: "t1"}}, p)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if want := []string{"workspace", "create", "--cwd", "D:\\x", "--focus", "t1"}; !slices.Equal(argv, want) {
		t.Errorf("got %v", argv)
	}
	if p.asked[0] != `input:dir=C:\here` {
		t.Errorf("the input starts from the context default, got %v", p.asked)
	}
	if e.Command[0] != "workspace" || len(e.Command) != 2 {
		t.Error("resolving must not modify the catalog entry")
	}
}

func TestResolveCancelAbortsTheWholeCommand(t *testing.T) {
	e := Entry{Command: []string{"tab", "rename"}, Arguments: []Argument{
		{Source: SourceContext, Key: "tab_id"},
		{Source: SourceInput, Prompt: "name"},
	}}
	argv, ok, err := Resolve(context.Background(), e, &Context{Origin: Origin{TabID: "t1"}}, &scripted{cancel: true})
	if err != nil || ok || argv != nil {
		t.Errorf("Esc in a question runs nothing: %v %v %v", argv, ok, err)
	}
}

func TestResolveSelectExcludesAndErrorsClearly(t *testing.T) {
	lists := &fakeLists{ws: []Workspace{{ID: "w1", Label: "one"}, {ID: "w2"}}}
	e := Entry{Command: []string{"workspace", "focus"}, Arguments: []Argument{
		{Source: SourceSelect, Selector: "workspaces", Prompt: "workspace", ExcludeContext: "workspace_id"},
	}}
	p := &scripted{choices: []string{"w2"}}
	argv, ok, err := Resolve(context.Background(), e, &Context{Origin: Origin{WorkspaceID: "w1"}, Lists: lists}, p)
	if err != nil || !ok || !slices.Equal(argv, []string{"workspace", "focus", "w2"}) {
		t.Fatalf("got %v %v %v", argv, ok, err)
	}
	if p.asked[0] != "choose:workspace:w2" {
		t.Errorf("the current workspace is not offered and an unlabeled one shows its id, got %v", p.asked)
	}

	_, _, err = Resolve(context.Background(), e, &Context{Origin: Origin{WorkspaceID: "w1"}, Lists: &fakeLists{ws: []Workspace{{ID: "w1"}}}}, &scripted{})
	if err == nil || !strings.Contains(err.Error(), "nothing to choose") {
		t.Errorf("an empty list is an explained error, got %v", err)
	}
	_, _, err = Resolve(context.Background(), Entry{Command: []string{"x"}, Arguments: []Argument{{Source: SourceContext, Key: "pane_id"}}}, &Context{}, &scripted{})
	if err == nil || !strings.Contains(err.Error(), "pane_id") {
		t.Errorf("a missing context key names the key, got %v", err)
	}
	_, _, err = Resolve(context.Background(), e, &Context{Origin: Origin{WorkspaceID: "w1"}, Lists: &fakeLists{err: errors.New("boom")}}, &scripted{})
	if err == nil {
		t.Error("a failing herdr list is reported")
	}
}

func TestAgentChoicesShowStatusGlyph(t *testing.T) {
	choices, err := SelectorChoices(context.Background(), &fakeLists{}, "agents", Origin{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := labels(choices); got[0] != "○ claude — work" || got[1] != "· agent — p2" {
		t.Errorf("got %v", got)
	}
	if _, err := SelectorChoices(context.Background(), &fakeLists{}, "nope", Origin{}, ""); err == nil {
		t.Error("an unknown selector is an error")
	}
	if _, err := SelectorChoices(context.Background(), nil, "tabs", Origin{}, ""); err == nil {
		t.Error("no herdr connection is an error, not a panic")
	}
}

func TestHerdrListerParsesTheRealShapes(t *testing.T) {
	f := herdrtest.New()
	f.Reply("", []string{"workspace", "list"}, `{"result":{"workspaces":[{"workspace_id":"w1","label":"a","number":2}]}}`)
	f.Reply("", []string{"tab", "list", "--workspace", "w1"}, `{"result":{"tabs":[{"tab_id":"w1:t1","workspace_id":"w1","label":"1","number":1}]}}`)
	f.Reply("", []string{"agent", "list"}, `{"result":{"agents":[{"pane_id":"p","agent":"claude","agent_status":"working","terminal_title_stripped":"T"}]}}`)
	l := HerdrLister{Runner: f}
	ws, err1 := l.Workspaces(context.Background())
	tabs, err2 := l.Tabs(context.Background(), "w1")
	ags, err3 := l.Agents(context.Background())
	if err1 != nil || err2 != nil || err3 != nil || ws[0].Number != 2 || tabs[0].ID != "w1:t1" || ags[0].Status != "working" {
		t.Fatalf("%v %v %v %+v %+v %+v", err1, err2, err3, ws, tabs, ags)
	}
}

// --- paridad con las pruebas del plugin original ---

func TestRecentsKeepTheValidIDsOfAMixedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, recentsFile), []byte(`["a", 5, null, "", "b", {"x":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadRecents(dir)
	if got.Rank("a") != 0 || got.Rank("b") != 1 {
		t.Errorf("one bad entry must not cost the whole history, got a=%d b=%d", got.Rank("a"), got.Rank("b"))
	}
}

func TestInputDefaultMustBeAStaticContextKey(t *testing.T) {
	data := `{"schema_version":1,"commands":[
	  {"id":"computed","title":"T","command":["x"],"arguments":[{"source":"input","prompt":"p","default_context":"next_tab_id"}]},
	  {"id":"static","title":"T","command":["x"],"arguments":[{"source":"input","prompt":"p","default_context":"cwd"}]}
	]}`
	entries, err := parseCatalog([]byte(data))
	if err != nil || len(entries) != 1 || entries[0].ID != "static" {
		t.Errorf("a computed key cannot seed an input: %v %v", ids(entries), err)
	}
}

func TestShippedCatalogOnlyDrivesKnownHerdrNouns(t *testing.T) {
	entries, err := Native()
	if err != nil {
		t.Fatal(err)
	}
	nouns := []string{"workspace", "tab", "pane", "agent", "server"}
	for _, e := range entries {
		if !slices.Contains(nouns, e.Command[0]) {
			t.Errorf("unexpected noun in %s: %s", e.ID, e.Command[0])
		}
	}
}
