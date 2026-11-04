package complete_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/complete"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

// fakeSource cuenta cuántas veces se le pidieron datos, para comprobar que solo
// se consultan cuando hacen falta.
type fakeSource struct {
	targets  []inventory.Target
	machines []string
	aliases  map[string]string
	askedT   int
	askedM   int
	askedA   int
}

func (f *fakeSource) Targets() []inventory.Target { f.askedT++; return f.targets }
func (f *fakeSource) Machines() []string          { f.askedM++; return f.machines }
func (f *fakeSource) Aliases() map[string]string  { f.askedA++; return f.aliases }

func TestAliasesAreOfferedWhereAnAgentIsExpected(t *testing.T) {
	t.Parallel()

	src := sample()
	src.aliases = map[string]string{"docs": "odoous/docs-agent", "api": "importer"}

	r := complete.Complete("hum say d", src)
	if !has(values(r), "docs") {
		t.Errorf("say: %v", values(r))
	}
	for _, c := range r.Candidates {
		if c.Value == "docs" && !strings.Contains(c.Desc, "odoous/docs-agent") {
			t.Errorf("the description should say where the alias points: %q", c.Desc)
		}
	}
	if r := complete.Complete("hum alias rm ", src); strings.Join(values(r), ",") != "api,docs" {
		t.Errorf("alias rm: %v, want the alias names sorted", values(r))
	}
	if r := complete.Complete("hum alias ", src); strings.Join(values(r), ",") != "set,rm,ls" {
		t.Errorf("alias verbs: %v", values(r))
	}
	if r := complete.Complete("hum alias set docs ", src); !has(values(r), "importer") {
		t.Errorf("the target of an alias is an agent: %v", values(r))
	}
	if r := complete.Complete("hum alias set ", src); len(r.Candidates) != 0 {
		t.Errorf("a new alias name is free text: %v", values(r))
	}
}

func TestAliasesAreOnlyAskedWhenNeeded(t *testing.T) {
	t.Parallel()

	src := sample()
	complete.Complete("hum ls --fr", src)
	complete.Complete("hum wait ", src)
	if src.askedA != 1 {
		t.Errorf("aliases asked %d times, want only for the agent position", src.askedA)
	}
}

func sample() *fakeSource {
	return &fakeSource{
		machines: []string{"odoous", "WSL Ubuntu"},
		targets: []inventory.Target{
			{PaneID: "wS:p1", Agent: "codex", Name: "importer", Status: "idle"},
			{PaneID: "wS:p4", Agent: "claude", Name: "Pane ID work", Status: "working"},
			{Machine: "odoous", PaneID: "w1:p2", Agent: "codex", Name: "docs-agent", Status: "blocked"},
			{Machine: "odoous", PaneID: "w1:p3", Agent: "claude", Name: "Configura Odoo 18 | odoo-inc | 5h", Status: "done"},
		},
	}
}

func values(r complete.Result) []string {
	out := make([]string, len(r.Candidates))
	for i, c := range r.Candidates {
		out[i] = c.Value
	}
	return out
}

func has(vs []string, want string) bool {
	for _, v := range vs {
		if v == want {
			return true
		}
	}
	return false
}

