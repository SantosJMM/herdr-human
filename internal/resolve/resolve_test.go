package resolve_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/inventory"
	"github.com/SantosJMM/herdr-human/internal/resolve"
)

func fixture() []inventory.Target {
	return []inventory.Target{
		{PaneID: "wS:p4", Agent: "claude", Name: "Pane ID work"},
		{PaneID: "wS:p1", Agent: "codex", Name: "importer"},
		{Machine: "odoous", PaneID: "w1:p1", Agent: "codex", Name: "Analiza /opt/ y propone"},
		{Machine: "odoous", PaneID: "w1:p2", Agent: "codex", Name: "Configura Odoo 18"},
	}
}

func TestResolveSingleMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantPane string
		wantMach string
	}{
		{"exact name", "importer", "wS:p1", ""},
		{"case insensitive", "IMPORTER", "wS:p1", ""},
		{"name prefix", "impo", "wS:p1", ""},
		{"name substring", "odoo 18", "w1:p2", "odoous"},
		{"subsequence", "cnf", "w1:p2", "odoous"},
		{"unique agent type", "claude", "wS:p4", ""},
		{"machine scoped exact", "odoous/configura odoo 18", "w1:p2", "odoous"},
		{"machine scoped prefix", "ODOOUS/Configura", "w1:p2", "odoous"},
		{"local scope", "local/importer", "wS:p1", ""},
		{"raw pane id local", "wS:p1", "wS:p1", ""},
		{"raw pane id remote", "w1:p2", "w1:p2", "odoous"},
		{"machine scoped pane id", "odoous/w1:p1", "w1:p1", "odoous"},
		{"name containing a slash", "analiza /opt", "w1:p1", "odoous"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolve.Resolve(fixture(), tt.query)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v", tt.query, err)
			}
			if got.PaneID != tt.wantPane || got.Machine != tt.wantMach {
				t.Errorf("Resolve(%q) = %s/%s, want %s/%s", tt.query, got.Machine, got.PaneID, tt.wantMach, tt.wantPane)
			}
		})
	}
}

func TestResolveAmbiguous(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"agent type across machines", "codex", 3},
		{"agent type within a machine", "odoous/codex", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolve.Resolve(fixture(), tt.query)
			var amb *resolve.AmbiguousError
			if !errors.As(err, &amb) {
				t.Fatalf("error = %v, want *resolve.AmbiguousError", err)
			}
			if len(amb.Candidates) != tt.want {
				t.Errorf("got %d candidates, want %d", len(amb.Candidates), tt.want)
			}
			if !strings.Contains(err.Error(), "odoous/w1:p2") && tt.want == 2 {
				t.Errorf("message %q should list candidates as machine/pane", err.Error())
			}
		})
	}
}

func TestResolveSamePaneIDOnTwoMachinesIsAmbiguous(t *testing.T) {
	t.Parallel()

	targets := append(fixture(), inventory.Target{PaneID: "w1:p2", Agent: "claude", Name: "local twin"})

	_, err := resolve.Resolve(targets, "w1:p2")
	var amb *resolve.AmbiguousError
	if !errors.As(err, &amb) || len(amb.Candidates) != 2 {
		t.Fatalf("error = %v, want an ambiguity between the two w1:p2", err)
	}

	got, err := resolve.Resolve(targets, "local/w1:p2")
	if err != nil || got.Machine != "" {
		t.Fatalf("Resolve(local/w1:p2) = %+v, %v; want the local one", got, err)
	}
}

func TestResolveExactBeatsPrefix(t *testing.T) {
	t.Parallel()

	targets := []inventory.Target{
		{PaneID: "w1:p1", Agent: "codex", Name: "api-gateway"},
		{PaneID: "w1:p2", Agent: "codex", Name: "api"},
	}

	got, err := resolve.Resolve(targets, "api")
	if err != nil || got.PaneID != "w1:p2" {
		t.Fatalf("Resolve(api) = %+v, %v; want the exact match w1:p2", got, err)
	}
}

func TestResolveNotFound(t *testing.T) {
	t.Parallel()

	for _, query := range []string{"nothing", "foo/importer", "odoous/importer"} {
		_, err := resolve.Resolve(fixture(), query)
		var nf *resolve.NotFoundError
		if !errors.As(err, &nf) {
			t.Errorf("Resolve(%q) error = %v, want *resolve.NotFoundError", query, err)
		}
	}
}

func TestResolveRejectsEmptyQuery(t *testing.T) {
	t.Parallel()

	for _, query := range []string{"", "   ", "odoous/"} {
		if _, err := resolve.Resolve(fixture(), query); err == nil {
			t.Errorf("Resolve(%q) error = nil, want an error", query)
		}
	}
}
