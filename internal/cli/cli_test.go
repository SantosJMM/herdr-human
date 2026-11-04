package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

const (
	okReply = `{"id":"x","result":{"type":"ok"}}`

	machines = "id1\tremoto2\tserver2\tdefault\tenabled\r\n" +
		"id3\todoous\tservidor-odoous\tdefault\tenabled\r\n"

	localAgents = `{"id":"x","result":{"agents":[` +
		`{"agent":"claude","agent_status":"working","focused":true,"pane_id":"wS:p4","tab_id":"wS:t4","workspace_id":"wS","terminal_title_stripped":"Pane ID work"},` +
		`{"agent":"codex","agent_status":"idle","focused":false,"pane_id":"wS:p1","tab_id":"wS:t1","workspace_id":"wS","name":"importer"}]}}`
	localWorkspaces = `{"id":"x","result":{"workspaces":[{"workspace_id":"wS","label":"raw"}]}}`

	remoteAgents = `{"id":"x","result":{"agents":[` +
		`{"agent":"codex","agent_status":"idle","focused":true,"pane_id":"w1:p2","tab_id":"w1:t2","workspace_id":"w1","terminal_title_stripped":"Configura Odoo 18"}]}}`
	remoteWorkspaces = `{"id":"x","result":{"workspaces":[{"workspace_id":"w1","label":"odoo-inc"}]}}`

	emptyAgents     = `{"id":"x","result":{"agents":[]}}`
	emptyWorkspaces = `{"id":"x","result":{"workspaces":[]}}`
)

func fixture() *herdrtest.Fake {
	f := herdrtest.New()
	f.Reply("", []string{"machine", "list"}, machines)
	f.Reply("", []string{"agent", "list"}, localAgents)
	f.Reply("", []string{"workspace", "list"}, localWorkspaces)
	f.Reply("odoous", []string{"agent", "list"}, remoteAgents)
	f.Reply("odoous", []string{"workspace", "list"}, remoteWorkspaces)
	f.Reply("remoto2", []string{"agent", "list"}, emptyAgents)
	f.Reply("remoto2", []string{"workspace", "list"}, emptyWorkspaces)
	// Servidores "antiguos": sin snapshot, se usan las listas.
	for _, m := range []string{"", "odoous", "remoto2"} {
		f.Reply(m, []string{"api", "snapshot"}, `{"error":{"code":"unknown_command","message":"no snapshot"},"id":"x"}`)
	}
	return f
}

type output struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, fake *herdrtest.Fake, args ...string) output {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{Runner: fake, Stdout: &stdout, Stderr: &stderr}, args)
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func lastCall(f *herdrtest.Fake) []string {
	calls := f.Calls()
	c := calls[len(calls)-1]
	return append([]string{c.Machine}, c.Args...)
}

func TestHelp(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "help")
	if out.code != 0 || !strings.Contains(out.stdout, "hum say") {
		t.Errorf("help: code=%d stdout=%q", out.code, out.stdout)
	}
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"frobnicate"}},
		{"id without query", []string{"id"}},
		{"say without message", []string{"say", "importer"}},
		{"name without name", []string{"name", "importer"}},
		{"unknown flag", []string{"ls", "--nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := run(t, fixture(), tt.args...)
			if out.code != 2 {
				t.Errorf("code = %d, want 2 (stderr %q)", out.code, out.stderr)
			}
			if out.stderr == "" {
				t.Error("stderr is empty, want a usage message")
			}
		})
	}
}

func TestLsHumanOutput(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "ls")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, want := range []string{"Local", "raw", "wS:p4", "importer", "odoous", "odoo-inc", "w1:p2", "Configura Odoo 18", "working"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, out.stdout)
		}
	}
	if strings.Index(out.stdout, "Local") > strings.Index(out.stdout, "odoous") {
		t.Errorf("local must come first:\n%s", out.stdout)
	}
}

