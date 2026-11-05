package keybind

import (
	"slices"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/palette"
)

// sample reproduce la forma de un config.toml real: comentarios, columnas
// alineadas, un comentario al final de una tecla y bloques con marcadores.
const sample = `# My Herdr Config

onboarding = false

[ui]
prompt_new_tab_name = false

# --- Over keys ---

[keys.indexed]
workspaces = "alt"
agents = "ctrl"

# --- Custom keys ---

[[keys.command]]
key = "prefix+alt+g"          # prefix=ctrl+b
type = "popup"
command = "lazygit"
description = "run lazygit"
width = "80%"
height = "80%"

[[keys.command]]
key = "f1"
type = "plugin_action"
command = "herdr-human.palette"
description = "command palette"

[[keys.command]]
key = "prefix+shift+l"
type = "plugin_action"
command = "herdr-human.pick-windows"
description = "talk to agents (hum pick)"

# BEGIN pane focus keys
[[keys.command]]
key = "alt+left"
type = "shell"
command = "herdr pane focus --current --direction left"
description = "focus pane left"
# END pane focus keys
`

var (
	palette1 = Binding{TypePluginAction, "herdr-human.palette"}
	pick     = Binding{TypePluginAction, "herdr-human.pick-windows"}
	lazygit  = Binding{"popup", "lazygit"}
	focusL   = Binding{TypeShell, "herdr pane focus --current --direction left"}
	fresh    = Binding{TypePluginAction, "x.plug.ping"}
)

func lines(s string) []string { return strings.SplitAfter(s, "\n") }

// onlyChanged devuelve los números de línea que cambian entre dos textos del mismo largo.
func onlyChanged(t *testing.T, before, after string) []int {
	t.Helper()
	a, b := lines(before), lines(after)
	if len(a) != len(b) {
		t.Fatalf("different number of lines: %d vs %d", len(a), len(b))
	}
	var changed []int
	for i := range a {
		if a[i] != b[i] {
			changed = append(changed, i)
		}
	}
	return changed
}

// --- teclas ---

