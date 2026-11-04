package actions_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

const tabCreated = `{"id":"x","result":{"type":"tab_created","root_pane":{"pane_id":"wS:pA","tab_id":"wS:tA","workspace_id":"wS"},"tab":{"tab_id":"wS:tA","workspace_id":"wS","label":"docs"}}}`

const agentStarted = `{"id":"x","result":{"type":"agent_started","agent":{"pane_id":"wS:pA","agent_status":"idle"},"argv":["claude"]}}`

const okReply = `{"id":"x","result":{"type":"ok"}}`

// creator prepara un servidor falso que contesta a la secuencia completa.
func creator(machine string) *herdrtest.Fake {
	f := herdrtest.New()
	f.Reply(machine, []string{"tab", "create", "--cwd", `C:\work`, "--label", "docs", "--focus"}, tabCreated)
	f.Reply(machine, []string{"agent", "start", "docs", "--kind", "claude", "--pane", "wS:pA"}, agentStarted)
	f.Reply(machine, []string{"pane", "rename", "wS:pA", "docs"}, okReply)
	return f
}

func argsOf(f *herdrtest.Fake) [][]string {
	var out [][]string
	for _, c := range f.Calls() {
		out = append(out, c.Args)
	}
	return out
}

func TestCreateRunsTabAgentAndNameInOrder(t *testing.T) {
	t.Parallel()

	f := creator("")
	got, err := actions.Service{Runner: f}.Create(context.Background(), "", actions.CreateOptions{
		Kind: "claude", Dir: `C:\work`, Name: "docs", Focus: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := [][]string{
		{"tab", "create", "--cwd", `C:\work`, "--label", "docs", "--focus"},
		{"agent", "start", "docs", "--kind", "claude", "--pane", "wS:pA"},
		{"pane", "rename", "wS:pA", "docs"},
	}
	if !reflect.DeepEqual(argsOf(f), want) {
		t.Errorf("calls = %v\nwant    %v", argsOf(f), want)
	}
	if got.TabID != "wS:tA" || got.PaneID != "wS:pA" || got.WorkspaceID != "wS" || !got.Started || !got.Named {
		t.Errorf("Created = %+v", got)
	}
}

func TestCreateTargetsTheRequestedMachine(t *testing.T) {
	t.Parallel()

	f := creator("odoous")
	if _, err := (actions.Service{Runner: f}).Create(context.Background(), "odoous", actions.CreateOptions{
		Kind: "claude", Dir: `C:\work`, Name: "docs", Focus: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, c := range f.Calls() {
		if c.Machine != "odoous" {
			t.Errorf("call %v went to %q, want odoous", c.Args, c.Machine)
		}
	}
}

func TestCreateWithoutNameOrFolderUsesTheKindAndNoCwd(t *testing.T) {
	t.Parallel()

	f := herdrtest.New()
	f.Reply("", []string{"tab", "create", "--label", "codex", "--no-focus"}, tabCreated)
	f.Reply("", []string{"agent", "start", "codex", "--kind", "codex", "--pane", "wS:pA", "--timeout", "60000"}, agentStarted)

	got, err := actions.Service{Runner: f}.Create(context.Background(), "", actions.CreateOptions{
		Kind: "codex", TimeoutMillis: 60000,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(f.Calls()) != 2 {
		t.Errorf("calls = %v, want only tab create and agent start (no pane rename without a name)", argsOf(f))
	}
	if got.Named {
		t.Errorf("Named = true without a requested name")
	}
}

func TestCreateValidatesBeforeCreatingAnything(t *testing.T) {
	t.Parallel()

	for name, opts := range map[string]actions.CreateOptions{
		"unknown kind": {Kind: "frobnicator"},
		"empty kind":   {},
		"bad name":     {Kind: "claude", Name: "Bad Name"},
	} {
		f := herdrtest.New()
		_, err := actions.Service{Runner: f}.Create(context.Background(), "", opts)
		if err == nil {
			t.Errorf("%s: Create succeeded", name)
		}
		if len(f.Calls()) != 0 {
			t.Errorf("%s: herdr was called before validating: %v", name, argsOf(f))
		}
	}
	if _, err := (actions.Service{Runner: herdrtest.New()}).Create(context.Background(), "", actions.CreateOptions{Kind: "x"}); !errors.Is(err, actions.ErrUnknownKind) {
		t.Errorf("error = %v, want ErrUnknownKind", err)
	}
}

func TestCreateReportsWhichStepFailedAndWhatWasLeft(t *testing.T) {
	t.Parallel()

	boom := errors.New("herdr says no")
	tests := []struct {
		name        string
		fail        []string
		step        string
		wantTab     bool
		wantStarted bool
		wantCalls   int
	}{
		{"tab", []string{"tab", "create", "--cwd", `C:\work`, "--label", "docs", "--focus"}, "tab", false, false, 1},
		{"agent", []string{"agent", "start", "docs", "--kind", "claude", "--pane", "wS:pA"}, "agent", true, false, 2},
		{"name", []string{"pane", "rename", "wS:pA", "docs"}, "name", true, true, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := creator("")
			f.Fail("", tt.fail, boom)
			got, err := actions.Service{Runner: f}.Create(context.Background(), "", actions.CreateOptions{
				Kind: "claude", Dir: `C:\work`, Name: "docs", Focus: true,
			})
			var ce *actions.CreateError
			if !errors.As(err, &ce) || ce.Step != tt.step || !errors.Is(err, boom) {
				t.Fatalf("error = %v, want a CreateError for step %q wrapping the cause", err, tt.step)
			}
			if (ce.Created.TabID != "") != tt.wantTab || ce.Created.Started != tt.wantStarted {
				t.Errorf("Created = %+v, want tab=%v started=%v", ce.Created, tt.wantTab, tt.wantStarted)
			}
			if got != ce.Created {
				t.Errorf("returned %+v but the error says %+v", got, ce.Created)
			}
			if len(f.Calls()) != tt.wantCalls {
				t.Errorf("calls = %v, want %d (no step after the failure, nothing undone)", argsOf(f), tt.wantCalls)
			}
		})
	}
}

func TestCreateFailsClearlyWhenTheTabHasNoPane(t *testing.T) {
	t.Parallel()

	f := herdrtest.New()
	f.Reply("", []string{"tab", "create", "--label", "claude", "--no-focus"}, `{"id":"x","result":{"type":"tab_created","tab":{"tab_id":"wS:tA"}}}`)
	_, err := actions.Service{Runner: f}.Create(context.Background(), "", actions.CreateOptions{Kind: "claude"})
	var ce *actions.CreateError
	if !errors.As(err, &ce) || ce.Step != "tab" || ce.Created.TabID != "wS:tA" {
		t.Errorf("error = %v, want a tab-step CreateError that remembers the tab", err)
	}
	if len(f.Calls()) != 1 {
		t.Errorf("calls = %v, the agent must not start without a pane", argsOf(f))
	}
}
