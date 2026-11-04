package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// readOnly son las únicas órdenes de hum que la prueba de humo puede lanzar:
// ninguna cambia nada en herdr. Si hace falta otra, hay que decidirlo aquí, a
// la vista, y no en el script.
var readOnly = map[string]bool{
	"help": true, "ls": true, "machines": true, "attention": true, "id": true,
	"look": true, "completion": true, "__complete": true, "alias ls": true,
}

func TestTheSmokeScriptOnlyRunsReadOnlyCommands(t *testing.T) {
	data, err := os.ReadFile("../../scripts/smoke.ps1")
	if err != nil {
		t.Fatal(err)
	}
	calls := regexp.MustCompile(`(?m)\bInvoke-Hum\s+(\S+)(?:\s+(\S+))?`).FindAllStringSubmatch(string(data), -1)
	if len(calls) < 8 {
		t.Fatalf("only %d Invoke-Hum calls found: the guard may be reading the script wrongly", len(calls))
	}
	for _, c := range calls {
		verb, sub := strings.Trim(c[1], `'"`), strings.Trim(c[2], `'"`)
		switch {
		case verb == "alias":
			if !readOnly["alias "+sub] {
				t.Errorf("the smoke test runs a command that could change things: hum alias %s", sub)
			}
		case !readOnly[verb]:
			t.Errorf("the smoke test runs a command that could change things: hum %s", verb)
		}
	}
}

func TestTheSmokeScriptNeverTouchesTheMachinesItMustNotQuery(t *testing.T) {
	data, err := os.ReadFile("../../scripts/smoke.ps1")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"remoto1", "remoto2"} {
		if strings.Contains(string(data), banned) {
			t.Errorf("the smoke test mentions %s, which must not be queried", banned)
		}
	}
	if !strings.Contains(string(data), "Set-StrictMode") {
		t.Error("the script should run in strict mode")
	}
}
