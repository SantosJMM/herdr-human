package herdr_test

import (
	"context"
	"errors"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/herdr"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func TestCallDecodesResult(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"workspace", "list"}, `{"id":"x","result":{"n":3}}`)

	var out struct {
		N int `json:"n"`
	}
	if err := herdr.Call(context.Background(), fake, "", []string{"workspace", "list"}, &out); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if out.N != 3 {
		t.Errorf("N = %d, want 3", out.N)
	}
}

func TestCallSurfacesAPIError(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"agent", "rename", "w1:p9", "x"},
		`{"error":{"code":"agent_not_found","message":"agent target w1:p9 not found"},"id":"x"}`)

	err := herdr.Call(context.Background(), fake, "", []string{"agent", "rename", "w1:p9", "x"}, nil)

	var apiErr *herdr.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want *herdr.APIError", err)
	}
	if apiErr.Code != "agent_not_found" {
		t.Errorf("Code = %q, want agent_not_found", apiErr.Code)
	}
}

func TestCallRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"x"}, `not json`)

	if err := herdr.Call(context.Background(), fake, "", []string{"x"}, nil); err == nil {
		t.Fatal("Call() error = nil, want a decode error")
	}
}

func TestCallPropagatesRunnerError(t *testing.T) {
	t.Parallel()

	boom := errors.New("ssh: connection refused")
	fake := herdrtest.New()
	fake.Fail("odoous", []string{"agent", "list"}, boom)

	err := herdr.Call(context.Background(), fake, "odoous", []string{"agent", "list"}, nil)
	if !errors.Is(err, boom) {
		t.Fatalf("Call() error = %v, want it to wrap the runner error", err)
	}
}

func TestListMachines(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"machine", "list"},
		"id1\tremoto2\tservidor-2\tdefault\tenabled\r\n"+
			"id2\tWSL Ubuntu\tssh://u@127.0.0.1:2222\tdefault\tdisabled\r\n"+
			"\r\n"+
			"broken-line\r\n"+
			"id4\todoous\tservidor-odoous\tdefault\tenabled\r\n")

	got, err := herdr.ListMachines(context.Background(), fake)
	if err != nil {
		t.Fatalf("ListMachines() error = %v", err)
	}

	want := []herdr.Machine{
		{ID: "id1", Label: "remoto2", Target: "servidor-2", Enabled: true},
		{ID: "id2", Label: "WSL Ubuntu", Target: "ssh://u@127.0.0.1:2222", Enabled: false},
		{ID: "id4", Label: "odoous", Target: "servidor-odoous", Enabled: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d machines, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("machine[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestListProfiles(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"machine", "list", "--json"},
		`[{"id":"a1","label":"odoous","target":"servidor-odoous","session":"default","enabled":true,"selected":false},`+
			`{"id":"b2","label":"remoto2","target":"srv","session":"main","enabled":false,"selected":false}]`)

	got, err := herdr.ListProfiles(context.Background(), fake)
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	want := []herdr.Profile{
		{ID: "a1", Label: "odoous", Target: "servidor-odoous", Session: "default", Enabled: true},
		{ID: "b2", Label: "remoto2", Target: "srv", Session: "main", Enabled: false},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("ListProfiles() = %+v, want %+v", got, want)
	}
}

func TestListProfilesRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"machine", "list", "--json"}, `not json`)

	if _, err := herdr.ListProfiles(context.Background(), fake); err == nil {
		t.Fatal("ListProfiles() error = nil, want a decode error")
	}
}

func TestSetEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		verb    string
	}{
		{"enable", true, "enable"},
		{"disable", false, "disable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := herdrtest.New()
			fake.Reply("", []string{"machine", tt.verb, "a1"}, "ok\n")

			if err := herdr.SetEnabled(context.Background(), fake, "a1", tt.enabled); err != nil {
				t.Fatalf("SetEnabled() error = %v", err)
			}
		})
	}
}

func TestSetEnabledSurfacesAPIError(t *testing.T) {
	t.Parallel()

	fake := herdrtest.New()
	fake.Reply("", []string{"machine", "enable", "zzz"}, `{"error":{"code":"machine_not_found","message":"no such profile"},"id":"x"}`)

	err := herdr.SetEnabled(context.Background(), fake, "zzz", true)
	var apiErr *herdr.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "machine_not_found" {
		t.Fatalf("SetEnabled() error = %v, want the API error", err)
	}
}