func TestTokenize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		line string
		want []string
	}{
		{"hum", []string{"hum"}},
		{"hum ", []string{"hum", ""}},
		{"hum ls --co", []string{"hum", "ls", "--co"}},
		{"hum  ls\t--json ", []string{"hum", "ls", "--json", ""}},
		{`hum say "two words" 'and more'`, []string{"hum", "say", "two words", "and more"}},
		{`hum say "unterminated`, []string{"hum", "say", "unterminated"}},
		{`hum new claude C:\Users\me\proj`, []string{"hum", "new", "claude", `C:\Users\me\proj`}},
		{"", []string{""}},
	}
	for _, tt := range tests {
		if got := complete.Tokenize(tt.line); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Tokenize(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestSubcommands(t *testing.T) {
	t.Parallel()

	all := values(complete.Complete("hum ", nil))
	for _, want := range []string{"ls", "id", "say", "ask", "attention", "look", "name", "rename", "close", "worktree", "alias", "go", "wait", "pick", "new", "machines", "completion", "help"} {
		if !has(all, want) {
			t.Errorf("subcommands lack %q: %v", want, all)
		}
	}
	if has(all, "__complete") {
		t.Errorf("the hidden command must not be suggested")
	}
	if got := values(complete.Complete("hum a", nil)); !reflect.DeepEqual(got, []string{"ask", "attention", "alias"}) {
		t.Errorf("prefix a = %v", got)
	}
	if got := values(complete.Complete("hum PI", nil)); !reflect.DeepEqual(got, []string{"pick"}) {
		t.Errorf("matching ignores case: %v", got)
	}
	if got := complete.Complete("hum zzz", nil); len(got.Candidates) != 0 {
		t.Errorf("nothing matches zzz: %v", got)
	}
}

func TestFlags(t *testing.T) {
	t.Parallel()

	got := values(complete.Complete("hum ls --", nil))
	for _, want := range []string{"--json", "--local", "--machine", "--fresh", "--color", "--full-names", "--verbose", "--server-timeout"} {
		if !has(got, want) {
			t.Errorf("ls flags lack %q: %v", want, got)
		}
	}
	if got := values(complete.Complete("hum ls --co", nil)); !reflect.DeepEqual(got, []string{"--color"}) {
		t.Errorf("ls --co = %v", got)
	}
	if got := values(complete.Complete("hum attention --w", nil)); !reflect.DeepEqual(got, []string{"--watch"}) {
		t.Errorf("attention --w = %v", got)
	}
	if has(values(complete.Complete("hum attention --", nil)), "--fresh") {
		t.Errorf("attention is always live and has no --fresh")
	}
	if got := values(complete.Complete("hum new --", nil)); !has(got, "--no-focus") || !has(got, "--name") {
		t.Errorf("new flags = %v", got)
	}
	if got := complete.Complete("hum frobnicate --", nil); len(got.Candidates) != 0 {
		t.Errorf("an unknown command has no flags: %v", got)
	}
}

func TestFlagValues(t *testing.T) {
	t.Parallel()

	src := sample()
	if got := values(complete.Complete("hum ls --color ", src)); !reflect.DeepEqual(got, []string{"auto", "always", "never"}) {
		t.Errorf("--color = %v", got)
	}
	if got := values(complete.Complete("hum ls --color=al", src)); !reflect.DeepEqual(got, []string{"--color=always"}) {
		t.Errorf("--color=al = %v", got)
	}
	if got := values(complete.Complete("hum wait x --until ", src)); !reflect.DeepEqual(got, []string{"idle", "working", "blocked", "done", "unknown"}) {
		t.Errorf("--until = %v", got)
	}
	m := values(complete.Complete("hum ls --machine ", src))
	if !has(m, "local") || !has(m, "odoous") || !has(m, "WSL Ubuntu") {
		t.Errorf("--machine = %v", m)
	}
	if got := values(complete.Complete("hum ls --machine o", src)); !reflect.DeepEqual(got, []string{"odoous"}) {
		t.Errorf("--machine o = %v", got)
	}
	if got := complete.Complete("hum wait x --timeout ", src); len(got.Candidates) != 0 || got.Files {
		t.Errorf("a numeric flag has nothing to suggest and no files: %+v", got)
	}
}

func TestWhoCandidatesAreSafeNamesAndRefs(t *testing.T) {
	t.Parallel()

	for _, cmd := range []string{"id", "say", "ask", "look", "name", "go", "wait"} {
		got := values(complete.Complete("hum "+cmd+" ", sample()))
		for _, want := range []string{"importer", "local/wS:p1", "local/wS:p4", "odoous/w1:p2", "docs-agent", "odoous/docs-agent", "odoous/w1:p3"} {
			if !has(got, want) {
				t.Errorf("%s: candidates lack %q: %v", cmd, want, got)
			}
		}
		for _, bad := range got {
			if strings.ContainsAny(bad, " |") && !strings.HasPrefix(bad, "WSL") {
				t.Errorf("%s: %q is a free-form title; only names hum accepts and refs may be offered", cmd, bad)
			}
		}
	}
}

func TestWhoCandidatesFilterByPrefixAndCarryADescription(t *testing.T) {
	t.Parallel()

	r := complete.Complete("hum say odoous/", sample())
	if got := values(r); !reflect.DeepEqual(got, []string{"odoous/w1:p2", "odoous/docs-agent", "odoous/w1:p3"}) {
		t.Errorf("odoous/ = %v", got)
	}
	var found bool
	for _, c := range r.Candidates {
		if c.Value == "odoous/w1:p2" {
			found = true
			for _, want := range []string{"codex", "blocked", "docs-agent"} {
				if !strings.Contains(c.Desc, want) {
					t.Errorf("description %q lacks %q", c.Desc, want)
				}
			}
		}
	}
	if !found {
		t.Fatal("no candidate for odoous/w1:p2")
	}
	if got := values(complete.Complete("hum id imp", sample())); !reflect.DeepEqual(got, []string{"importer"}) {
		t.Errorf("imp = %v", got)
	}
}

func TestOnlyTheFirstPositionTakesAnAgent(t *testing.T) {
	t.Parallel()

	for _, line := range []string{"hum say importer ", "hum ask importer hello ", "hum name importer ", "hum say --wait importer "} {
		if got := complete.Complete(line, sample()); len(got.Candidates) != 0 || got.Files {
			t.Errorf("%q: the message or the new name is free text, got %+v", line, got)
		}
	}
	// Las opciones con valor no cuentan como posicionales.
	if got := values(complete.Complete("hum say --timeout 5000 ", sample())); !has(got, "importer") {
		t.Errorf("a flag value must not count as the agent argument: %v", got)
	}
	if got := values(complete.Complete("hum ls --machine odoous ", sample())); len(got) != 0 {
		t.Errorf("ls takes no arguments: %v", got)
	}
}

func TestNewCompletesKindsMachinePrefixesAndFolders(t *testing.T) {
	t.Parallel()

	kinds := values(complete.Complete("hum new ", nil))
	if !has(kinds, "claude") || !has(kinds, "codex") || len(kinds) < 20 {
		t.Errorf("kinds = %v", kinds)
	}
	if got := values(complete.Complete("hum new cl", nil)); !reflect.DeepEqual(got, []string{"claude", "cline"}) {
		t.Errorf("cl = %v", got)
	}
	if got := values(complete.Complete("hum new odoous/cod", nil)); !reflect.DeepEqual(got, []string{"odoous/codex"}) {
		t.Errorf("odoous/cod = %v", got)
	}
	if got := complete.Complete("hum new claude ", nil); !got.Files || len(got.Candidates) != 0 {
		t.Errorf("the folder is a file path: %+v", got)
	}
	if got := complete.Complete("hum new claude --name x ", nil); !got.Files {
		t.Errorf("the folder after other flags: %+v", got)
	}
}

func TestMachinesAndCompletionSubcommands(t *testing.T) {
	t.Parallel()

	if got := values(complete.Complete("hum machines ", sample())); !reflect.DeepEqual(got, []string{"enable", "disable", "toggle", "menu"}) {
		t.Errorf("machines = %v", got)
	}
	if got := values(complete.Complete("hum machines enable ", sample())); !reflect.DeepEqual(got, []string{"odoous", "WSL Ubuntu"}) {
		t.Errorf("machines enable = %v", got)
	}
	if got := complete.Complete("hum machines menu ", sample()); len(got.Candidates) != 0 {
		t.Errorf("menu takes nothing: %v", got)
	}
	if got := values(complete.Complete("hum completion ", nil)); !reflect.DeepEqual(got, []string{"bash", "zsh", "powershell"}) {
		t.Errorf("completion = %v", got)
	}
	if got := complete.Complete("hum help ", nil); len(got.Candidates) != 0 {
		t.Errorf("help takes nothing: %v", got)
	}
}

func TestDataIsOnlyRequestedWhenNeeded(t *testing.T) {
	t.Parallel()

	for _, line := range []string{"hum ", "hum ls --", "hum ls --color ", "hum new ", "hum new claude ", "hum completion ", "hum say importer "} {
		src := sample()
		complete.Complete(line, src)
		if src.askedT != 0 || src.askedM != 0 {
			t.Errorf("%q asked the source (targets %d, machines %d) although it does not need it", line, src.askedT, src.askedM)
		}
	}
	src := sample()
	complete.Complete("hum say ", src)
	if src.askedT != 1 || src.askedM != 0 {
		t.Errorf("an agent position needs the agents only: %d %d", src.askedT, src.askedM)
	}
	src = sample()
	complete.Complete("hum ls --machine ", src)
	if src.askedM != 1 || src.askedT != 0 {
		t.Errorf("--machine needs the machines only: %d %d", src.askedT, src.askedM)
	}
}

func TestValuesFromTheSourceAreSanitised(t *testing.T) {
	t.Parallel()

	src := &fakeSource{
		machines: []string{"ok", "evil\x1b[2Jmachine", "line\nbreak"},
		targets: []inventory.Target{
			{Machine: "evil\x1b[2Jmachine", PaneID: "w1:p1", Agent: "codex", Status: "idle", Name: "n"},
			{PaneID: "w1:p2", Agent: "co\x07dex", Status: "id\tle", Name: "docs-agent"},
		},
	}
	all := append(values(complete.Complete("hum say ", src)), values(complete.Complete("hum ls --machine ", src))...)
	for _, v := range all {
		if strings.ContainsAny(v, "\x1b\x07\n\r\t") {
			t.Errorf("a control character reached the completions: %q", v)
		}
	}
	if has(all, "ok") == false {
		t.Errorf("harmless values must survive: %v", all)
	}
	for _, c := range complete.Complete("hum say ", src).Candidates {
		if strings.ContainsAny(c.Desc, "\x1b\x07\n\r\t") {
			t.Errorf("a control character reached a description: %q", c.Desc)
		}
	}
}

func TestDuplicatesAreRemovedAndOrderIsStable(t *testing.T) {
	t.Parallel()

	src := &fakeSource{targets: []inventory.Target{
		{PaneID: "w1:p1", Name: "same", Agent: "a", Status: "idle"},
		{PaneID: "w1:p2", Name: "same", Agent: "a", Status: "idle"},
	}}
	got := values(complete.Complete("hum id ", src))
	seen := map[string]bool{}
	for _, v := range got {
		if seen[v] {
			t.Errorf("%q is duplicated: %v", v, got)
		}
		seen[v] = true
	}
	again := values(complete.Complete("hum id ", src))
	if !reflect.DeepEqual(got, again) {
		t.Errorf("order changed between calls: %v vs %v", got, again)
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	r := complete.Result{Candidates: []complete.Candidate{
		{Value: "local/wS:p1", Desc: "codex idle"},
		{Value: "plain"},
	}}
	if got := complete.Format("bash", r); got != "local/wS:p1\nplain\n" {
		t.Errorf("bash = %q", got)
	}
	if got := complete.Format("zsh", r); got != "local/wS\\:p1:codex idle\nplain\n" {
		t.Errorf("zsh = %q (colons in values must be escaped)", got)
	}
	if got := complete.Format("powershell", r); got != "local/wS:p1\tcodex idle\nplain\n" {
		t.Errorf("powershell = %q", got)
	}
	if got := complete.Format("bash", complete.Result{Files: true}); got != "@files\n" {
		t.Errorf("files = %q", got)
	}
	if got := complete.Format("bash", complete.Result{}); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestScripts(t *testing.T) {
	t.Parallel()

	for _, shell := range complete.Shells {
		s, err := complete.Script(shell)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		for _, want := range []string{"__complete", "--shell " + shell, "--line", "@files"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s script lacks %q", shell, want)
			}
		}
	}
	ps, _ := complete.Script("powershell")
	if !strings.Contains(ps, "Register-ArgumentCompleter -Native -CommandName hum") {
		t.Errorf("powershell script must register a native completer")
	}
	// Con un espacio final, el AST de PowerShell no lo incluye: el script debe
	// reponerlo o se completaría la palabra anterior en vez de una nueva.
	if !strings.Contains(ps, "' ' * ($upto - $line.Length)") {
		t.Errorf("powershell script must pad the trailing spaces up to the cursor")
	}
	bash, _ := complete.Script("bash")
	if !strings.Contains(bash, "complete -F _hum_complete hum") {
		t.Errorf("bash script must register the function")
	}
	zsh, _ := complete.Script("zsh")
	if !strings.HasPrefix(zsh, "#compdef hum") || !strings.Contains(zsh, "compdef _hum hum") {
		t.Errorf("zsh script must be a compdef file")
	}
	if _, err := complete.Script("fish"); err == nil {
		t.Errorf("fish is not supported yet and must say so")
	}
}
