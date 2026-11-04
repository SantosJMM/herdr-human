package actions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

const ok = `{"id":"x","result":{"type":"ok"}}`

var (
	local  = inventory.Target{PaneID: "wS:p1", Agent: "codex", Name: "importer"}
	remote = inventory.Target{Machine: "odoous", PaneID: "w1:p2", Agent: "codex", Name: "dev"}
)

func calls(f *herdrtest.Fake) [][]string {
	var out [][]string
	for _, c := range f.Calls() {
		out = append(out, append([]string{c.Machine}, c.Args...))
	}
	return out
}

func TestSay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target inventory.Target
		text   string
		opts   actions.SayOptions
		reply  []string
		want   []string
	}{
		{"local", local, "hello", actions.SayOptions{},
			[]string{"agent", "prompt", "wS:p1", "hello"},
			[]string{"", "agent", "prompt", "wS:p1", "hello"}},
		{"remote routes through the machine", remote, "hello", actions.SayOptions{},
			[]string{"agent", "prompt", "w1:p2", "hello"},
			[]string{"odoous", "agent", "prompt", "w1:p2", "hello"}},
		{"wait with timeout", local, "hello", actions.SayOptions{Wait: true, TimeoutMillis: 5000},
			[]string{"agent", "prompt", "wS:p1", "hello", "--wait", "--timeout", "5000"},
			[]string{"", "agent", "prompt", "wS:p1", "hello", "--wait", "--timeout", "5000"}},
		// herdr toma como texto el argumento que sigue al pane id, aunque
		// empiece por "-", y no admite "--"; las opciones van después.
		{"text starting with a dash goes right after the target", local, "-rf now", actions.SayOptions{},
			[]string{"agent", "prompt", "wS:p1", "-rf now"},
			[]string{"", "agent", "prompt", "wS:p1", "-rf now"}},
		{"dash text keeps wait flags after it", local, "-rf now", actions.SayOptions{Wait: true},
			[]string{"agent", "prompt", "wS:p1", "-rf now", "--wait"},
			[]string{"", "agent", "prompt", "wS:p1", "-rf now", "--wait"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := herdrtest.New()
			fake.Reply(tt.target.Machine, tt.reply, ok)
			svc := actions.Service{Runner: fake}

			if err := svc.Say(context.Background(), tt.target, tt.text, tt.opts); err != nil {
				t.Fatalf("Say() error = %v", err)
			}
			if got := calls(fake); len(got) != 1 || !reflect.DeepEqual(got[0], tt.want) {
				t.Errorf("calls = %q, want [%q]", got, tt.want)
			}
		})
	}
}

func TestSayRejectsEmptyText(t *testing.T) {
	t.Parallel()

	svc := actions.Service{Runner: herdrtest.New()}
	if err := svc.Say(context.Background(), local, "  ", actions.SayOptions{}); err == nil {
		t.Fatal("Say() error = nil, want an error for empty text")
	}
}

func TestLook(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("odoous", []string{"agent", "read", "w1:p2", "--lines", "20"}, "line one\nline two\n")
	svc := actions.Service{Runner: fake}

	got, err := svc.Look(context.Background(), remote, 20)
	if err != nil {
		t.Fatalf("Look() error = %v", err)
	}
	if got != "line one\nline two\n" {
		t.Errorf("Look() = %q", got)
	}
}

func TestLookWithoutLinesUsesDefault(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "read", "wS:p1"}, "x")
	svc := actions.Service{Runner: fake}

	if _, err := svc.Look(context.Background(), local, 0); err != nil {
		t.Fatalf("Look() error = %v", err)
	}
}

func TestNameRenamesAgentAndPane(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("odoous", []string{"agent", "rename", "w1:p2", "odoo18"}, ok)
	fake.Reply("odoous", []string{"pane", "rename", "w1:p2", "odoo18"}, ok)
	svc := actions.Service{Runner: fake}

	if err := svc.Name(context.Background(), remote, "odoo18"); err != nil {
		t.Fatalf("Name() error = %v", err)
	}
	want := [][]string{
		{"odoous", "agent", "rename", "w1:p2", "odoo18"},
		{"odoous", "pane", "rename", "w1:p2", "odoo18"},
	}
	if got := calls(fake); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestNameClearsWhenEmpty(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "rename", "wS:p1", "--clear"}, ok)
	fake.Reply("", []string{"pane", "rename", "wS:p1", "--clear"}, ok)
	svc := actions.Service{Runner: fake}

	if err := svc.Name(context.Background(), local, ""); err != nil {
		t.Fatalf("Name() error = %v", err)
	}
}

