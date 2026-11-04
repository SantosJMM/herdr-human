package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

// stepClock avanza solo cuando el test lo pide, para fijar la edad de la caché.
type stepClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *stepClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func cachedEnv(t *testing.T) (cli.Env, *stepClock) {
	t.Helper()
	clk := &stepClock{t: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	return cli.Env{CacheDir: t.TempDir(), Now: clk.now}, clk
}

func remoteCalls(f *herdrtest.Fake, machine string) int {
	n := 0
	for _, c := range f.Calls() {
		if c.Machine == machine {
			n++
		}
	}
	return n
}

func TestLsReusesTheRemoteAnswerAndSaysSo(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	first := runEnv(t, f, env, "ls")
	if first.code != 0 || strings.Contains(first.stdout, "cached") {
		t.Fatalf("first ls: code=%d stdout=%q", first.code, first.stdout)
	}
	live := remoteCalls(f, "odoous")

	clk.advance(3 * time.Second)
	second := runEnv(t, f, env, "ls")
	if remoteCalls(f, "odoous") != live {
		t.Errorf("odoous was queried again within the TTL")
	}
	if !strings.Contains(findLine(t, second.stdout, "odoous"), "cached 3s") {
		t.Errorf("the machine header should say the answer is cached:\n%s", second.stdout)
	}
	if strings.Contains(findLine(t, second.stdout, "Local"), "cached") {
		t.Errorf("the local server is never cached:\n%s", second.stdout)
	}
	if !strings.Contains(second.stdout, "Configura Odoo 18") {
		t.Errorf("the cached agents must still be listed:\n%s", second.stdout)
	}
}

func TestFreshBypassesTheCacheAndRenewsIt(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	runEnv(t, f, env, "ls")
	live := remoteCalls(f, "odoous")

	clk.advance(time.Second)
	out := runEnv(t, f, env, "ls", "--fresh")
	if remoteCalls(f, "odoous") <= live || strings.Contains(out.stdout, "cached") {
		t.Errorf("--fresh must query odoous again (calls %d -> %d)\n%s", live, remoteCalls(f, "odoous"), out.stdout)
	}

	live = remoteCalls(f, "odoous")
	clk.advance(time.Second)
	if out := runEnv(t, f, env, "ls"); remoteCalls(f, "odoous") != live || !strings.Contains(out.stdout, "cached 1s") {
		t.Errorf("the fresh run should have renewed the cache:\n%s", out.stdout)
	}
}

func TestJSONOutputIsTheSameWhetherCachedOrNot(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	live := runEnv(t, f, env, "ls", "--json")
	clk.advance(2 * time.Second)
	cached := runEnv(t, f, env, "ls", "--json")
	if live.stdout != cached.stdout {
		t.Errorf("--json must not change when the answer comes from the cache:\nlive   %s\ncached %s", live.stdout, cached.stdout)
	}
}

func TestResolvingANameUsesTheCache(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	if out := runEnv(t, f, env, "id", "odoous/configura"); out.code != 0 {
		t.Fatalf("code=%d stderr=%q", out.code, out.stderr)
	}
	live := remoteCalls(f, "odoous")

	clk.advance(2 * time.Second)
	out := runEnv(t, f, env, "id", "odoous/configura")
	if out.code != 0 || out.stdout != "w1:p2\n" || remoteCalls(f, "odoous") != live {
		t.Errorf("code=%d stdout=%q, odoous calls %d -> %d (want the cache)", out.code, out.stdout, live, remoteCalls(f, "odoous"))
	}
}

func TestAnAgentMissingFromTheCacheIsLookedUpLiveBeforeGivingUp(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	runEnv(t, f, env, "id", "odoous/configura")
	live := remoteCalls(f, "odoous")

	// Entre medias nace un agente nuevo en odoous.
	f.Reply("odoous", []string{"agent", "list"}, `{"id":"x","result":{"agents":[`+
		`{"agent":"codex","agent_status":"idle","focused":true,"pane_id":"w1:p2","tab_id":"w1:t2","workspace_id":"w1","terminal_title_stripped":"Configura Odoo 18"},`+
		`{"agent":"claude","agent_status":"idle","focused":false,"pane_id":"w1:p9","tab_id":"w1:t9","workspace_id":"w1","name":"newbie"}]}}`)

	clk.advance(2 * time.Second)
	out := runEnv(t, f, env, "id", "odoous/newbie")
	if out.code != 0 || out.stdout != "w1:p9\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q: a miss on cached data must retry live", out.code, out.stdout, out.stderr)
	}
	if remoteCalls(f, "odoous") <= live {
		t.Errorf("the retry should have queried odoous")
	}
}