func TestNormalizeGivesTheCanonicalFormOrExplainsWhy(t *testing.T) {
	for in, want := range map[string]string{
		"F1":               "f1",
		" ctrl+a ":         "ctrl+a",
		"alt+ctrl+left":    "ctrl+alt+left",
		"shift+prefix+l":   "", // el prefijo va primero
		"prefix+shift+l":   "prefix+shift+l",
		"PREFIX+Alt+G":     "prefix+alt+g",
		"ctrl+alt+]":       "ctrl+alt+]",
		"prefix+?":         "prefix+?",
		"prefix+minus":     "prefix+minus",
		"cmd+shift+k":      "shift+cmd+k",
		"ctrl+ctrl+a":      "", // repetido
		"wat+q":            "", // modificador inventado
		"ctrl+":            "",
		"+a":               "",
		"":                 "",
		"ctrl+alt":         "", // acaba en un modificador
		"prefix":           "",
		"ctrl+two words":   "",
		"prefix+prefix+a":  "",
		"enter":            "enter",
		"ctrl+alt+shift+f": "ctrl+alt+shift+f",
	} {
		got, err := Normalize(in)
		if want == "" {
			if err == nil {
				t.Errorf("Normalize(%q) = %q, want an error", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q %v, want %q", in, got, err, want)
		}
	}
}

func TestSameKeyIgnoresCaseAndModifierOrder(t *testing.T) {
	if !SameKey("Alt+Ctrl+Left", "ctrl+alt+left") || SameKey("ctrl+a", "ctrl+b") || SameKey("ctrl+", "ctrl+") {
		t.Error("keys are compared in canonical form; invalid keys are never the same")
	}
}

func TestOnlySafeDirectKeysAreAcceptedWithoutAPrefix(t *testing.T) {
	for key, want := range map[string]bool{
		"a": false, "?": false, "1": false, // comen lo que se escribe
		"f1": true, "home": true, "ctrl+a": true, "alt+left": true, "prefix+a": true, "prefix+?": true, "": false,
	} {
		if got := IsSafeDirect(key); got != want {
			t.Errorf("IsSafeDirect(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestRuneNamesFollowHerdrsSpelling(t *testing.T) {
	for r, want := range map[rune]string{'-': "minus", ',': "comma", '&': "ampersand", '+': "plus", '`': "backtick", ' ': "space", 'A': "a", ']': "]", '?': "?"} {
		if got := RuneName(r); got != want {
			t.Errorf("RuneName(%q) = %q, want %q", r, got, want)
		}
	}
}

// --- lectura ---

func TestParseFindsEveryCommandBlockWithItsKey(t *testing.T) {
	c := Parse(sample)
	if got := len(c.Blocks()); got != 4 {
		t.Fatalf("want 4 [[keys.command]] blocks, got %d", got)
	}
	for b, want := range map[Binding]string{palette1: "f1", pick: "prefix+shift+l", lazygit: "prefix+alt+g", focusL: "alt+left", fresh: ""} {
		if got := c.KeyOf(b); got != want {
			t.Errorf("KeyOf(%v) = %q, want %q", b, got, want)
		}
	}
	if c.Text() != sample {
		t.Error("reading the file must not change its text")
	}
}

func TestParseIgnoresWhatItDoesNotUnderstand(t *testing.T) {
	c := Parse("[[keys.command]]\nkey = [\"a\", \"b\"]\ntype = \"shell\"\ncommand = \"x\"\n\n[[other]]\nkey = \"f9\"\ntype = \"shell\"\ncommand = \"y\"\n")
	if got := c.KeyOf(Binding{TypeShell, "x"}); got != "" {
		t.Errorf("a list value is not a single key, got %q", got)
	}
	if len(c.Find(Binding{TypeShell, "y"})) != 0 {
		t.Error("a block of another table is not a key binding")
	}
}

// --- edición ---

func TestSetOnAnExistingBlockChangesOnlyItsKeyAndKeepsTheComment(t *testing.T) {
	out, err := Parse(sample).Set(lazygit, "ctrl+alt+g", "")
	if err != nil {
		t.Fatal(err)
	}
	changed := onlyChanged(t, sample, out)
	if len(changed) != 1 {
		t.Fatalf("exactly one line changes, got %v", changed)
	}
	if got := strings.TrimRight(lines(out)[changed[0]], "\n"); got != `key = "ctrl+alt+g"          # prefix=ctrl+b` {
		t.Errorf("the alignment and the trailing comment stay: %q", got)
	}
}

func TestSetAppendsANewBlockAndLeavesTheRestByteForByte(t *testing.T) {
	out, err := Parse(sample).Set(fresh, "f4", "Ping the thing")
	if err != nil {
		t.Fatal(err)
	}
	want := sample + "\n[[keys.command]]\nkey = \"f4\"\ntype = \"plugin_action\"\ncommand = \"x.plug.ping\"\ndescription = \"Ping the thing\"\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if !strings.HasPrefix(out, sample) {
		t.Error("nothing before the new block may change")
	}
}

func TestSetHandlesAFileWithoutATrailingNewlineAndAnEmptyFile(t *testing.T) {
	out, _ := Parse("onboarding = false").Set(fresh, "f4", "")
	if out != "onboarding = false\n\n[[keys.command]]\nkey = \"f4\"\ntype = \"plugin_action\"\ncommand = \"x.plug.ping\"\n" {
		t.Errorf("got %q", out)
	}
	out, _ = Parse("").Set(fresh, "f4", "")
	if out != "[[keys.command]]\nkey = \"f4\"\ntype = \"plugin_action\"\ncommand = \"x.plug.ping\"\n" {
		t.Errorf("an empty file starts with the block, got %q", out)
	}
}

func TestSetKeepsWindowsLineEndings(t *testing.T) {
	crlf := strings.ReplaceAll(sample, "\n", "\r\n")
	for name, b := range map[string]Binding{"new block": fresh, "existing block": lazygit} {
		out, err := Parse(crlf).Set(b, "ctrl+alt+k", "d")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
			t.Errorf("%s: a bare \\n in a CRLF file", name)
		}
	}
}

func TestSetOnABlockWithoutAKeyLineInsertsOne(t *testing.T) {
	out, err := Parse("[[keys.command]]\ntype = \"shell\"\ncommand = \"x\"\n").Set(Binding{TypeShell, "x"}, "f5", "")
	if err != nil || out != "[[keys.command]]\nkey = \"f5\"\ntype = \"shell\"\ncommand = \"x\"\n" {
		t.Errorf("got %q %v", out, err)
	}
}

func TestSetRefusesWhatCannotBeWrittenInsideAString(t *testing.T) {
	for _, key := range []string{"", `a"b`, "a\nb", `a\b`} {
		if _, err := Parse(sample).Set(fresh, key, ""); err == nil {
			t.Errorf("Set(%q) must fail", key)
		}
	}
}

func TestSetEscapesTheCommandAndTheDescription(t *testing.T) {
	b := Binding{TypeShell, `"C:\x y\h.exe" a`}
	out, _ := Parse("").Set(b, "f6", `say "hi"`)
	if !strings.Contains(out, `command = "\"C:\\x y\\h.exe\" a"`) || !strings.Contains(out, `description = "say \"hi\""`) {
		t.Errorf("quotes and backslashes are escaped:\n%s", out)
	}
	if got := Parse(out).KeyOf(b); got != "f6" {
		t.Errorf("what was written reads back as the same binding, got %q", got)
	}
}

func TestRemoveDeletesTheBlockAndOneSeparatingBlankLine(t *testing.T) {
	out, err := Parse(sample).Remove(palette1)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(sample, "[[keys.command]]\nkey = \"f1\"\ntype = \"plugin_action\"\ncommand = \"herdr-human.palette\"\ndescription = \"command palette\"\n\n", "", 1)
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if Parse(out).KeyOf(palette1) != "" || Parse(out).KeyOf(pick) != "prefix+shift+l" {
		t.Error("only that action loses its shortcut")
	}
}

func TestRemoveTheLastBlockLeavesItsMarkerCommentsAndNoDanglingBlankLine(t *testing.T) {
	out, err := Parse(sample).Remove(focusL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# BEGIN pane focus keys\n# END pane focus keys\n") {
		t.Errorf("comments around the block are not ours to delete:\n%s", out)
	}
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("no double blank lines:\n%s", out)
	}
}

func TestAddThenRemoveGivesBackTheOriginalFile(t *testing.T) {
	for _, original := range []string{sample, strings.ReplaceAll(sample, "\n", "\r\n"), "onboarding = false\n", ""} {
		added, err := Parse(original).Set(fresh, "f4", "d")
		if err != nil {
			t.Fatal(err)
		}
		removed, err := Parse(added).Remove(fresh)
		if err != nil {
			t.Fatal(err)
		}
		if removed != original {
			t.Errorf("remove(add(x)) must be x.\nwant %q\ngot  %q", original, removed)
		}
	}
}

func TestRemoveOfAnUnboundActionIsAnError(t *testing.T) {
	if _, err := Parse(sample).Remove(fresh); err != ErrNotBound {
		t.Errorf("got %v", err)
	}
}

func TestRemoveClearsEveryDuplicateBlock(t *testing.T) {
	dup := "[[keys.command]]\nkey = \"f1\"\ntype = \"shell\"\ncommand = \"x\"\n\n[[keys.command]]\nkey = \"f2\"\ntype = \"shell\"\ncommand = \"x\"\n\n[[keys.command]]\nkey = \"f3\"\ntype = \"shell\"\ncommand = \"other\"\n"
	out, err := Parse(dup).Remove(Binding{TypeShell, "x"})
	if err != nil || out != "[[keys.command]]\nkey = \"f3\"\ntype = \"shell\"\ncommand = \"other\"\n" {
		t.Errorf("got %q %v", out, err)
	}
}

// --- conflictos ---

func conflictNames(cs []Conflict) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Source+":"+c.Name)
	}
	return out
}

