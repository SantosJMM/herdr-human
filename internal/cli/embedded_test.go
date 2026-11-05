package cli_test

import (
	"strings"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func TestEmbeddedFlagIsAcceptedButStillNeedsATerminal(t *testing.T) {
	t.Parallel()
	// Pasar el flag no cambia que haga falta una terminal: se llega hasta ese error,
	// no a un error de uso.
	for _, args := range [][]string{{"palette", "--embedded"}, {"pick", "--embedded"}} {
		out := run(t, herdrtest.New(), args...)
		if out.code != 1 || !strings.Contains(out.stderr, "interactive terminal") {
			t.Errorf("%v: want the 'needs a terminal' failure (1), got %+v", args, out)
		}
	}
}

func TestEmbeddedFlagUsageErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"palette", "--embedded", "extra"}, {"palette", "--wat"}, {"pick", "--embedded", "extra"}} {
		if out := run(t, herdrtest.New(), args...); out.code != 2 {
			t.Errorf("%v: want usage error (2), got %+v", args, out)
		}
	}
}