func TestLsJSON(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "ls", "--json")
	var payload struct {
		Targets  []map[string]any `json:"targets"`
		Warnings []map[string]any `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out.stdout), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.stdout)
	}
	if len(payload.Targets) != 3 {
		t.Errorf("targets = %d, want 3", len(payload.Targets))
	}
}

func TestLsWarnsAboutUnreachableMachine(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Fail("remoto2", []string{"agent", "list"}, errors.New("ssh: timeout"))

	out := run(t, fake, "ls")
	if out.code != 0 {
		t.Fatalf("code = %d, want 0 with a warning", out.code)
	}
	if !strings.Contains(out.stderr, "remoto2") {
		t.Errorf("stderr = %q, want a warning naming remoto2", out.stderr)
	}
	if !strings.Contains(out.stdout, "importer") {
		t.Errorf("stdout lost the reachable agents:\n%s", out.stdout)
	}
}

func TestLsVerbosePrintsTimingsToStderr(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "ls", "--verbose")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, want := range []string{"timings", "local", "odoous", "remoto2"} {
		if !strings.Contains(out.stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, out.stderr)
		}
	}
	if strings.Contains(out.stdout, "timings") {
		t.Error("timings must go to stderr so stdout stays parseable")
	}
}

func TestLsWithoutVerboseStaysQuiet(t *testing.T) {
	t.Parallel()

	if out := run(t, fixture(), "ls"); strings.Contains(out.stderr, "timings") {
		t.Errorf("stderr = %q, want no timings without --verbose", out.stderr)
	}
}

func TestTimeoutFlagCutsOffSlowMachines(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Hang("odoous", []string{"agent", "list"})

	out := run(t, fake, "ls", "--server-timeout", "50ms")
	if out.code != 0 || !strings.Contains(out.stderr, "odoous") || !strings.Contains(out.stderr, "deadline") {
		t.Errorf("code=%d stderr=%q, want a deadline warning for odoous", out.code, out.stderr)
	}
}

func TestInvalidTimeoutIsAUsageError(t *testing.T) {
	t.Parallel()

	if out := run(t, fixture(), "ls", "--server-timeout", "soon"); out.code != 2 {
		t.Errorf("code = %d, want 2", out.code)
	}
}

func TestLsLocalFlag(t *testing.T) {
	t.Parallel()

	fake := fixture()
	out := run(t, fake, "ls", "--local")
	if out.code != 0 || strings.Contains(out.stdout, "odoous") {
		t.Errorf("code=%d stdout=%q, want local agents only", out.code, out.stdout)
	}
}

func TestLsMachineFlag(t *testing.T) {
	t.Parallel()

	fake := fixture()
	out := run(t, fake, "ls", "--machine", "odoous")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, c := range fake.Calls() {
		if c.Machine == "remoto2" {
			t.Errorf("remoto2 was queried despite --machine odoous")
		}
	}
}

func TestIDOutputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"pane id only", []string{"id", "importer"}, "wS:p1\n"},
		{"flags after the query", []string{"id", "odoous/configura", "--cmd"}, "herdr --machine odoous agent prompt w1:p2\n"},
		{"local command", []string{"id", "importer", "--cmd"}, "herdr agent prompt wS:p1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := run(t, fixture(), tt.args...)
			if out.code != 0 || out.stdout != tt.want {
				t.Errorf("code=%d stdout=%q, want %q (stderr %q)", out.code, out.stdout, tt.want, out.stderr)
			}
		})
	}
}

func TestIDJSON(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "id", "--json", "odoous/configura")
	var got struct {
		Machine string `json:"machine"`
		PaneID  string `json:"pane_id"`
	}
	if err := json.Unmarshal([]byte(out.stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.stdout)
	}
	if got.Machine != "odoous" || got.PaneID != "w1:p2" {
		t.Errorf("got %+v", got)
	}
}

func TestResolutionErrors(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "id", "nothing")
	if out.code != 1 || !strings.Contains(out.stderr, `no agent matches "nothing"`) {
		t.Errorf("not found: code=%d stderr=%q", out.code, out.stderr)
	}

	out = run(t, fixture(), "id", "codex")
	if out.code != 1 || !strings.Contains(out.stderr, "local/wS:p1") || !strings.Contains(out.stderr, "odoous/w1:p2") {
		t.Errorf("ambiguous: code=%d stderr=%q, want both candidates listed", out.code, out.stderr)
	}
}

func TestScopedQueryDoesNotTouchOtherMachines(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("odoous", []string{"agent", "prompt", "w1:p2", "hello"}, okReply)

	out := run(t, fake, "say", "odoous/configura", "hello")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, c := range fake.Calls() {
		if c.Machine == "remoto2" {
			t.Errorf("remoto2 was queried for an odoous-scoped query")
		}
	}
	if !strings.Contains(out.stdout, "odoous/w1:p2") {
		t.Errorf("stdout = %q, want the confirmation to name the target", out.stdout)
	}
}

func TestSayJoinsWordsAndPassesWaitFlags(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "run the tests", "--wait", "--timeout", "3000"}, okReply)

	out := run(t, fake, "say", "importer", "run", "the", "tests", "--wait", "--timeout", "3000")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	want := []string{"", "agent", "prompt", "wS:p1", "run the tests", "--wait", "--timeout", "3000"}
	if got := lastCall(fake); !reflect.DeepEqual(got, want) {
		t.Errorf("last call = %q, want %q", got, want)
	}
}

func TestSayDoubleDashKeepsLeadingDashText(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "-v please"}, okReply)

	out := run(t, fake, "say", "importer", "--", "-v", "please")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
}

func TestLook(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "read", "wS:p1", "--lines", "5"}, "tail of the terminal\n")

	out := run(t, fake, "look", "importer", "--lines", "5")
	if out.code != 0 || out.stdout != "tail of the terminal\n" {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("odoous", []string{"agent", "rename", "w1:p2", "odoo18"}, okReply)
	fake.Reply("odoous", []string{"pane", "rename", "w1:p2", "odoo18"}, okReply)

	out := run(t, fake, "name", "odoous/configura", "odoo18")
	if out.code != 0 || !strings.Contains(out.stdout, "odoo18") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestNameClear(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "rename", "wS:p1", "--clear"}, okReply)
	fake.Reply("", []string{"pane", "rename", "wS:p1", "--clear"}, okReply)

	if out := run(t, fake, "name", "importer", "--clear"); out.code != 0 {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestNameRejectsInvalidName(t *testing.T) {
	t.Parallel()

	out := run(t, fixture(), "name", "importer", "Not Valid")
	if out.code != 1 || !strings.Contains(out.stderr, "lowercase") {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

func TestGo(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "focus", "wS:p1"}, okReply)

	if out := run(t, fake, "go", "importer"); out.code != 0 {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}

const (
	waitIdleReply    = `{"id":"x","result":{"agent":{"agent_status":"idle","pane_id":"wS:p1"}}}`
	waitBlockedReply = `{"id":"x","result":{"agent":{"agent_status":"blocked","pane_id":"wS:p1"}}}`
	waitTimeoutReply = `{"error":{"code":"timeout","message":"timed out waiting for agent status"},"id":"x"}`
	notifyReply      = `{"id":"x","result":{"shown":true}}`
)

func TestWaitReportsReachedStatus(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "wait", "wS:p1"}, waitIdleReply)

	out := run(t, fake, "wait", "importer")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	if want := "local/wS:p1 (importer) is idle\n"; out.stdout != want {
		t.Errorf("stdout = %q, want %q", out.stdout, want)
	}
}

func TestWaitPassesUntilAndTimeout(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "wait", "wS:p1", "--until", "idle", "--until", "done", "--timeout", "4000"}, waitIdleReply)

	out := run(t, fake, "wait", "importer", "--until", "idle", "--until", "done", "--timeout", "4000")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
}

func TestWaitExitsThreeWhenBlocked(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "wait", "wS:p1"}, waitBlockedReply)

	out := run(t, fake, "wait", "importer")
	if out.code != 3 {
		t.Fatalf("code = %d, want 3 (stderr %q)", out.code, out.stderr)
	}
	if !strings.Contains(out.stdout, "is blocked") {
		t.Errorf("stdout = %q, want the reached status", out.stdout)
	}
	if !strings.Contains(out.stderr, "waiting for you") || !strings.Contains(out.stderr, "hum go local/wS:p1") {
		t.Errorf("stderr = %q, want a hint to go to the agent", out.stderr)
	}
}

func TestWaitBlockedIsSuccessWhenRequested(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "wait", "wS:p1", "--until", "blocked"}, waitBlockedReply)

	out := run(t, fake, "wait", "importer", "--until", "blocked")
	if out.code != 0 {
		t.Fatalf("code = %d, want 0 (stderr %q)", out.code, out.stderr)
	}
}

func TestWaitExitsFourOnTimeout(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "wait", "wS:p1", "--timeout", "500"}, waitTimeoutReply)

	out := run(t, fake, "wait", "importer", "--timeout", "500")
	if out.code != 4 {
		t.Fatalf("code = %d, want 4 (stderr %q)", out.code, out.stderr)
	}
	if !strings.Contains(out.stderr, "timed out") {
		t.Errorf("stderr = %q, want a timeout message", out.stderr)
	}
}

func TestWaitNotifiesLocallyWhenAsked(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("odoous", []string{"agent", "wait", "w1:p2"}, `{"id":"x","result":{"agent":{"agent_status":"idle"}}}`)
	fake.Reply("", []string{"notification", "show", "Configura Odoo 18 is idle", "--body", "odoous/w1:p2"}, notifyReply)

	out := run(t, fake, "wait", "odoous/w1:p2", "--notify")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	if got := lastCall(fake); got[1] != "notification" {
		t.Errorf("last call = %q, want the notification", got)
	}
}

func TestWaitNotifyFailureIsOnlyAWarning(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "wait", "wS:p1"}, waitIdleReply)
	fake.Fail("", []string{"notification", "show", "importer is idle", "--body", "local/wS:p1"}, errors.New("no ui"))

	out := run(t, fake, "wait", "importer", "--notify")
	if out.code != 0 {
		t.Fatalf("code = %d, want 0 (stderr %q)", out.code, out.stderr)
	}
	if !strings.Contains(out.stderr, "warning: notification") {
		t.Errorf("stderr = %q, want a notification warning", out.stderr)
	}
}

func TestWaitUsageErrors(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"without who":    {"wait"},
		"invalid status": {"wait", "importer", "--until", "sleeping"},
		"extra argument": {"wait", "importer", "now"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if out := run(t, fixture(), args...); out.code != 2 {
				t.Errorf("code = %d, want 2 (stderr %q)", out.code, out.stderr)
			}
		})
	}
}

const askIdleReply = `{"id":"x","result":{"agent":{"agent_status":"idle"}}}`

func TestAskPrintsOnlyTheReplyOnStdout(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "run the tests", "--wait", "--timeout", "5000"}, askIdleReply)
	fake.Reply("", []string{"agent", "read", "wS:p1", "--lines", "200"}, "> run the tests\nAll 42 tests passed.\n")

	out := run(t, fake, "ask", "importer", "run", "the", "tests", "--timeout", "5000")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	if want := "All 42 tests passed.\n"; out.stdout != want {
		t.Errorf("stdout = %q, want %q", out.stdout, want)
	}
	for _, want := range []string{"local/wS:p1 (importer)", "idle in", "reply: after your message"} {
		if !strings.Contains(out.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", out.stderr, want)
		}
	}
}

func TestAskSaysWhenItFellBackToTheLastLines(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "ping", "--wait"}, askIdleReply)
	fake.Reply("", []string{"agent", "read", "wS:p1", "--lines", "200"}, "a\nb\nc\n")

	out := run(t, fake, "ask", "importer", "ping", "--lines", "2")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	if out.stdout != "b\nc\n" {
		t.Errorf("stdout = %q, want the last 2 lines", out.stdout)
	}
	if !strings.Contains(out.stderr, "reply: last 2 lines (your message was not found)") {
		t.Errorf("stderr = %q, want the fallback notice", out.stderr)
	}
}

func TestAskExitsThreeWhenTheAgentEndsBlocked(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "deploy", "--wait"},
		`{"id":"x","result":{"agent":{"agent_status":"blocked"}}}`)
	fake.Reply("", []string{"agent", "read", "wS:p1", "--lines", "200"}, "> deploy\nRun rm -rf build? [y/n]\n")

	out := run(t, fake, "ask", "importer", "deploy")
	if out.code != 3 {
		t.Fatalf("code = %d, want 3 (stderr %q)", out.code, out.stderr)
	}
	if out.stdout != "" {
		t.Errorf("stdout = %q, want it empty", out.stdout)
	}
	if !strings.Contains(out.stderr, "waiting for you: Run rm -rf build? [y/n]") {
		t.Errorf("stderr = %q, want the pending question", out.stderr)
	}
}

func TestAskExitsThreeWhenAlreadyBlocked(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "hi", "--wait"},
		`{"error":{"code":"agent_blocked","message":"waiting"},"id":"x"}`)

	out := run(t, fake, "ask", "importer", "hi")
	if out.code != 3 || !strings.Contains(out.stderr, "hum go local/wS:p1") {
		t.Errorf("code = %d, stderr %q, want 3 and a hint to go to the agent", out.code, out.stderr)
	}
}

func TestAskExitsFourOnTimeout(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "hi", "--wait", "--timeout", "100"}, waitTimeoutReply)

	out := run(t, fake, "ask", "importer", "hi", "--timeout", "100")
	if out.code != 4 {
		t.Fatalf("code = %d, want 4 (stderr %q)", out.code, out.stderr)
	}
}

func TestAskUsageErrors(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"without who":     {"ask"},
		"without message": {"ask", "importer"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if out := run(t, fixture(), args...); out.code != 2 {
				t.Errorf("code = %d, want 2 (stderr %q)", out.code, out.stderr)
			}
		})
	}
}