func TestConflictWithAnotherCommandInTheFile(t *testing.T) {
	c := Parse(sample)
	got := c.Conflicts("F1", fresh)
	if !slices.Equal(conflictNames(got), []string{"custom:herdr-human.palette"}) || !got[0].Blocking() {
		t.Errorf("got %v", got)
	}
	if again := c.Conflicts("f1", palette1); len(again) != 0 {
		t.Errorf("an action's own key is not a conflict, got %v", again)
	}
	if got := c.Conflicts("alt+ctrl+x", fresh); len(got) != 0 {
		t.Errorf("a free key has no conflicts, got %v", got)
	}
	if got := Parse("[[keys.command]]\nkey = \"ctrl+alt+k\"\ntype = \"shell\"\ncommand = \"a\"\n").Conflicts("alt+ctrl+k", fresh); len(got) != 1 {
		t.Errorf("modifier order does not matter, got %v", got)
	}
}

func TestConflictWithAnInternalActionTheFileSets(t *testing.T) {
	c := Parse("[keys]\nprefix = \"ctrl+space\"\nnext_tab = [\"prefix+n\", \"ctrl+alt+]\"]\n")
	if got := c.Conflicts("ctrl+alt+]", fresh); !slices.Equal(conflictNames(got), []string{"configured:keys.next_tab"}) {
		t.Errorf("got %v", got)
	}
	if got := c.Conflicts("ctrl+space", fresh); !slices.Equal(conflictNames(got), []string{"configured:keys.prefix"}) {
		t.Errorf("got %v", got)
	}
	// prefix+n es lo que el archivo dice para next_tab: no se cuenta además como el
	// valor por defecto de herdr.
	if got := c.Conflicts("prefix+n", fresh); !slices.Equal(conflictNames(got), []string{"configured:keys.next_tab"}) {
		t.Errorf("an overridden default is not reported twice, got %v", got)
	}
}

