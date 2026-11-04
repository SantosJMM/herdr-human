package inventory_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

const machineList = "id1\tremoto2\tserver2\tdefault\tenabled\r\n" +
	"id2\toff\tserver3\tdefault\tdisabled\r\n" +
	"id3\todoous\tservidor-odoous\tdefault\tenabled\r\n"

var (
	agentList     = []string{"agent", "list"}
	workspaceList = []string{"workspace", "list"}
	machineListCm = []string{"machine", "list"}
)

func agentsJSON(agents ...string) string {
	return `{"id":"x","result":{"type":"agent_list","agents":[` + strings.Join(agents, ",") + `]}}`
}

func workspacesJSON(pairs ...string) string {
	var items []string
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, `{"workspace_id":"`+pairs[i]+`","label":"`+pairs[i+1]+`"}`)
	}
	return `{"id":"x","result":{"type":"workspace_list","workspaces":[` + strings.Join(items, ",") + `]}}`
}

const (
	localAgent  = `{"agent":"claude","agent_status":"working","cwd":"C:\\raw","focused":true,"pane_id":"wS:p4","tab_id":"wS:t4","workspace_id":"wS","terminal_title_stripped":"Pane ID work"}`
	namedAgent  = `{"agent":"codex","agent_status":"idle","focused":false,"pane_id":"wS:p1","tab_id":"wS:t1","workspace_id":"wS","name":"importer","terminal_title_stripped":"ignored"}`
	remoteAgent = `{"agent":"codex","agent_status":"idle","cwd":"/opt/odoo-dev/18","focused":true,"pane_id":"w1:p2","tab_id":"w1:t2","workspace_id":"w1","terminal_title_stripped":"Configura Odoo 18"}`
)

func newFake() *herdrtest.Fake {
	f := herdrtest.New()
	f.Reply("", machineListCm, machineList)
	f.Reply("", agentList, agentsJSON(localAgent, namedAgent))
	f.Reply("", workspaceList, workspacesJSON("wS", "raw"))
	f.Reply("odoous", agentList, agentsJSON(remoteAgent))
	f.Reply("odoous", workspaceList, workspacesJSON("w1", "odoo-inc"))
	f.Reply("remoto2", agentList, agentsJSON())
	f.Reply("remoto2", workspaceList, workspacesJSON())
	// Servidores "antiguos": rechazan el snapshot y obligan a usar las listas.
	for _, m := range []string{"", "odoous", "remoto2"} {
		f.Reply(m, snapshotCmd, unsupported)
	}
	return f
}

var snapshotCmd = []string{"api", "snapshot"}

const unsupported = `{"error":{"code":"unknown_command","message":"no snapshot here"},"id":"x"}`

func snapshotJSON(workspaces string, agents ...string) string {
	return `{"id":"x","result":{"snapshot":{"agents":[` + strings.Join(agents, ",") +
		`],"workspaces":[` + workspaces + `]}}}`
}

// snapshotFake modela servidores modernos: solo responden al snapshot.
func snapshotFake() *herdrtest.Fake {
	f := herdrtest.New()
	f.Reply("", machineListCm, machineList)
	f.Reply("", snapshotCmd, snapshotJSON(`{"workspace_id":"wS","label":"raw"}`, localAgent, namedAgent))
	f.Reply("odoous", snapshotCmd, snapshotJSON(`{"workspace_id":"w1","label":"odoo-inc"}`, remoteAgent))
	f.Reply("remoto2", snapshotCmd, snapshotJSON(``))
	return f
}

