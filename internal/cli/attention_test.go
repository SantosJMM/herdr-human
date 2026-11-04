package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

// agentsJSON arma una respuesta de `agent list` con un agente por estado.
func agentsJSON(statuses ...string) string {
	rows := make([]string, 0, len(statuses))
	for i, s := range statuses {
		rows = append(rows, `{"agent":"codex","agent_status":"`+s+`","focused":false,"pane_id":"wS:p`+string(rune('1'+i))+
			`","tab_id":"wS:t1","workspace_id":"wS","name":"agent`+string(rune('1'+i))+`"}`)
	}
	return `{"id":"x","result":{"agents":[` + strings.Join(rows, ",") + `]}}`
}

func attentionFixture(listReplies ...string) *herdrtest.Fake {
	f := herdrtest.New()
	f.Sequence("", []string{"agent", "list"}, listReplies...)
	f.Reply("", []string{"workspace", "list"}, localWorkspaces)
	f.Reply("", []string{"api", "snapshot"}, `{"error":{"code":"unknown_command","message":"no snapshot"},"id":"x"}`)
	return f
}

func TestAttentionGroupsByUrgency(t *testing.T) {
	t.Parallel()

	fake := attentionFixture(agentsJSON("idle", "working", "done", "blocked", "idle"))
	out := run(t, fake, "attention", "--local")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}

	needs := strings.Index(out.stdout, "Needs you (2)")
	working := strings.Index(out.stdout, "Working (1)")
	idle := strings.Index(out.stdout, "Idle: 2 more (hum ls)")
	if needs < 0 || working < needs || idle < working {
		t.Fatalf("stdout = %q, want Needs you, Working and Idle sections in that order", out.stdout)
	}
	blocked := strings.Index(out.stdout, "agent4")
	done := strings.Index(out.stdout, "agent3")
	if blocked < 0 || done < blocked || done > working {
		t.Errorf("stdout = %q, want blocked before done inside Needs you", out.stdout)
	}
	if strings.Contains(out.stdout, "agent1") {
		t.Errorf("stdout = %q, idle agents must only be counted", out.stdout)
	}
}

func TestAttentionWhenNothingNeedsYou(t *testing.T) {
	t.Parallel()

	out := run(t, attentionFixture(agentsJSON("idle", "idle")), "attention", "--local")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, want := range []string{"Nothing needs you.", "Idle: 2 more (hum ls)"} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", out.stdout, want)
		}
	}
}

func TestAttentionJSON(t *testing.T) {
	t.Parallel()

	out := run(t, attentionFixture(agentsJSON("blocked", "working", "idle")), "attention", "--local", "--json")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, want := range []string{`"needs_you"`, `"working"`, `"idle": 1`, `"pane_id": "wS:p1"`} {
		if !strings.Contains(out.stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", out.stdout, want)
		}
	}
}

func TestAttentionUsageErrors(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"extra argument":       {"attention", "now"},
		"notify without watch": {"attention", "--notify"},
		"json with watch":      {"attention", "--watch", "--json"},
		"bad interval":         {"attention", "--watch", "--interval", "0s"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if out := run(t, attentionFixture(agentsJSON("idle")), args...); out.code != 2 {
				t.Errorf("code = %d, want 2 (stderr %q)", out.code, out.stderr)
			}
		})
	}
}

// runWatch ejecuta `attention --watch` con una pausa falsa que corta tras n sondeos.
func runWatch(t *testing.T, fake *herdrtest.Fake, polls int, terminal bool, args ...string) output {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sleeps := 0
	var stdout, stderr bytes.Buffer
	code := cli.Run(ctx, cli.Env{
		Runner: fake, Stdout: &stdout, Stderr: &stderr, Terminal: terminal,
		Sleep: func(ctx context.Context, d time.Duration) error {
			sleeps++
			if sleeps >= polls {
				cancel()
			}
			return ctx.Err()
		},
	}, append([]string{"attention", "--watch", "--local"}, args...))
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestWatchNotifiesOncePerTransition(t *testing.T) {
	t.Parallel()

	fake := attentionFixture(
		agentsJSON("working", "idle"), // línea base: no avisa
		agentsJSON("blocked", "idle"), // agent1 pasa a blocked: avisa
		agentsJSON("blocked", "idle"), // sin cambio: no repite
		agentsJSON("blocked", "done"), // agent2 pasa a done: avisa
	)
	fake.Reply("", []string{"notification", "show", "agent1 is blocked", "--body", "local/wS:p1"}, notifyReply)
	fake.Reply("", []string{"notification", "show", "agent2 is done", "--body", "local/wS:p2"}, notifyReply)

	out := runWatch(t, fake, 4, false, "--notify")
	if out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	var shown []string
	for _, c := range fake.Calls() {
		if len(c.Args) > 1 && c.Args[0] == "notification" {
			shown = append(shown, c.Args[2])
		}
	}
	if want := "agent1 is blocked,agent2 is done"; strings.Join(shown, ",") != want {
		t.Errorf("notifications = %q, want %q", shown, want)
	}
}

func TestWatchDoesNotNotifyWithoutTheFlag(t *testing.T) {
	t.Parallel()

	fake := attentionFixture(agentsJSON("working"), agentsJSON("blocked"))
	if out := runWatch(t, fake, 2, false); out.code != 0 {
		t.Fatalf("code = %d, stderr %q", out.code, out.stderr)
	}
	for _, c := range fake.Calls() {
		if len(c.Args) > 0 && c.Args[0] == "notification" {
			t.Errorf("unexpected notification call %q", c.Args)
		}
	}
}

func TestWatchRedrawsInPlaceOnlyOnATerminal(t *testing.T) {
	t.Parallel()

	const clear = "\x1b[H\x1b[2J"
	frames := []string{agentsJSON("working"), agentsJSON("blocked")}

	onTerminal := runWatch(t, attentionFixture(frames...), 2, true)
	if got := strings.Count(onTerminal.stdout, clear); got != 2 {
		t.Errorf("terminal output cleared %d times, want 2 (stdout %q)", got, onTerminal.stdout)
	}
	piped := runWatch(t, attentionFixture(frames...), 2, false)
	if strings.Contains(piped.stdout, "\x1b") {
		t.Errorf("piped output has escape codes: %q", piped.stdout)
	}
	if !strings.Contains(piped.stdout, "Working (1)") || !strings.Contains(piped.stdout, "Needs you (1)") {
		t.Errorf("piped output = %q, want both frames", piped.stdout)
	}
}
