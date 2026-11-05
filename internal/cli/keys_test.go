package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

const keysConfig = `# my config

[keys.indexed]
workspaces = "alt"

[[keys.command]]
key = "f1"
type = "plugin_action"
command = "herdr-human.palette"
description = "command palette"
`

type keysEnv struct {
	path   string
	fake   *herdrtest.Fake
	checks int
	// verdict devuelve lo que dice `herdr config check` de un texto.
	verdict func(text string) string
}

func newKeysEnv(t *testing.T, initial string) *keysEnv {
	t.Helper()
	k := &keysEnv{path: filepath.Join(t.TempDir(), "config.toml"), fake: herdrtest.New()}
	if initial != "" {
		if err := os.WriteFile(k.path, []byte(initial), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	k.fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	k.fake.Reply("", []string{"server", "reload-config"}, okReply)
	return k
}

func (k *keysEnv) run(t *testing.T, goos string, args ...string) output {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Runner: k.fake, Stdout: &stdout, Stderr: &stderr, GOOS: goos,
		Getenv: envWith(map[string]string{"HERDR_CONFIG_PATH": k.path}),
		CheckConfig: func(_ context.Context, path string) (string, error) {
			k.checks++
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			if k.verdict != nil {
				return k.verdict(string(data)), nil
			}
			return "config: ok\n", nil
		},
	}, append([]string{"keys"}, args...))
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (k *keysEnv) text(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(k.path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (k *keysEnv) reloads() int {
	n := 0
	for _, c := range k.fake.Calls() {
		if len(c.Args) >= 2 && c.Args[0] == "server" && c.Args[1] == "reload-config" {
			n++
		}
	}
	return n
}

func TestKeysLsShowsTheBoundEntriesAndAllWithAll(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	out := k.run(t, "windows", "ls")
	if out.code != 0 || !strings.Contains(out.stdout, "f1") || !strings.Contains(out.stdout, "herdr-human.palette") {
		t.Fatalf("%+v", out)
	}
	if strings.Contains(out.stdout, "config.reload") {
		t.Errorf("without --all only the entries that have a shortcut:\n%s", out.stdout)
	}
	all := k.run(t, "windows", "ls", "--all")
	for _, want := range []string{"config.reload", "tab.close", "x.plug.ping", "Ping the thing", "-"} {
		if !strings.Contains(all.stdout, want) {
			t.Errorf("--all lists everything that can be bound; lacks %q:\n%s", want, all.stdout)
		}
	}
	for _, ask := range []string{"agent.focus", "workspace.new", "hum.agent.say", "tab.next"} {
		if !strings.Contains(all.stdout, ask) {
			t.Errorf("entries that ask something are bindable too (the shortcut opens the palette on them); lacks %q:\n%s", ask, all.stdout)
		}
	}
}

func TestKeysSetBindsAnEntryThatAsksByOpeningThePaletteOnIt(t *testing.T) {
	for goos, want := range map[string]string{
		"windows": `command = "\"%HERDR_BIN_PATH%\" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=agent.focus"`,
		"linux":   `command = "\"$HERDR_BIN_PATH\" plugin pane open --plugin herdr-human --entrypoint palette --focus --env HUM_PALETTE_ENTRY=agent.focus"`,
	} {
		k := newKeysEnv(t, keysConfig)
		if out := k.run(t, goos, "set", "agent.focus", "ctrl+alt+f"); out.code != 0 {
			t.Fatalf("%s: %+v", goos, out)
		}
		got := k.text(t)
		if !strings.Contains(got, `type = "shell"`) || !strings.Contains(got, want) || !strings.Contains(got, `description = "Agent: Focus…"`) {
			t.Errorf("%s: want %s in:\n%s", goos, want, got)
		}
		if ls := k.run(t, goos, "ls"); !strings.Contains(ls.stdout, "ctrl+alt+f") || !strings.Contains(ls.stdout, "agent.focus") {
			t.Errorf("%s: the binding reads back as that entry's shortcut:\n%s", goos, ls.stdout)
		}
		if rm := k.run(t, goos, "rm", "agent.focus"); rm.code != 0 || strings.Contains(k.text(t), "agent.focus") {
			t.Errorf("%s: it can be removed again: %+v", goos, rm)
		}
	}
}

func TestKeysLsWithNothingBoundSaysHowToSeeWhatCanBe(t *testing.T) {
	out := newKeysEnv(t, "").run(t, "linux", "ls")
	if out.code != 0 || !strings.Contains(out.stdout, "--all") {
		t.Errorf("%+v", out)
	}
}

func TestKeysSetBindsAPluginActionAndReloads(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	out := k.run(t, "windows", "set", "x.plug.ping", "Alt+Ctrl+K")
	if out.code != 0 || !strings.Contains(out.stdout, "ctrl+alt+k -> Ping the thing (x.plug.ping)") {
		t.Fatalf("%+v", out)
	}
	want := keysConfig + "\n[[keys.command]]\nkey = \"ctrl+alt+k\"\ntype = \"plugin_action\"\ncommand = \"x.plug.ping\"\ndescription = \"Ping the thing\"\n"
	if got := k.text(t); got != want {
		t.Errorf("the file got exactly one new block:\n%s", got)
	}
	if k.reloads() != 1 || k.checks != 2 {
		t.Errorf("one check of the old file, one of the new, one reload: checks=%d reloads=%d", k.checks, k.reloads())
	}
	if _, err := os.Stat(k.path + ".bak-hum-" + todayStamp()); err != nil {
		t.Errorf("the backup exists: %v", err)
	}
}

func TestKeysSetBindsAHerdrCommandInTheShellSyntaxOfTheSystem(t *testing.T) {
	for goos, want := range map[string]string{
		"windows": `command = "\"%HERDR_BIN_PATH%\" tab close %HERDR_ACTIVE_TAB_ID%"`,
		"linux":   `command = "\"$HERDR_BIN_PATH\" tab close \"$HERDR_ACTIVE_TAB_ID\""`,
	} {
		k := newKeysEnv(t, keysConfig)
		if out := k.run(t, goos, "set", "tab.close", "ctrl+alt+w"); out.code != 0 {
			t.Fatalf("%s: %+v", goos, out)
		}
		if got := k.text(t); !strings.Contains(got, `type = "shell"`) || !strings.Contains(got, want) {
			t.Errorf("%s: want %s in:\n%s", goos, want, got)
		}
	}
}

func TestKeysSetChangesTheKeyOfAnAlreadyBoundEntryInPlace(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	if out := k.run(t, "windows", "set", "herdr-human.palette", "f3"); out.code != 0 {
		t.Fatalf("%+v", out)
	}
	if got := k.text(t); got != strings.Replace(keysConfig, `key = "f1"`, `key = "f3"`, 1) {
		t.Errorf("only the key changes:\n%s", got)
	}
}

func TestKeysSetRefusesAKeyAnotherEntryAlreadyUses(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	out := k.run(t, "windows", "set", "x.plug.ping", "f1")
	if out.code != 1 || !strings.Contains(out.stderr, "Open palette already uses f1") || !strings.Contains(out.stderr, "free it first") {
		t.Errorf("the conflict is named by title: %+v", out)
	}
	if k.text(t) != keysConfig || k.reloads() != 0 {
		t.Error("nothing was changed")
	}
}

func TestKeysSetRefusesTheModifierHerdrAlreadyUsesForIndexedKeys(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	if out := k.run(t, "windows", "set", "x.plug.ping", "alt+2"); out.code != 1 || !strings.Contains(out.stderr, "keys.indexed.workspaces") {
		t.Errorf("%+v", out)
	}
}

func TestKeysSetWarnsAboutHerdrsDefaultsAndForceBindsAnyway(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	out := k.run(t, "windows", "set", "x.plug.ping", "prefix+n")
	if out.code != 1 || !strings.Contains(out.stderr, "herdr's default") || !strings.Contains(out.stderr, "--force") || k.text(t) != keysConfig {
		t.Fatalf("a default is a warning that needs --force: %+v", out)
	}
	if forced := k.run(t, "windows", "set", "x.plug.ping", "prefix+n", "--force"); forced.code != 0 || !strings.Contains(k.text(t), `key = "prefix+n"`) {
		t.Errorf("%+v", forced)
	}
}

func TestKeysSetRefusesBadKeysAndPlainKeysWithoutTouchingTheFile(t *testing.T) {
	for _, key := range []string{"wat+q", "ctrl+", "a", "?"} {
		k := newKeysEnv(t, keysConfig)
		out := k.run(t, "windows", "set", "x.plug.ping", key)
		if out.code != 1 || k.text(t) != keysConfig || k.checks != 0 {
			t.Errorf("%q: %+v checks=%d", key, out, k.checks)
		}
	}
}

func TestKeysSetDoesNotApplyWhatHerdrRejects(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	k.verdict = func(text string) string {
		if strings.Contains(text, `"ctrl+alt+k"`) {
			return "config: issues found\ninvalid keybinding: keys.command[1].key = \"ctrl+alt+k\"; disabling binding\n"
		}
		return "config: ok\n"
	}
	out := k.run(t, "windows", "set", "x.plug.ping", "ctrl+alt+k")
	if out.code != 1 || !strings.Contains(out.stderr, "nothing was changed") || !strings.Contains(out.stderr, "invalid keybinding") {
		t.Errorf("%+v", out)
	}
	if k.text(t) != keysConfig || k.reloads() != 0 {
		t.Error("a rejected change is never written")
	}
}

func TestKeysSetReportsAFailedReloadButKeepsTheFile(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	k.fake.Fail("", []string{"server", "reload-config"}, os.ErrClosed)
	out := k.run(t, "windows", "set", "x.plug.ping", "ctrl+alt+k")
	if out.code != 0 || !strings.Contains(out.stderr, "herdr server reload-config") || !strings.Contains(k.text(t), "ctrl+alt+k") {
		t.Errorf("saved, and it says how to reload: %+v", out)
	}
}

func TestKeysSetUnknownEntrySuggestsSimilarOnes(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	out := k.run(t, "windows", "set", "tab.cl", "ctrl+alt+k")
	if out.code != 1 || !strings.Contains(out.stderr, "tab.close") {
		t.Errorf("a partial id suggests the full one: %+v", out)
	}
	if out := k.run(t, "windows", "set", "nope", "ctrl+alt+k"); out.code != 1 || !strings.Contains(out.stderr, "keys ls --all") {
		t.Errorf("%+v", out)
	}
	if k.text(t) != keysConfig {
		t.Error("nothing was changed")
	}
}

func TestKeysRmRemovesTheShortcut(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	out := k.run(t, "windows", "rm", "herdr-human.palette")
	if out.code != 0 || strings.Contains(k.text(t), "herdr-human.palette") || k.reloads() != 1 {
		t.Fatalf("%+v\n%s", out, k.text(t))
	}
	if again := k.run(t, "windows", "rm", "herdr-human.palette"); again.code != 1 || !strings.Contains(again.stderr, "has no shortcut") {
		t.Errorf("%+v", again)
	}
}

func TestKeysUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"nope"}, {"set"}, {"set", "a"}, {"set", "a", "b", "c"}, {"rm"}, {"rm", "a", "b"}, {"ls", "extra"}} {
		if out := newKeysEnv(t, "").run(t, "windows", args...); out.code != 2 {
			t.Errorf("%v: want usage error (2), got %+v", args, out)
		}
	}
}

func todayStamp() string { return time.Now().Format("20060102") }

func TestKeysSetMessagesNameTheKeyOnlyOnce(t *testing.T) {
	k := newKeysEnv(t, keysConfig)
	for key, want := range map[string]string{
		"wat+q": `hum: "wat+q": "wat" is not a modifier`,
		"a":     "hum: a is a plain key: it would swallow what you type",
		"f1":    "hum: Open palette already uses f1",
	} {
		out := k.run(t, "windows", "set", "x.plug.ping", key)
		if !strings.Contains(out.stderr, want) || strings.Count(out.stderr, "hum: ") != 1 {
			t.Errorf("%q: want %q once, got %q", key, want, out.stderr)
		}
	}
}