func TestLoadMergesLocalAndEnabledMachines(t *testing.T) {
	t.Parallel()

	inv, err := inventory.Load(context.Background(), newFake(), inventory.Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(inv.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", inv.Warnings)
	}

	type row struct{ machine, workspace, pane, name string }
	var got []row
	for _, tg := range inv.Targets {
		got = append(got, row{tg.Machine, tg.Workspace, tg.PaneID, tg.Name})
	}
	want := []row{
		{"", "raw", "wS:p4", "Pane ID work"},
		{"", "raw", "wS:p1", "importer"},
		{"odoous", "odoo-inc", "w1:p2", "Configura Odoo 18"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("target[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadSkipsDisabledMachines(t *testing.T) {
	t.Parallel()

	fake := newFake()
	if _, err := inventory.Load(context.Background(), fake, inventory.Options{}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, c := range fake.Calls() {
		if c.Machine == "off" {
			t.Fatalf("disabled machine was queried: %+v", c)
		}
	}
}

func TestLoadKeepsAgentFields(t *testing.T) {
	t.Parallel()

	inv, err := inventory.Load(context.Background(), newFake(), inventory.Options{LocalOnly: true})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	first := inv.Targets[0]
	if first.Agent != "claude" || first.Status != "working" || !first.Focused || first.Cwd != `C:\raw` ||
		first.TabID != "wS:t4" || first.WorkspaceID != "wS" {
		t.Errorf("unexpected fields: %+v", first)
	}
}

func TestLoadLocalOnlyDoesNotListMachines(t *testing.T) {
	t.Parallel()

	fake := newFake()
	inv, err := inventory.Load(context.Background(), fake, inventory.Options{LocalOnly: true})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(inv.Targets) != 2 {
		t.Errorf("got %d targets, want 2", len(inv.Targets))
	}
	for _, c := range fake.Calls() {
		if c.Machine != "" || strings.Join(c.Args, " ") == "machine list" {
			t.Errorf("unexpected remote activity: %+v", c)
		}
	}
}

func TestLoadMachineFilterQueriesOnlyThoseMachines(t *testing.T) {
	t.Parallel()

	fake := newFake()
	inv, err := inventory.Load(context.Background(), fake, inventory.Options{Machines: []string{"ODOOUS"}})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(inv.Targets) != 3 {
		t.Errorf("got %d targets, want 3", len(inv.Targets))
	}
	for _, c := range fake.Calls() {
		if c.Machine == "remoto2" {
			t.Errorf("machine outside the filter was queried: %+v", c)
		}
	}
}

func TestLoadUnknownMachineIsAnError(t *testing.T) {
	t.Parallel()

	_, err := inventory.Load(context.Background(), newFake(), inventory.Options{Machines: []string{"nope"}})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Load() error = %v, want an unknown machine error naming it", err)
	}
}

func TestLoadRemoteFailureBecomesWarning(t *testing.T) {
	t.Parallel()

	boom := errors.New("ssh: connection refused")
	fake := newFake()
	fake.Fail("odoous", agentList, boom)

	inv, err := inventory.Load(context.Background(), fake, inventory.Options{})
	if err != nil {
		t.Fatalf("Load() error = %v, want the local data to survive", err)
	}
	if len(inv.Targets) != 2 {
		t.Errorf("got %d targets, want the 2 local ones", len(inv.Targets))
	}
	if len(inv.Warnings) != 1 || inv.Warnings[0].Machine != "odoous" || !errors.Is(inv.Warnings[0].Err, boom) {
		t.Errorf("warnings = %+v, want one for odoous wrapping the cause", inv.Warnings)
	}
}

func TestLoadLocalFailureIsFatal(t *testing.T) {
	t.Parallel()

	boom := errors.New("socket closed")
	fake := newFake()
	fake.Fail("", agentList, boom)

	if _, err := inventory.Load(context.Background(), fake, inventory.Options{}); !errors.Is(err, boom) {
		t.Fatalf("Load() error = %v, want the local failure", err)
	}
}

func TestLoadTimesOutSlowMachines(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.Hang("odoous", agentList)

	start := time.Now()
	inv, err := inventory.Load(context.Background(), fake, inventory.Options{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Load() took %v, the slow machine was not cut off", elapsed)
	}
	if len(inv.Warnings) != 1 || !errors.Is(inv.Warnings[0].Err, context.DeadlineExceeded) {
		t.Errorf("warnings = %+v, want a deadline warning", inv.Warnings)
	}
}

func TestLoadFallsBackToWorkspaceID(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.Fail("", workspaceList, errors.New("boom"))

	inv, err := inventory.Load(context.Background(), fake, inventory.Options{LocalOnly: true})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := inv.Targets[0].Workspace; got != "wS" {
		t.Errorf("Workspace = %q, want the id as fallback", got)
	}
}

func TestLoadScopeRestrictsQueriedServers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		scope        string
		wantTargets  int
		wantMachines []string
	}{
		{"machine scope", "Odoous", 3, []string{"", "odoous"}},
		{"local scope", "local", 2, []string{""}},
		{"unknown prefix is ignored", "analiza ", 3, []string{"", "odoous", "remoto2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := newFake()
			inv, err := inventory.Load(context.Background(), fake, inventory.Options{Scope: tt.scope})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(inv.Targets) != tt.wantTargets {
				t.Errorf("got %d targets, want %d", len(inv.Targets), tt.wantTargets)
			}

			seen := map[string]bool{}
			for _, c := range fake.Calls() {
				if strings.Join(c.Args, " ") != "machine list" {
					seen[c.Machine] = true
				}
			}
			if len(seen) != len(tt.wantMachines) {
				t.Errorf("queried servers = %v, want exactly %v", seen, tt.wantMachines)
			}
			for _, m := range tt.wantMachines {
				if !seen[m] {
					t.Errorf("server %q was not queried (seen %v)", m, seen)
				}
			}
		})
	}
}

func TestLoadUsesOneSnapshotCallPerServer(t *testing.T) {
	t.Parallel()

	fake := snapshotFake()
	inv, err := inventory.Load(context.Background(), fake, inventory.Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(inv.Targets) != 3 || inv.Targets[2].Workspace != "odoo-inc" || inv.Targets[0].Workspace != "raw" {
		t.Errorf("unexpected targets: %+v", inv.Targets)
	}

	perServer := map[string]int{}
	for _, c := range fake.Calls() {
		if strings.Join(c.Args, " ") == "machine list" {
			continue
		}
		perServer[c.Machine]++
		if c.Args[0] != "api" {
			t.Errorf("legacy call made on a snapshot-capable server: %+v", c)
		}
	}
	for server, n := range perServer {
		if n != 1 {
			t.Errorf("server %q received %d calls, want exactly 1", server, n)
		}
	}
}

func TestLoadFallsBackToListsWhenSnapshotIsUnsupported(t *testing.T) {
	t.Parallel()

	inv, err := inventory.Load(context.Background(), newFake(), inventory.Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(inv.Targets) != 3 || len(inv.Warnings) != 0 {
		t.Errorf("targets=%d warnings=%v, want 3 targets and none", len(inv.Targets), inv.Warnings)
	}
}

func TestLoadDoesNotRetryAfterTransportFailure(t *testing.T) {
	t.Parallel()

	// Un fallo de transporte (no una negativa de herdr) no debe duplicar la
	// espera repitiendo la consulta por el camino antiguo.
	fake := snapshotFake()
	fake.Fail("odoous", snapshotCmd, errors.New("ssh: connection refused"))

	inv, err := inventory.Load(context.Background(), fake, inventory.Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(inv.Warnings) != 1 || inv.Warnings[0].Machine != "odoous" {
		t.Errorf("warnings = %+v, want one for odoous", inv.Warnings)
	}
	n := 0
	for _, c := range fake.Calls() {
		if c.Machine == "odoous" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("odoous received %d calls, want 1 (no fallback)", n)
	}
}

func TestLoadReportsTimingsPerServer(t *testing.T) {
	t.Parallel()

	boom := errors.New("down")
	fake := snapshotFake()
	fake.Fail("odoous", snapshotCmd, boom)

	inv, err := inventory.Load(context.Background(), fake, inventory.Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	var order []string
	for _, tm := range inv.Timings {
		order = append(order, tm.Machine)
		if tm.Elapsed < 0 {
			t.Errorf("negative elapsed for %q", tm.Machine)
		}
	}
	want := []string{"", "remoto2", "odoous"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("timing order = %q, want %q", order, want)
	}
	if last := inv.Timings[len(inv.Timings)-1]; !errors.Is(last.Err, boom) {
		t.Errorf("odoous timing error = %v, want the failure recorded", last.Err)
	}
}

func TestLoadLocalScopeSkipsMachineList(t *testing.T) {
	t.Parallel()

	fake := newFake()
	if _, err := inventory.Load(context.Background(), fake, inventory.Options{Scope: "local"}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, c := range fake.Calls() {
		if strings.Join(c.Args, " ") == "machine list" {
			t.Fatal("machine list was called for a local-only scope")
		}
	}
}