func TestConflictWithTheIndexedModifiers(t *testing.T) {
	c := Parse(sample)
	for key, want := range map[string][]string{
		"alt+3":   {"configured:keys.indexed.workspaces"},
		"ctrl+9":  {"configured:keys.indexed.agents"},
		"alt+0":   nil, // solo 1..9
		"alt+a":   nil,
		"shift+1": nil,
	} {
		if got := conflictNames(c.Conflicts(key, fresh)); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", key, got, want)
		}
	}
}

func TestConflictWithHerdrsDefaultsIsAWarningNotABlock(t *testing.T) {
	c := Parse("")
	for key, want := range map[string]string{
		"prefix+n":       "default:keys.next_tab",
		"prefix+g":       "default:keys.goto",
		"prefix+x":       "default:keys.close_pane",
		"prefix+shift+l": "default:keys.swap_pane_right",
		"ctrl+b":         "default:keys.prefix",
		"prefix+3":       "default:keys.switch_tab",
		"prefix+?":       "default:keys.help",
	} {
		got := c.Conflicts(key, fresh)
		if len(got) != 1 || got[0].Source+":"+got[0].Name != want || got[0].Blocking() {
			t.Errorf("%s: got %v, want one non-blocking %s", key, got, want)
		}
	}
	for _, free := range []string{"f12", "ctrl+alt+k", "prefix+0", "prefix+shift+q", "cmd+k"} {
		if got := c.Conflicts(free, fresh); len(got) != 0 {
			t.Errorf("%s should be free, got %v", free, got)
		}
	}
	d := c.Conflicts("prefix+n", fresh)[0]
	if !strings.Contains(d.Describe(), "default") || !strings.Contains(d.Describe(), "prefix+n") {
		t.Errorf("describe: %q", d.Describe())
	}
}

