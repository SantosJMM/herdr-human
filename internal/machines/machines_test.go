package machines_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
	"github.com/SantosJMM/herdr-human/internal/machines"
)

var listCmd = []string{"machine", "list", "--json"}

func profile(id, label string, enabled bool) string {
	state := "false"
	if enabled {
		state = "true"
	}
	return `{"id":"` + id + `","label":"` + label + `","target":"t-` + label + `","session":"default","enabled":` + state + `,"selected":false}`
}

func listJSON(profiles ...string) string {
	return "[" + strings.Join(profiles, ",") + "]"
}

func calls(f *herdrtest.Fake, verb string) int {
	n := 0
	for _, c := range f.Calls() {
		if len(c.Args) >= 2 && c.Args[0] == "machine" && c.Args[1] == verb {
			n++
		}
	}
	return n
}

func TestListSortsByLabelIgnoringCase(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", listCmd, listJSON(profile("1", "remoto2", true), profile("2", "WSL Ubuntu", false), profile("3", "odoous", true)))

	got, err := machines.Service{Runner: fake}.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var labels []string
	for _, p := range got {
		labels = append(labels, p.Label)
	}
	if want := "odoous,remoto2,WSL Ubuntu"; strings.Join(labels, ",") != want {
		t.Errorf("labels = %q, want %q", labels, want)
	}
}

func TestFind(t *testing.T) {
	t.Parallel()

	profiles := []herdr.Profile{
		{ID: "aaa111", Label: "odoous"},
		{ID: "bbb222", Label: "odoo-old"},
		{ID: "ccc333", Label: "remoto2"},
	}

	tests := []struct {
		name    string
		query   string
		wantID  string
		wantErr string
	}{
		{"exact label", "odoous", "aaa111", ""},
		{"label ignoring case", "REMOTO2", "ccc333", ""},
		{"profile id", "bbb222", "bbb222", ""},
		{"unique label prefix", "remo", "ccc333", ""},
		{"exact beats prefix", "odoous", "aaa111", ""},
		{"ambiguous prefix", "odoo", "", "matches 2"},
		{"unknown", "nope", "", `no saved machine matches "nope"`},
		{"empty", "  ", "", "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := machines.Find(profiles, tt.query)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Find(%q) error = %v, want it to contain %q", tt.query, err, tt.wantErr)
				}
				return
			}
			if err != nil || got.ID != tt.wantID {
				t.Errorf("Find(%q) = %+v, %v; want id %s", tt.query, got, err, tt.wantID)
			}
		})
	}
}

func TestSetEnabledChangesAndConfirmsState(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	// Lectura previa (apagada) y lectura de confirmación (encendida).
	fake.Sequence("", listCmd, listJSON(profile("id1", "remoto2", false)), listJSON(profile("id1", "remoto2", true)))
	fake.Reply("", []string{"machine", "enable", "id1"}, "ok")

	svc := machines.Service{Runner: fake}
	res, err := svc.SetEnabled(context.Background(), herdr.Profile{ID: "id1", Label: "remoto2", Enabled: false}, true)
	if err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if !res.Changed || !res.Profile.Enabled {
		t.Errorf("result = %+v, want a confirmed change", res)
	}
	if n := calls(fake, "enable"); n != 1 {
		t.Errorf("enable called %d times, want exactly 1", n)
	}
}

func TestSetEnabledIsANoOpWhenAlreadyInTheWantedState(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", listCmd, listJSON(profile("id1", "odoous", true)))

	res, err := machines.Service{Runner: fake}.SetEnabled(context.Background(), herdr.Profile{ID: "id1", Label: "odoous", Enabled: true}, true)
	if err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if res.Changed {
		t.Errorf("result = %+v, want Changed=false", res)
	}
	if n := calls(fake, "enable") + calls(fake, "disable"); n != 0 {
		t.Errorf("made %d mutations, want 0", n)
	}
}

func TestSetEnabledRefusesWhenTheProfileChangedElsewhere(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	// La persona vio "apagada", pero otro cliente ya la encendió.
	fake.Reply("", listCmd, listJSON(profile("id1", "remoto2", true)))

	_, err := machines.Service{Runner: fake}.SetEnabled(context.Background(), herdr.Profile{ID: "id1", Label: "remoto2", Enabled: false}, false)
	if !errors.Is(err, machines.ErrStateChanged) {
		t.Fatalf("SetEnabled() error = %v, want ErrStateChanged", err)
	}
	if n := calls(fake, "enable") + calls(fake, "disable"); n != 0 {
		t.Errorf("made %d mutations, want 0", n)
	}
}

func TestSetEnabledRefusesWhenTheProfileWasRemoved(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", listCmd, listJSON(profile("other", "odoous", true)))

	_, err := machines.Service{Runner: fake}.SetEnabled(context.Background(), herdr.Profile{ID: "id1", Label: "remoto2"}, true)
	if !errors.Is(err, machines.ErrProfileGone) {
		t.Fatalf("SetEnabled() error = %v, want ErrProfileGone", err)
	}
}

func TestSetEnabledReportsAnUnconfirmedOutcomeWithoutRetrying(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	// herdr acepta el comando pero el estado no cambia.
	fake.Reply("", listCmd, listJSON(profile("id1", "remoto2", false)))
	fake.Reply("", []string{"machine", "enable", "id1"}, "ok")

	_, err := machines.Service{Runner: fake}.SetEnabled(context.Background(), herdr.Profile{ID: "id1", Label: "remoto2"}, true)
	if !errors.Is(err, machines.ErrUnconfirmed) {
		t.Fatalf("SetEnabled() error = %v, want ErrUnconfirmed", err)
	}
	if n := calls(fake, "enable"); n != 1 {
		t.Errorf("enable called %d times, want 1 (a mutation of uncertain outcome is never retried)", n)
	}
}

func TestSetEnabledDoesNotRetryAFailedMutation(t *testing.T) {
	t.Parallel()

	boom := errors.New("socket closed")
	fake := herdrtest.New()
	fake.Reply("", listCmd, listJSON(profile("id1", "remoto2", false)))
	fake.Fail("", []string{"machine", "enable", "id1"}, boom)

	_, err := machines.Service{Runner: fake}.SetEnabled(context.Background(), herdr.Profile{ID: "id1", Label: "remoto2"}, true)
	if !errors.Is(err, boom) {
		t.Fatalf("SetEnabled() error = %v, want the mutation error", err)
	}
	if n := calls(fake, "enable"); n != 1 {
		t.Errorf("enable called %d times, want 1", n)
	}
}
