package inventory_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/inventory"
)

func drain(t *testing.T, s inventory.Stream) []inventory.Update {
	t.Helper()

	var got []inventory.Update
	timeout := time.After(5 * time.Second)
	for {
		select {
		case u, ok := <-s.Updates:
			if !ok {
				return got
			}
			got = append(got, u)
		case <-timeout:
			t.Fatalf("the update channel never closed; got %d updates", len(got))
		}
	}
}

func TestStartAnnouncesTheServersAndDeliversEachOneAsItAnswers(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.Hang("odoous", agentList)

	s, err := inventory.Start(context.Background(), fake, inventory.Options{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if want := []string{"", "remoto2", "odoous"}; !slices.Equal(s.Machines, want) {
		t.Errorf("Machines = %v, want %v (local first, then machine list order)", s.Machines, want)
	}

	got := drain(t, s)
	if len(got) != 3 {
		t.Fatalf("got %d updates, want one per server", len(got))
	}
	last := got[2]
	if last.Machine != "odoous" || !errors.Is(last.Err, context.DeadlineExceeded) {
		t.Errorf("the slow server should arrive last with a deadline error, got %+v", last)
	}
	for _, u := range got[:2] {
		if u.Err != nil {
			t.Errorf("%q failed: %v", u.Machine, u.Err)
		}
		if u.Machine == "" && len(u.Targets) != 2 {
			t.Errorf("local targets = %d, want 2", len(u.Targets))
		}
	}
}

func TestStartDeliversALocalFailureInsteadOfAborting(t *testing.T) {
	t.Parallel()

	boom := errors.New("socket closed")
	fake := newFake()
	fake.Fail("", agentList, boom)

	s, err := inventory.Start(context.Background(), fake, inventory.Options{})
	if err != nil {
		t.Fatalf("Start() error = %v, want the failure delivered as an update", err)
	}
	for _, u := range drain(t, s) {
		if u.Machine == "" && !errors.Is(u.Err, boom) {
			t.Errorf("local update error = %v, want the socket failure", u.Err)
		}
	}
}

func TestStartRejectsUnknownMachines(t *testing.T) {
	t.Parallel()

	if _, err := inventory.Start(context.Background(), newFake(), inventory.Options{Machines: []string{"nope"}}); err == nil {
		t.Errorf("Start() accepted an unknown machine")
	}
}

func TestStartLocalOnlyHasASingleServer(t *testing.T) {
	t.Parallel()

	fake := newFake()
	s, err := inventory.Start(context.Background(), fake, inventory.Options{LocalOnly: true})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if want := []string{""}; !slices.Equal(s.Machines, want) {
		t.Errorf("Machines = %v, want %v", s.Machines, want)
	}
	if got := drain(t, s); len(got) != 1 {
		t.Errorf("got %d updates, want 1", len(got))
	}
}

func TestLoadMachineQueriesOnlyThatServer(t *testing.T) {
	t.Parallel()

	fake := newFake()
	targets, err := inventory.LoadMachine(context.Background(), fake, "odoous", 0)
	if err != nil || len(targets) != 1 || targets[0].Machine != "odoous" {
		t.Fatalf("LoadMachine = %+v, %v", targets, err)
	}
	for _, c := range fake.Calls() {
		if c.Machine != "odoous" {
			t.Errorf("queried %q, want only odoous", c.Machine)
		}
	}
}

func TestLoadMachineHonoursItsOwnTimeout(t *testing.T) {
	t.Parallel()

	fake := newFake()
	fake.Hang("odoous", agentList)
	if _, err := inventory.LoadMachine(context.Background(), fake, "odoous", 50*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want a deadline error", err)
	}
}