// --- qué entradas se pueden enlazar ---

func entryByID(t *testing.T, id string) palette.Entry {
	t.Helper()
	entries, err := palette.Native()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("entry %q not in the catalog", id)
	return palette.Entry{}
}

func TestSpecForPluginActionsUsesTheQualifiedID(t *testing.T) {
	b, ok := SpecFor(palette.Entry{Kind: palette.KindPlugin, ID: "persiyanov.reviewr.toggle"}, "windows")
	if !ok || b != (Binding{TypePluginAction, "persiyanov.reviewr.toggle"}) {
		t.Errorf("got %v %v", b, ok)
	}
	if _, ok := SpecFor(palette.Entry{Kind: palette.KindPlugin}, "linux"); ok {
		t.Error("an action without an id cannot be bound")
	}
}

func TestSpecForNativeCommandsIsAShellCommandInTheSystemsSyntax(t *testing.T) {
	for _, tt := range []struct {
		id, goos, want string
	}{
		{"config.reload", "windows", `"%HERDR_BIN_PATH%" server reload-config`},
		{"config.reload", "linux", `"$HERDR_BIN_PATH" server reload-config`},
		{"tab.close", "windows", `"%HERDR_BIN_PATH%" tab close %HERDR_ACTIVE_TAB_ID%`},
		{"tab.close", "darwin", `"$HERDR_BIN_PATH" tab close "$HERDR_ACTIVE_TAB_ID"`},
		{"pane.close", "windows", `"%HERDR_BIN_PATH%" pane close %HERDR_ACTIVE_PANE_ID%`},
		{"pane.move.new_tab", "linux", `"$HERDR_BIN_PATH" pane move "$HERDR_ACTIVE_PANE_ID" --new-tab`},
	} {
		b, ok := SpecFor(entryByID(t, tt.id), tt.goos)
		if !ok || b.Type != TypeShell || b.Command != tt.want {
			t.Errorf("%s on %s = %v %v, want shell %q", tt.id, tt.goos, b, ok, tt.want)
		}
	}
}

func TestEntriesThatAskOpenThePaletteOnThemInsteadOfRunningBlind(t *testing.T) {
	for _, tt := range []struct {
		id, goos, want string
	}{
		{"agent.focus", "windows", `"%HERDR_BIN_PATH%" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=agent.focus`},
		{"agent.focus", "linux", `"$HERDR_BIN_PATH" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=agent.focus`},
		{"workspace.new", "windows", `"%HERDR_BIN_PATH%" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=workspace.new`},
		{"tab.next", "darwin", `"$HERDR_BIN_PATH" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=tab.next`},
		{"hum.agent.say", "windows", `"%HERDR_BIN_PATH%" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=hum.agent.say`},
		{"machine.enable", "linux", `"$HERDR_BIN_PATH" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=machine.enable`},
	} {
		b, how, ok := SpecHow(entryByID(t, tt.id), tt.goos)
		if !ok || how != HowOpenPalette || b.Type != TypeShell || b.Command != tt.want {
			t.Errorf("%s on %s = %v how=%v ok=%v, want shell %q", tt.id, tt.goos, b, how, ok, tt.want)
		}
	}
}

func TestHowAnEntryRunsDependsOnWhetherItNeedsToAsk(t *testing.T) {
	for id, want := range map[string]How{
		"tab.close":     HowDirect,      // un id que herdr ya da: se ejecuta tal cual
		"config.reload": HowDirect,      // sin argumentos
		"agent.focus":   HowOpenPalette, // elige un agente
		"worktree.new":  HowOpenPalette, // pide una rama
		"hum.agent.new": HowOpenPalette, // comando de hum
	} {
		if _, how, ok := SpecHow(entryByID(t, id), "windows"); !ok || how != want {
			t.Errorf("%s: how = %v ok=%v, want %v", id, how, ok, want)
		}
	}
	if _, how, _ := SpecHow(palette.Entry{Kind: palette.KindPlugin, ID: "a.b"}, "linux"); how != HowPluginAction {
		t.Errorf("a plugin action is invoked by herdr, got %v", how)
	}
}

