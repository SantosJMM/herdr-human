package inventory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

// clock es un reloj que solo avanza cuando se le dice.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newCache(t *testing.T) (*inventory.Cache, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	return &inventory.Cache{Path: filepath.Join(t.TempDir(), "sub", "inventory.json"), TTL: 10 * time.Second, Now: clk.Now}, clk
}

func calls(f *herdrtest.Fake, machine string) int {
	n := 0
	for _, c := range f.Calls() {
		if c.Machine == machine {
			n++
		}
	}
	return n
}

func load(t *testing.T, f *herdrtest.Fake, c *inventory.Cache) inventory.Inventory {
	t.Helper()
	inv, err := inventory.LoadCached(context.Background(), f, inventory.Options{Timeout: 200 * time.Millisecond}, c)
	if err != nil {
		t.Fatalf("LoadCached: %v", err)
	}
	return inv
}

func timingOf(inv inventory.Inventory, machine string) inventory.Timing {
	for _, tm := range inv.Timings {
		if tm.Machine == machine {
			return tm
		}
	}
	return inventory.Timing{}
}

func TestCacheServesRemoteMachinesWithinTheTTLButNeverTheLocalServer(t *testing.T) {
	t.Parallel()

	f := newFake()
	c, clk := newCache(t)
	load(t, f, c)
	liveRemote, liveLocal := calls(f, "odoous"), calls(f, "")
	if liveRemote == 0 {
		t.Fatal("the first load must query odoous")
	}

	clk.advance(3 * time.Second)
	inv := load(t, f, c)
	if got := calls(f, "odoous"); got != liveRemote {
		t.Errorf("odoous was queried again (%d -> %d calls) within the TTL", liveRemote, got)
	}
	if got := calls(f, ""); got <= liveLocal {
		t.Errorf("the local server must always be live, calls %d -> %d", liveLocal, got)
	}
	tm := timingOf(inv, "odoous")
	if !tm.Cached || tm.Age != 3*time.Second {
		t.Errorf("odoous timing = %+v, want cached with age 3s", tm)
	}
	if timingOf(inv, "").Cached {
		t.Errorf("the local server must never be reported as cached")
	}
	if len(inv.Targets) != 3 {
		t.Errorf("targets = %d, want the 2 local and 1 remote agents", len(inv.Targets))
	}
}

func TestCachedTargetsKeepEveryField(t *testing.T) {
	t.Parallel()

	f := newFake()
	c, clk := newCache(t)
	live := load(t, f, c)
	clk.advance(time.Second)
	cached := load(t, f, c)

	find := func(inv inventory.Inventory) inventory.Target {
		for _, tg := range inv.Targets {
			if tg.Machine == "odoous" {
				return tg
			}
		}
		t.Fatal("no odoous target")
		return inventory.Target{}
	}
	if find(live) != find(cached) {
		t.Errorf("cached target differs:\n live   %+v\n cached %+v", find(live), find(cached))
	}
}

func TestCacheExpiresAfterTheTTL(t *testing.T) {
	t.Parallel()

	f := newFake()
	c, clk := newCache(t)
	load(t, f, c)
	before := calls(f, "odoous")

	clk.advance(11 * time.Second)
	inv := load(t, f, c)
	if calls(f, "odoous") <= before {
		t.Errorf("an expired entry must be queried again")
	}
	if timingOf(inv, "odoous").Cached {
		t.Errorf("a live answer must not be reported as cached")
	}
}

func TestFailuresAreNeverCached(t *testing.T) {
	t.Parallel()

	f := newFake()
	f.Hang("odoous", agentList)
	c, _ := newCache(t)
	inv := load(t, f, c)
	if !errors.Is(timingOf(inv, "odoous").Err, context.DeadlineExceeded) {
		t.Fatalf("odoous should have timed out: %+v", timingOf(inv, "odoous"))
	}
	before := calls(f, "odoous")
	load(t, f, c)
	if calls(f, "odoous") <= before {
		t.Errorf("a failed machine must be retried, not served from the cache")
	}
}

func TestBypassIgnoresTheStoredEntriesButRefreshesThem(t *testing.T) {
	t.Parallel()

	f := newFake()
	c, clk := newCache(t)
	load(t, f, c)
	before := calls(f, "odoous")

	clk.advance(time.Second)
	c.Bypass = true
	load(t, f, c)
	if calls(f, "odoous") <= before {
		t.Errorf("--fresh must query the machine again")
	}

	c.Bypass = false
	after := calls(f, "odoous")
	clk.advance(time.Second)
	load(t, f, c)
	if calls(f, "odoous") != after {
		t.Errorf("the entry written by the fresh load should be usable afterwards")
	}
}

func TestANilCacheIsAlwaysLive(t *testing.T) {
	t.Parallel()

	f := newFake()
	if _, err := inventory.LoadCached(context.Background(), f, inventory.Options{}, nil); err != nil {
		t.Fatal(err)
	}
	before := calls(f, "odoous")
	if _, err := inventory.LoadCached(context.Background(), f, inventory.Options{}, nil); err != nil {
		t.Fatal(err)
	}
	if calls(f, "odoous") <= before {
		t.Errorf("without a cache every load is live")
	}
}

