package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
)

func aliasEnv(t *testing.T) cli.Env {
	t.Helper()
	return cli.Env{ConfigDir: filepath.Join(t.TempDir(), "conf")}
}

func TestAliasSetListAndRemove(t *testing.T) {
	t.Parallel()

	env := aliasEnv(t)
	fake := fixture()
	if out := runEnv(t, fake, env, "alias", "ls"); out.code != 0 || out.stdout != "" || out.stderr != "" {
		t.Errorf("empty ls: code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
	for _, args := range [][]string{{"alias", "set", "docs", "odoous/configura"}, {"alias", "set", "api", "importer"}} {
		if out := runEnv(t, fake, env, args...); out.code != 0 || !strings.Contains(out.stdout, args[2]) {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, out.code, out.stdout, out.stderr)
		}
	}
	out := runEnv(t, fake, env, "alias", "ls")
	if out.stdout != "api -> importer\ndocs -> odoous/configura\n" {
		t.Errorf("ls = %q, want both aliases sorted by name", out.stdout)
	}
	if out := runEnv(t, fake, env, "alias", "rm", "docs"); out.code != 0 {
		t.Errorf("rm: code=%d stderr=%q", out.code, out.stderr)
	}
	if out := runEnv(t, fake, env, "alias", "ls"); out.stdout != "api -> importer\n" {
		t.Errorf("after rm: %q", out.stdout)
	}
	if out := runEnv(t, fake, env, "alias", "rm", "docs"); out.code != 1 || !strings.Contains(out.stderr, "not found") {
		t.Errorf("removing it again: code=%d stderr=%q", out.code, out.stderr)
	}
	if len(fake.Calls()) != 0 {
		t.Errorf("managing aliases must not call herdr: %v", fake.Calls())
	}
}

func TestAliasRejectsBadInput(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"uppercase name": {"alias", "set", "Docs", "importer"},
		"dash name":      {"alias", "set", "--", "-x", "importer"},
		"dash target":    {"alias", "set", "docs", "--", "-x"},
		"control target": {"alias", "set", "docs", "a\x1b[0m"},
	} {
		env := aliasEnv(t)
		out := runEnv(t, fixture(), env, args...)
		if out.code == 0 {
			t.Errorf("%s: accepted", name)
		}
		if got := runEnv(t, fixture(), env, "alias", "ls"); got.stdout != "" {
			t.Errorf("%s: something was saved: %q", name, got.stdout)
		}
	}
}

func TestAliasUsageErrors(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"no verb":       {"alias"},
		"unknown verb":  {"alias", "frobnicate"},
		"set no target": {"alias", "set", "docs"},
		"set extra":     {"alias", "set", "docs", "a", "b"},
		"rm no name":    {"alias", "rm"},
		"ls extra":      {"alias", "ls", "x"},
	} {
		out := runEnv(t, fixture(), aliasEnv(t), args...)
		if out.code != 2 || !strings.Contains(out.stderr, "usage: hum alias") {
			t.Errorf("%s: code=%d stderr=%q, want a usage error", name, out.code, out.stderr)
		}
	}
}

func TestAliasNeedsAConfigurationFolderToSave(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "alias", "set", "docs", "importer")
	if out.code != 1 || !strings.Contains(out.stderr, "configuration") {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestAnAliasResolvesInEveryCommandThatTakesAWho(t *testing.T) {
	t.Parallel()

	env := aliasEnv(t)
	if out := runEnv(t, fixture(), env, "alias", "set", "docs", "odoous/configura"); out.code != 0 {
		t.Fatal(out.stderr)
	}
	out := runEnv(t, fixture(), env, "id", "docs")
	if out.code != 0 || !strings.Contains(out.stdout, "w1:p2") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestTheAliasWinsOverAnyOtherMatch(t *testing.T) {
	t.Parallel()

	env := aliasEnv(t)
	// "codex" es un tipo de agente que existe en dos máquinas: sin alias es ambiguo.
	if out := run(t, fixture(), "id", "codex"); out.code == 0 {
		t.Fatal("the fixture should make \"codex\" ambiguous")
	}
	if out := runEnv(t, fixture(), env, "alias", "set", "codex", "importer"); out.code != 0 {
		t.Fatal(out.stderr)
	}
	out := runEnv(t, fixture(), env, "id", "codex")
	if out.code != 0 || out.stdout != "wS:p1\n" {
		t.Errorf("code=%d stdout=%q stderr=%q, want the aliased agent", out.code, out.stdout, out.stderr)
	}
}

func TestADeadAliasSaysWhichAliasFailed(t *testing.T) {
	t.Parallel()

	env := aliasEnv(t)
	if out := runEnv(t, fixture(), env, "alias", "set", "docs", "zzz-nothing"); out.code != 0 {
		t.Fatal(out.stderr)
	}
	out := runEnv(t, fixture(), env, "id", "docs")
	if out.code != 1 || !strings.Contains(out.stderr, `alias "docs"`) || !strings.Contains(out.stderr, "zzz-nothing") {
		t.Errorf("code=%d stderr=%q, want the alias named in the error", out.code, out.stderr)
	}
}

func TestAnAliasIsNotFollowedTwice(t *testing.T) {
	t.Parallel()

	env := aliasEnv(t)
	runEnv(t, fixture(), env, "alias", "set", "a", "b")
	runEnv(t, fixture(), env, "alias", "set", "b", "importer")
	if out := runEnv(t, fixture(), env, "id", "a"); out.code == 0 {
		t.Errorf("an alias chain was followed: %q", out.stdout)
	}
}

func TestACorruptAliasFileNeverBreaksNormalResolution(t *testing.T) {
	t.Parallel()

	env := aliasEnv(t)
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.ConfigDir, "aliases.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := runEnv(t, fixture(), env, "id", "importer"); out.code != 0 || out.stdout != "wS:p1\n" {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
	if out := runEnv(t, fixture(), env, "alias", "ls"); out.code != 1 || !strings.Contains(out.stderr, "aliases.json") {
		t.Errorf("alias ls must report the corrupt file: code=%d stderr=%q", out.code, out.stderr)
	}
}