func TestSpecForStillRefusesWhatCannotBeWrittenSafely(t *testing.T) {
	// El id es lo único que se escribe en el comando que abre la paleta.
	for _, e := range []palette.Entry{
		{Kind: palette.KindNative, ID: "a b; rm", Command: []string{"x y"}},
		{Kind: palette.KindHum, ID: `x"y`, Command: []string{"new"}},
		{Kind: palette.KindNative, ID: "", Command: []string{"x y"}},
		{Kind: palette.KindPlugin},
		{Kind: palette.Kind(99), ID: "ok.id"},
	} {
		if b, ok := SpecFor(e, "linux"); ok {
			t.Errorf("%+v must not be bindable, got %v", e, b)
		}
	}
	// Un argumento literal inseguro ya no impide enlazarla: la entrada se abre en la
	// paleta, y el argumento no se escribe en el comando.
	unsafe := palette.Entry{Kind: palette.KindNative, ID: "weird.cmd", Command: []string{"x"}, Arguments: []palette.Argument{{Source: palette.SourceLiteral, Value: "a b; rm"}}}
	b, how, ok := SpecHow(unsafe, "linux")
	if !ok || how != HowOpenPalette || strings.Contains(b.Command, "rm") {
		t.Errorf("an unsafe literal never reaches the shell command: %v %v %v", b, how, ok)
	}
}

func TestEveryCatalogEntryCanNowBeBoundAndNeverCarriesShellMetacharacters(t *testing.T) {
	entries, _ := palette.Native()
	direct, open := 0, 0
	for _, e := range entries {
		for _, goos := range []string{"windows", "linux"} {
			b, how, ok := SpecHow(e, goos)
			if !ok {
				t.Errorf("%s (%s) must be bindable: any entry can open the palette on itself", e.ID, goos)
				continue
			}
			if b.Type != TypeShell || strings.ContainsAny(b.Command, ";&|<>`\n\r") {
				t.Errorf("%s: unexpected spec %v", e.ID, b)
			}
			if goos == "windows" {
				switch how {
				case HowDirect:
					direct++
					if e.Kind != palette.KindNative {
						t.Errorf("%s: only herdr commands run directly", e.ID)
					}
				case HowOpenPalette:
					open++
					if !strings.HasSuffix(b.Command, "HUM_PALETTE_ENTRY="+e.ID) {
						t.Errorf("%s: the command must end with its own id: %s", e.ID, b.Command)
					}
				}
			}
		}
	}
	if direct < 15 || open < 15 {
		t.Errorf("both ways matter: %d direct, %d open the palette", direct, open)
	}
	if direct+open != len(entries) {
		t.Errorf("%d + %d does not add up to the %d catalog entries", direct, open, len(entries))
	}
}

func TestChordsFindsThePluginAndTheShellBindings(t *testing.T) {
	text := sample + "\n[[keys.command]]\nkey = \"f9\"\ntype = \"shell\"\ncommand = \"\\\"%HERDR_BIN_PATH%\\\" server reload-config\"\n"
	entries := []palette.Entry{
		{Kind: palette.KindPlugin, ID: "herdr-human.palette"},
		entryByID(t, "config.reload"),
		entryByID(t, "tab.close"),
	}
	got := Chords(entries, Parse(text), "windows")
	if got["herdr-human.palette"] != "f1" || got["config.reload"] != "f9" || got["tab.close"] != "" {
		t.Errorf("got %v", got)
	}
}