func TestACorruptOrForeignFileIsIgnored(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"garbage":       "not json at all",
		"wrong version": `{"version":99,"machines":{"odoous":{"at":"2026-11-04T12:00:00Z","targets":[]}}}`,
		"empty":         "",
		"oversized":     `{"version":1,"machines":{},"pad":"` + strings.Repeat("x", 9<<20) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFake()
			c, _ := newCache(t)
			if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(c.Path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			inv := load(t, f, c)
			if len(inv.Targets) != 3 || timingOf(inv, "odoous").Cached {
				t.Errorf("a bad cache file must behave like no cache: %+v", inv.Timings)
			}
		})
	}
}

func TestSavePreservesOtherMachinesAndPrunesOldOnes(t *testing.T) {
	t.Parallel()

	f := newFake()
	c, clk := newCache(t)
	load(t, f, c) // guarda odoous y remoto2

	// Una consulta solo a odoous no debe borrar la entrada de remoto2.
	clk.advance(11 * time.Second)
	if _, err := inventory.LoadCached(context.Background(), f, inventory.Options{Machines: []string{"odoous"}}, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"remoto2"`) {
		t.Errorf("the entry of an unrelated machine was dropped:\n%s", data)
	}

	// Pasada una hora, las entradas se podan al guardar.
	clk.advance(2 * time.Hour)
	if _, err := inventory.LoadCached(context.Background(), f, inventory.Options{Machines: []string{"odoous"}}, c); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(c.Path)
	if strings.Contains(string(data), `"remoto2"`) {
		t.Errorf("entries older than an hour should be pruned:\n%s", data)
	}
}

func TestTheCacheFileIsPrivateToTheUser(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	c, _ := newCache(t)
	load(t, newFake(), c)
	for path, want := range map[string]os.FileMode{c.Path: 0o600, filepath.Dir(c.Path): 0o700} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}

func TestInvalidateRemovesTheFileAndToleratesItsAbsence(t *testing.T) {
	t.Parallel()

	c, _ := newCache(t)
	if err := c.Invalidate(); err != nil {
		t.Errorf("invalidating a missing file: %v", err)
	}
	load(t, newFake(), c)
	if _, err := os.Stat(c.Path); err != nil {
		t.Fatalf("the cache file should exist: %v", err)
	}
	if err := c.Invalidate(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.Path); !os.IsNotExist(err) {
		t.Errorf("the cache file should be gone, stat error = %v", err)
	}
}

func TestStartCachedDeliversCachedMachinesAtOnce(t *testing.T) {
	t.Parallel()

	f := newFake()
	c, clk := newCache(t)
	load(t, f, c)
	clk.advance(2 * time.Second)

	f.Hang("odoous", agentList) // si se consultara, bloquearía
	s, err := inventory.StartCached(context.Background(), f, inventory.Options{}, c)
	if err != nil {
		t.Fatal(err)
	}
	cachedSeen := map[string]bool{}
	for u := range s.Updates {
		cachedSeen[u.Machine] = u.Cached
	}
	if !cachedSeen["odoous"] || !cachedSeen["remoto2"] || cachedSeen[""] {
		t.Errorf("cached flags = %v, want remotes cached and local live", cachedSeen)
	}
}

func TestAssembleBuildsTheSameInventoryAsLoad(t *testing.T) {
	t.Parallel()

	f := newFake()
	s, err := inventory.Start(context.Background(), f, inventory.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]inventory.Update{}
	for u := range s.Updates {
		got[u.Machine] = u
	}
	inv, err := inventory.Assemble(s.Machines, got)
	if err != nil {
		t.Fatal(err)
	}
	want, err := inventory.Load(context.Background(), newFake(), inventory.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Targets) != len(want.Targets) || len(inv.Timings) != len(want.Timings) {
		t.Errorf("Assemble: %d targets %d timings; Load: %d and %d", len(inv.Targets), len(inv.Timings), len(want.Targets), len(want.Timings))
	}

	got[""] = inventory.Update{Machine: "", Err: errors.New("socket closed")}
	if _, err := inventory.Assemble(s.Machines, got); err == nil {
		t.Errorf("a local failure must be fatal")
	}
}

func TestPeekReturnsStoredRemoteAgentsEvenAfterTheTTLButNotAncientOnes(t *testing.T) {
	t.Parallel()

	f := newFake()
	c, clk := newCache(t)
	load(t, f, c)

	clk.advance(5 * time.Minute) // caducada para usarla, válida para sugerir nombres
	got := c.Peek()
	if len(got) != 1 || got[0].Machine != "odoous" || got[0].PaneID != "w1:p2" {
		t.Errorf("Peek = %+v, want the cached odoous agent", got)
	}
	for _, tg := range got {
		if tg.Machine == "" {
			t.Errorf("the local server is never cached, got %+v", tg)
		}
	}

	clk.advance(2 * time.Hour)
	if got := c.Peek(); len(got) != 0 {
		t.Errorf("entries older than an hour must not be offered: %+v", got)
	}
}

func TestPeekOnAMissingOrBadCacheIsEmptyAndIgnoresBypass(t *testing.T) {
	t.Parallel()

	var none *inventory.Cache
	if got := none.Peek(); len(got) != 0 {
		t.Errorf("a nil cache has nothing: %+v", got)
	}
	c, _ := newCache(t)
	if got := c.Peek(); len(got) != 0 {
		t.Errorf("a missing file has nothing: %+v", got)
	}

	load(t, newFake(), c)
	c.Bypass = true
	if got := c.Peek(); len(got) != 1 {
		t.Errorf("Peek must work with Bypass set (--fresh only affects normal reads): %+v", got)
	}
}
