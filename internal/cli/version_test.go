package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func runVersion(version string, args ...string) output {
	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), cli.Env{
		Runner: herdrtest.New(), Stdout: &stdout, Stderr: &stderr, Version: version,
	}, args)
	return output{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestVersionPrintsTheVersionInEveryForm(t *testing.T) {
	t.Parallel()
	for _, form := range []string{"version", "-v", "--version"} {
		out := runVersion("0.1.2 (windows/amd64)", form)
		if out.code != 0 || out.stdout != "hum 0.1.2 (windows/amd64)\n" || out.stderr != "" {
			t.Errorf("%s: %+v", form, out)
		}
	}
}

func TestVersionWithoutAVersionSaysUnknown(t *testing.T) {
	t.Parallel()
	if out := runVersion("", "version"); out.code != 0 || out.stdout != "hum unknown\n" {
		t.Errorf("%+v", out)
	}
}

func TestVersionRejectsExtraArguments(t *testing.T) {
	t.Parallel()
	out := runVersion("0.1.2", "version", "now")
	if out.code != 2 || out.stdout != "" || !strings.Contains(out.stderr, "version takes no arguments") {
		t.Errorf("%+v", out)
	}
}

func TestHelpListsTheVersionCommand(t *testing.T) {
	t.Parallel()
	if out := runVersion("", "help"); !strings.Contains(out.stdout, "hum version") {
		t.Errorf("the help must mention the command:\n%s", out.stdout)
	}
}