func TestNameValidatesBeforeCallingHerdr(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"Has Upper", "with space", "toolong-toolong-toolong-toolong-x", "ñandú"} {
		fake := herdrtest.New()
		svc := actions.Service{Runner: fake}

		err := svc.Name(context.Background(), local, bad)
		if !errors.Is(err, actions.ErrInvalidName) {
			t.Errorf("Name(%q) error = %v, want ErrInvalidName", bad, err)
		}
		if n := len(fake.Calls()); n != 0 {
			t.Errorf("Name(%q) made %d herdr calls, want 0", bad, n)
		}
	}
}

func TestNameReportsPartialFailure(t *testing.T) {
	t.Parallel()

	boom := errors.New("pane gone")
	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "rename", "wS:p1", "x"}, ok)
	fake.Fail("", []string{"pane", "rename", "wS:p1", "x"}, boom)
	svc := actions.Service{Runner: fake}

	err := svc.Name(context.Background(), local, "x")
	if !errors.Is(err, boom) {
		t.Fatalf("Name() error = %v, want it to wrap the pane rename failure", err)
	}
}

func TestFocus(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("odoous", []string{"agent", "focus", "w1:p2"}, ok)
	svc := actions.Service{Runner: fake}

	if err := svc.Focus(context.Background(), remote); err != nil {
		t.Fatalf("Focus() error = %v", err)
	}
}

const (
	waitIdle    = `{"id":"x","result":{"agent":{"agent_status":"idle","pane_id":"w1:p2"}}}`
	waitBlocked = `{"id":"x","result":{"agent":{"agent_status":"blocked","pane_id":"w1:p2"}}}`
	waitTimeout = `{"error":{"code":"timeout","message":"timed out waiting for agent status"},"id":"x"}`
)

func TestWaitBuildsArgsAndReturnsStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		opts  actions.WaitOptions
		want  []string
		reply string
		state string
	}{
		{"defaults", actions.WaitOptions{},
			[]string{"odoous", "agent", "wait", "w1:p2"}, waitIdle, "idle"},
		{"until and timeout", actions.WaitOptions{Until: []string{"idle", "done"}, TimeoutMillis: 2500},
			[]string{"odoous", "agent", "wait", "w1:p2", "--until", "idle", "--until", "done", "--timeout", "2500"}, waitBlocked, "blocked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := herdrtest.New()
			fake.Reply(tt.want[0], tt.want[1:], tt.reply)

			got, err := actions.Service{Runner: fake}.Wait(context.Background(), remote, tt.opts)
			if err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
			if got != tt.state {
				t.Errorf("Wait() status = %q, want %q", got, tt.state)
			}
			if c := calls(fake); !reflect.DeepEqual(c, [][]string{tt.want}) {
				t.Errorf("calls = %q, want %q", c, [][]string{tt.want})
			}
		})
	}
}

func TestWaitMapsTimeoutToErrTimeout(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "wait", "wS:p1", "--timeout", "500"}, waitTimeout)

	_, err := actions.Service{Runner: fake}.Wait(context.Background(), local, actions.WaitOptions{TimeoutMillis: 500})
	if !errors.Is(err, actions.ErrTimeout) {
		t.Fatalf("Wait() error = %v, want ErrTimeout", err)
	}
}

func TestWaitKeepsOtherAPIErrors(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "wait", "wS:p1"}, `{"error":{"code":"agent_not_found","message":"gone"},"id":"x"}`)

	_, err := actions.Service{Runner: fake}.Wait(context.Background(), local, actions.WaitOptions{})
	if err == nil || errors.Is(err, actions.ErrTimeout) {
		t.Fatalf("Wait() error = %v, want the API error", err)
	}
}