func TestANameThatReallyDoesNotExistFailsAfterOneLiveRetry(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	runEnv(t, f, env, "id", "odoous/configura")
	clk.advance(time.Second)

	out := runEnv(t, f, env, "id", "odoous/zzz-nothing")
	if out.code != 1 || !strings.Contains(out.stderr, `no agent matches "odoous/zzz-nothing"`) {
		t.Errorf("code=%d stderr=%q, want the usual not-found error", out.code, out.stderr)
	}
}

func TestAttentionAndPickNeverUseTheCache(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	runEnv(t, f, env, "ls")
	live := remoteCalls(f, "odoous")
	clk.advance(time.Second)
	runEnv(t, f, env, "attention")
	if remoteCalls(f, "odoous") <= live {
		t.Errorf("attention shows statuses, so it must always be live")
	}
}

func TestCreatingOrRenamingInvalidatesTheCache(t *testing.T) {
	t.Parallel()

	for name, run := range map[string]func(*testing.T, *herdrtest.Fake, cli.Env){
		"name": func(t *testing.T, f *herdrtest.Fake, env cli.Env) {
			f.Reply("odoous", []string{"agent", "rename", "w1:p2", "docs"}, okReply)
			f.Reply("odoous", []string{"pane", "rename", "w1:p2", "docs"}, okReply)
			if out := runEnv(t, f, env, "name", "odoous/configura", "docs"); out.code != 0 {
				t.Fatalf("name: %q", out.stderr)
			}
		},
		"new": func(t *testing.T, f *herdrtest.Fake, env cli.Env) {
			f.Reply("odoous", []string{"tab", "create", "--label", "claude", "--focus"}, newTab)
			f.Reply("odoous", []string{"agent", "start", "claude", "--kind", "claude", "--pane", "wS:pA"}, newStarted)
			if out := runEnv(t, f, env, "new", "odoous/claude"); out.code != 0 {
				t.Fatalf("new: %q", out.stderr)
			}
		},
	} {
		f := fixture()
		env, clk := cachedEnv(t)
		runEnv(t, f, env, "ls")
		if _, err := os.Stat(filepath.Join(env.CacheDir, "inventory.json")); err != nil {
			t.Fatalf("%s: the cache file should exist after ls: %v", name, err)
		}
		clk.advance(time.Second)
		run(t, f, env)
		if _, err := os.Stat(filepath.Join(env.CacheDir, "inventory.json")); !os.IsNotExist(err) {
			t.Errorf("%s: the cache should be gone after changing something (stat error %v)", name, err)
		}
	}
}

func TestWithoutACacheDirectoryEverythingIsLive(t *testing.T) {
	t.Parallel()

	f := fixture()
	runEnv(t, f, cli.Env{}, "ls")
	live := remoteCalls(f, "odoous")
	runEnv(t, f, cli.Env{}, "ls")
	if remoteCalls(f, "odoous") <= live {
		t.Errorf("no cache directory means no cache")
	}
}

func TestFreshIsAcceptedByEveryCommandThatUsesTheCache(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"ls", "--fresh"}, {"id", "importer", "--fresh"}, {"look", "importer", "--fresh"},
		{"go", "importer", "--fresh"}, {"wait", "importer", "--fresh"},
	} {
		f := fixture()
		f.Reply("", []string{"agent", "read", "wS:p1"}, "x")
		f.Reply("", []string{"agent", "focus", "wS:p1"}, okReply)
		f.Reply("", []string{"agent", "wait", "wS:p1"}, `{"id":"x","result":{"agent":{"agent_status":"idle"}}}`)
		if out := run(t, f, args...); out.code == 2 {
			t.Errorf("%v: --fresh was rejected: %s", args, out.stderr)
		}
	}
	if out := run(t, fixture(), "attention", "--fresh"); out.code != 2 {
		t.Errorf("attention is always live, so --fresh should be a usage error (code %d)", out.code)
	}
}

func TestAttentionKeepsTheCacheWarmWithoutReadingIt(t *testing.T) {
	t.Parallel()

	f := fixture()
	env, clk := cachedEnv(t)
	runEnv(t, f, env, "attention")
	if _, err := os.Stat(filepath.Join(env.CacheDir, "inventory.json")); err != nil {
		t.Fatalf("attention should have renewed the cache: %v", err)
	}

	live := remoteCalls(f, "odoous")
	clk.advance(2 * time.Second)
	out := runEnv(t, f, env, "ls")
	if remoteCalls(f, "odoous") != live || !strings.Contains(out.stdout, "cached 2s") {
		t.Errorf("ls right after attention should use what attention stored:\n%s", out.stdout)
	}

	// Y attention nunca lee: aunque la caché sea reciente, consulta de nuevo.
	runEnv(t, f, env, "attention")
	if remoteCalls(f, "odoous") <= live {
		t.Errorf("attention must stay live")
	}
}
