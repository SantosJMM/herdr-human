package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/actions"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/inventory"
)

var remoteTarget = inventory.Target{Machine: "odoous", PaneID: "w1:p2", Name: "Configura Odoo 18 | odoo-inc | 5h", Agent: "codex"}

const (
	okJSON   = `{"id":"x","result":{"type":"ok"}}`
	idleJSON = `{"id":"x","result":{"agent":{"agent_status":"idle"}}}`
)

func newPickActions(f *herdrtest.Fake, copyFn func(string) error) pickActions {
	return pickActions{svc: actions.Service{Runner: f}, copy: copyFn}
}

func TestPickActionsSayAndRenameTargetTheRightMachine(t *testing.T) {
	t.Parallel()

	f := herdrtest.New()
	f.Reply("odoous", []string{"agent", "prompt", "w1:p2", "hello"}, okJSON)
	f.Reply("odoous", []string{"agent", "rename", "w1:p2", "docs"}, okJSON)
	f.Reply("odoous", []string{"pane", "rename", "w1:p2", "docs"}, okJSON)
	p := newPickActions(f, nil)

	if err := p.Say(context.Background(), remoteTarget, "hello"); err != nil {
		t.Errorf("Say: %v", err)
	}
	if err := p.Rename(context.Background(), remoteTarget, "docs"); err != nil {
		t.Errorf("Rename: %v", err)
	}
	if err := p.Rename(context.Background(), remoteTarget, "Bad Name"); !errors.Is(err, actions.ErrInvalidName) {
		t.Errorf("Rename with a bad name = %v, want ErrInvalidName", err)
	}
}

func TestPickActionsAskReturnsTheReplyAndHowItWasFound(t *testing.T) {
	t.Parallel()

	f := herdrtest.New()
	f.Reply("odoous", []string{"agent", "prompt", "w1:p2", "run it", "--wait"}, idleJSON)
	f.Reply("odoous", []string{"agent", "read", "w1:p2", "--lines", "200"}, "> run it\nAll good.\n")

	r, err := newPickActions(f, nil).Ask(context.Background(), remoteTarget, "run it")
	if err != nil || r.Status != "idle" || r.Text != "All good." || !r.AfterMessage || r.Lines != actions.DefaultReplyLines {
		t.Errorf("Ask = %+v, %v", r, err)
	}
}

func TestPickActionsWaitNotifiesLocallyAndReturnsTheStatus(t *testing.T) {
	t.Parallel()

	f := herdrtest.New()
	f.Reply("odoous", []string{"agent", "wait", "w1:p2"}, idleJSON)
	f.Reply("", []string{"notification", "show", "Configura Odoo 18 is idle", "--body", "odoous/w1:p2"}, okJSON)

	status, err := newPickActions(f, nil).WaitNotify(context.Background(), remoteTarget)
	if err != nil || status != "idle" {
		t.Errorf("WaitNotify = %q, %v", status, err)
	}
	last := f.Calls()[len(f.Calls())-1]
	if last.Machine != "" || last.Args[0] != "notification" {
		t.Errorf("the notification must be shown on this machine, got %+v", last)
	}
}

func TestPickActionsWaitSurvivesAFailedNotification(t *testing.T) {
	t.Parallel()

	f := herdrtest.New()
	f.Reply("odoous", []string{"agent", "wait", "w1:p2"}, idleJSON)
	f.Fail("", []string{"notification", "show", "Configura Odoo 18 is idle", "--body", "odoous/w1:p2"}, errors.New("no ui"))

	status, err := newPickActions(f, nil).WaitNotify(context.Background(), remoteTarget)
	if err != nil || status != "idle" {
		t.Errorf("WaitNotify = %q, %v, want the status despite the notification failure", status, err)
	}
}

func TestPickActionsCopyNeedsAClipboard(t *testing.T) {
	t.Parallel()

	var got string
	p := newPickActions(herdrtest.New(), func(s string) error { got = s; return nil })
	if err := p.Copy("w1:p2"); err != nil || got != "w1:p2" {
		t.Errorf("Copy = %v, copied %q", err, got)
	}
	if err := newPickActions(herdrtest.New(), nil).Copy("x"); err == nil || !strings.Contains(err.Error(), "clipboard") {
		t.Errorf("Copy without a clipboard = %v, want a clear error", err)
	}
}
