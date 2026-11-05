package palette

import (
	"context"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// --- sin herdr: la lista de exclusiones es coherente ---

func TestExcludedCommandsAreExplainedAndNotAlsoInTheCatalog(t *testing.T) {
	t.Parallel()
	inCatalog, err := CatalogCommands()
	if err != nil {
		t.Fatal(err)
	}
	for command, reason := range Excluded() {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%q is excluded without a reason", command)
		}
		if ids, both := inCatalog[command]; both {
			t.Errorf("%q is both in the catalog (%v) and excluded (%s): pick one", command, ids, reason)
		}
	}
}

// --- con herdr: no se nos escapa ningún comando ---

// herdrOutput ejecuta `herdr args...` y devuelve stdout y stderr juntos.
func herdrOutput(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "herdr", args...).CombinedOutput()
	return string(out)
}

// subcommands lee la sección "Commands:" de la ayuda de clap de un comando.
func subcommands(help string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(strings.ReplaceAll(help, "\r", ""), "\n") {
		switch {
		case strings.HasPrefix(line, "Commands:"):
			in = true
		case in && strings.TrimSpace(line) == "":
			return out
		case in:
			if f := strings.Fields(line); len(f) > 0 && f[0] != "help" {
				out = append(out, f[0])
			}
		}
	}
	return out
}

var topLevelNoun = regexp.MustCompile(`(?m)^\s+herdr ([a-z-]+) <subcommand>`)

// herdrLeaves recorre el árbol de comandos del herdr instalado y devuelve las
// rutas de los comandos hoja ("pane split", "plugin action invoke"...).
func herdrLeaves(t *testing.T) []string {
	t.Helper()
	// La ayuda de nivel superior de herdr no es de clap: los sustantivos salen de
	// sus líneas de uso. `plugin` y `server` no aparecen ahí con <subcommand> y
	// se añaden a mano. Los comandos sueltos de nivel superior (status, update,
	// completion) no se recorren.
	nouns := map[string]bool{"plugin": true, "server": true}
	for _, m := range topLevelNoun.FindAllStringSubmatch(herdrOutput(t, "--help"), -1) {
		nouns[m[1]] = true
	}

	var leaves []string
	var walk func(path []string)
	walk = func(path []string) {
		subs := subcommands(herdrOutput(t, append(slices.Clone(path), "--help")...))
		if len(subs) == 0 || len(path) > 3 {
			leaves = append(leaves, strings.Join(path, " "))
			return
		}
		for _, s := range subs {
			walk(append(slices.Clone(path), s))
		}
	}
	for noun := range nouns {
		walk([]string{noun})
	}
	sort.Strings(leaves)
	return leaves
}

// Este test responde a «¿cómo sabemos que están todos los comandos?»: recorre el
// herdr instalado y exige que cada comando esté en el catálogo o excluido con
// motivo. Si una versión nueva de herdr añade comandos, falla y obliga a decidir.
// Si herdr no está instalado (por ejemplo, en CI), se salta.
func TestCatalogAndExclusionsAccountForEveryInstalledHerdrCommand(t *testing.T) {
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Skip("herdr is not installed; this check compares the catalog with the real CLI")
	}
	inCatalog, err := CatalogCommands()
	if err != nil {
		t.Fatal(err)
	}
	skipped := Excluded()

	leaves := herdrLeaves(t)
	if len(leaves) < 50 {
		t.Fatalf("only %d herdr commands found: the help parser is probably broken, not herdr", len(leaves))
	}
	real := map[string]bool{}
	var missing []string
	for _, leaf := range leaves {
		real[leaf] = true
		_, covered := inCatalog[leaf]
		_, left := skipped[leaf]
		if !covered && !left {
			missing = append(missing, leaf)
		}
	}
	if len(missing) > 0 {
		t.Errorf("herdr has %d command(s) that are neither in commands.json nor in excluded.go (add each to one, with a reason if excluded):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}

	// Lo contrario: entradas que apuntan a comandos que herdr ya no tiene.
	var stale []string
	for command := range inCatalog {
		if !real[command] {
			stale = append(stale, "commands.json: "+command)
		}
	}
	for command := range skipped {
		if !real[command] {
			stale = append(stale, "excluded.go: "+command)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("these entries name commands this herdr does not have:\n  %s", strings.Join(stale, "\n  "))
	}
	t.Logf("herdr has %d commands: %d in the palette catalog (%d entries), %d excluded on purpose",
		len(leaves), len(inCatalog), func() int {
			n := 0
			for _, ids := range inCatalog {
				n += len(ids)
			}
			return n
		}(), len(skipped))
}
