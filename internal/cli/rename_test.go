package cli_test

import (
	"strings"
	"testing"
)

func TestRenameTabUsesTheTabOfTheAgent(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"tab", "rename", "wS:t1", "Importer review"}, okReply)

	out := run(t, fake, "rename", "--tab", "importer", "Importer", "review")
	if out.code != 0 || !strings.Contains(out.stdout, "Importer review") || out.stderr != "" {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestRenameWorkspaceOnARemoteMachine(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("odoous", []string{"workspace", "rename", "w1", "odoo-main"}, okReply)

	out := run(t, fake, "rename", "--workspace", "odoous/configura", "odoo-main")
	if out.code != 0 || !strings.Contains(out.stdout, "odoo-main") {
		t.Errorf("code=%d stdout=%q stderr=%q", out.code, out.stdout, out.stderr)
	}
}

func TestRenameNeedsExactlyOneKindAndALabel(t *testing.T) {
	t.Parallel()

	for name, args := range map[string][]string{
		"no kind":    {"rename", "importer", "x"},
		"both kinds": {"rename", "--tab", "--workspace", "importer", "x"},
		"no label":   {"rename", "--tab", "importer"},
		"no args":    {"rename"},
	} {
		f := fixture()
		out := run(t, f, args...)
		if out.code != 2 || !strings.Contains(out.stderr, "usage: hum rename") {
			t.Errorf("%s: code=%d stderr=%q, want a usage error", name, out.code, out.stderr)
		}
	}
}

func TestRenameRejectsLabelsHerdrCouldTakeAsAnOption(t *testing.T) {
	t.Parallel()

	for _, label := range []string{"-x", "  ", "bad\x1b[31m"} {
		fake := fixture()
		out := run(t, fake, "rename", "--tab", "importer", label)
		if out.code == 0 {
			t.Errorf("label %q was accepted", label)
		}
		for _, c := range fake.Calls() {
			if len(c.Args) > 1 && c.Args[1] == "rename" {
				t.Errorf("label %q reached herdr: %v", label, c.Args)
			}
		}
	}
}

func TestRenameShowsAHintWhenHerdrFails(t *testing.T) {
	t.Parallel()

	fake := fixture()
	fake.Reply("", []string{"tab", "rename", "wS:t1", "x"}, `{"error":{"code":"tab_not_found","message":"no tab"},"id":"x"}`)

	out := run(t, fake, "rename", "--tab", "importer", "x")
	if out.code != 1 || !strings.Contains(out.stderr, "tab_not_found") {
		t.Errorf("code=%d stderr=%q", out.code, out.stderr)
	}
}