func TestNotifyShowsLocalNotification(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"notification", "show", "dev is idle", "--body", "odoous/w1:p2"},
		`{"id":"x","result":{"shown":true}}`)

	if err := (actions.Service{Runner: fake}).Notify(context.Background(), "dev is idle", "odoous/w1:p2"); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestExtractReply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		screen    string
		text      string
		lines     int
		want      string
		wantAfter bool
	}{
		{"text found, reply follows", "old\n> run the tests\n\nAll 42 tests passed.\n", "run the tests", 40,
			"All 42 tests passed.", true},
		{"uses the last occurrence", "> run the tests\nfirst\n> run the tests\nsecond\n", "run the tests", 40,
			"second", true},
		{"ignores case and spacing", "›   Run   The tests\nok\n", "run the   tests", 40, "ok", true},
		{"multi-line message matches its first line", "> first line\nsecond line\nreply\n", "first line\nsecond line", 40,
			"second line\nreply", true},
		{"text missing falls back to the last lines", "a\nb\nc\nd\n", "nothing here", 2, "c\nd", false},
		{"nothing after the text falls back", "a\nb\n> ping\n", "ping", 2, "b\n> ping", false},
		{"fewer lines than asked", "only\n", "zzz", 40, "only", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, after := actions.ExtractReply(tt.screen, tt.text, tt.lines)
			if got != tt.want || after != tt.wantAfter {
				t.Errorf("ExtractReply() = (%q, %v), want (%q, %v)", got, after, tt.want, tt.wantAfter)
			}
		})
	}
}

func TestAskSendsWaitsAndReadsTheReply(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("odoous", []string{"agent", "prompt", "w1:p2", "run the tests", "--wait", "--timeout", "9000"},
		`{"id":"x","result":{"agent":{"agent_status":"idle"}}}`)
	fake.Reply("odoous", []string{"agent", "read", "w1:p2", "--lines", "200"}, "> run the tests\nAll 42 tests passed.\n")

	got, err := actions.Service{Runner: fake}.Ask(context.Background(), remote, "run the tests",
		actions.AskOptions{TimeoutMillis: 9000, Lines: 40})
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	want := actions.Answer{Status: "idle", Reply: "All 42 tests passed.", AfterMessage: true}
	if got != want {
		t.Errorf("Ask() = %+v, want %+v", got, want)
	}
}

func TestAskAsksForTheStatusWhenPromptDoesNotReturnIt(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "prompt", "wS:p1", "hi", "--wait"}, ok)
	fake.Reply("", []string{"agent", "get", "wS:p1"}, `{"id":"x","result":{"agent":{"agent_status":"blocked"}}}`)
	fake.Reply("", []string{"agent", "read", "wS:p1", "--lines", "200"}, "> hi\nAllow this command? [y/n]\n")

	got, err := actions.Service{Runner: fake}.Ask(context.Background(), local, "hi", actions.AskOptions{Lines: 40})
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if got.Status != "blocked" || got.Reply != "Allow this command? [y/n]" {
		t.Errorf("Ask() = %+v, want the blocked status and the question", got)
	}
}

func TestAskMapsHerdrErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		reply string
		want  error
	}{
		{"already blocked", `{"error":{"code":"agent_blocked","message":"waiting"},"id":"x"}`, actions.ErrBlocked},
		{"timeout", `{"error":{"code":"timeout","message":"late"},"id":"x"}`, actions.ErrTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := herdrtest.New()
			fake.Reply("", []string{"agent", "prompt", "wS:p1", "hi", "--wait"}, tt.reply)

			_, err := actions.Service{Runner: fake}.Ask(context.Background(), local, "hi", actions.AskOptions{})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Ask() error = %v, want %v", err, tt.want)
			}
			if n := len(fake.Calls()); n != 1 {
				t.Errorf("made %d herdr calls, want only the prompt", n)
			}
		})
	}
}

func TestAskRejectsEmptyText(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	if _, err := (actions.Service{Runner: fake}).Ask(context.Background(), local, "  ", actions.AskOptions{}); err == nil {
		t.Fatal("Ask() error = nil, want an error")
	}
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("made %d herdr calls, want 0", n)
	}
}

func TestValidName(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"docs-agent", "a", "x_1", strings.Repeat("a", 32)} {
		if err := actions.ValidName(ok); err != nil {
			t.Errorf("ValidName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "Bad Name", "UPPER", "dots.not", strings.Repeat("a", 33)} {
		if err := actions.ValidName(bad); !errors.Is(err, actions.ErrInvalidName) {
			t.Errorf("ValidName(%q) = %v, want ErrInvalidName", bad, err)
		}
	}
}
