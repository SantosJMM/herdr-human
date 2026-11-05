package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// block es una tabla [[actions]] o [[panes]] del manifest, leída como texto: el
// manifest solo usa claves simples, así que no hace falta un lector de TOML.
type block struct {
	kind, id, platforms, title, command, text string
}

func readManifest(t *testing.T) []block {
	t.Helper()
	data, err := os.ReadFile("../../herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}
	header := regexp.MustCompile(`(?m)^\[\[(actions|panes)\]\]\s*$`)
	var blocks []block
	for _, part := range header.Split(string(data), -1)[1:] {
		blocks = append(blocks, block{text: part})
	}
	kinds := header.FindAllStringSubmatch(string(data), -1)
	field := func(text, name string) string {
		m := regexp.MustCompile(`(?m)^` + name + ` = (.+)$`).FindStringSubmatch(text)
		if m == nil {
			return ""
		}
		return strings.Trim(m[1], `"`)
	}
	for i := range blocks {
		blocks[i].kind = kinds[i][1]
		blocks[i].id = field(blocks[i].text, "id")
		blocks[i].platforms = field(blocks[i].text, "platforms")
		blocks[i].title = field(blocks[i].text, "title")
		blocks[i].command = field(blocks[i].text, "command")
	}
	return blocks
}

func find(blocks []block, kind, id string) *block {
	for i := range blocks {
		if blocks[i].kind == kind && blocks[i].id == id {
			return &blocks[i]
		}
	}
	return nil
}

// Este es el motivo de existir del manifest actual: powershell.exe tarda cerca de
// un segundo en arrancar y cada acción lo lanzaba dos veces (la acción y su pane)
// antes de dibujar nada. Si alguien vuelve a poner un intérprete delante de
// cualquier acción o pane, el popup vuelve a ser lento.
func TestNothingStartsThroughAnInterpreter(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	if len(blocks) == 0 {
		t.Fatal("no blocks found: the manifest parser is broken")
	}
	for _, b := range blocks {
		if !strings.HasPrefix(b.command, `["bin/hum", `) {
			t.Errorf("%s %q must start bin/hum directly, got command = %s", b.kind, b.id, b.command)
		}
		for _, banned := range []string{"powershell", "pwsh", `"sh"`, `"cmd`, "node", "bash"} {
			if strings.Contains(b.command, banned) {
				t.Errorf("%s %q must not go through %s: %s", b.kind, b.id, banned, b.command)
			}
		}
	}
}

func TestEveryActionOpensAPaneThatExists(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	popup := regexp.MustCompile(`^\["bin/hum", "popup", "([a-z]+)"\]$`)
	for _, b := range blocks {
		if b.kind != "actions" {
			continue
		}
		var pane string
		switch m := popup.FindStringSubmatch(b.command); {
		case m != nil:
			pane = m[1]
		case b.command == `["bin/hum", "palette", "open"]`:
			pane = "palette"
		default:
			t.Errorf("action %q has an unexpected command %s", b.id, b.command)
			continue
		}
		if find(blocks, "panes", pane) == nil {
			t.Errorf("action %q opens pane %q, which does not exist", b.id, pane)
		}
	}
}

func TestEveryPaneRunsSomethingThatExists(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"overview":  `["bin/hum", "popup", "show", "ls"]`,
		"attention": `["bin/hum", "popup", "show", "attention"]`,
		"machines":  `["bin/hum", "machines", "menu"]`,
		"pick":      `["bin/hum", "pick", "--embedded"]`,
		"palette":   `["bin/hum", "palette", "--embedded"]`,
	}
	blocks := readManifest(t)
	for id, command := range want {
		pane := find(blocks, "panes", id)
		if pane == nil {
			t.Errorf("pane %q is missing", id)
			continue
		}
		if pane.command != command {
			t.Errorf("pane %q runs %s, want %s", id, pane.command, command)
		}
		if pane.platforms != "" {
			t.Errorf("pane %q has one definition for every platform, got platforms = %s", id, pane.platforms)
		}
	}
}

// Los atajos de config.toml usan estos ids (p. ej. herdr-human.pick-windows):
// quitar uno rompe un atajo sin avisar.
func TestActionIDsUsedByShortcutsAreKept(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	for _, name := range []string{"overview", "machines", "pick", "attention"} {
		for _, platform := range []struct{ suffix, platforms string }{
			{"windows", `["windows"]`}, {"unix", `["linux", "macos"]`},
		} {
			id := name + "-" + platform.suffix
			action := find(blocks, "actions", id)
			if action == nil {
				t.Errorf("action %s was removed; configured shortcuts depend on it", id)
				continue
			}
			if action.platforms != platform.platforms {
				t.Errorf("action %s platforms = %s, want %s", id, action.platforms, platform.platforms)
			}
		}
	}
	if find(blocks, "actions", "palette") == nil {
		t.Error("action palette is missing")
	}
}

func TestActionTitlesMentionWhatTheyDo(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	for id, want := range map[string]string{
		"pick-windows": "Talk to agents", "pick-unix": "Talk to agents",
		"attention-windows": "Who needs me", "attention-unix": "Who needs me",
		"palette": "command palette",
	} {
		if a := find(blocks, "actions", id); a == nil || !strings.Contains(a.title, want) {
			t.Errorf("action %s title should mention %q: %+v", id, want, a)
		}
	}
}

func TestPopupSizes(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	for id, size := range map[string][2]string{
		"pick": {"90%", "80%"}, "palette": {"70%", "60%"}, "machines": {"90%", "65%"},
		"overview": {"80%", "70%"}, "attention": {"80%", "60%"},
	} {
		pane := find(blocks, "panes", id)
		if pane == nil {
			t.Errorf("pane %s is missing", id)
			continue
		}
		for _, want := range []string{`placement = "popup"`, `width = "` + size[0] + `"`, `height = "` + size[1] + `"`} {
			if !strings.Contains(pane.text, want) {
				t.Errorf("pane %s lacks %s", id, want)
			}
		}
	}
}

// Herdr ya dibuja el marco y el titulo de un popup. Las TUI de hum que dibujan su
// propio marco (el selector y la paleta) deben pedir el modo incrustado, o salen
// como una caja dentro de otra; las vistas de texto no tienen marco que quitar.
func TestPopupsThatDrawAFrameAskToBeEmbedded(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	for _, id := range []string{"pick", "palette"} {
		pane := find(blocks, "panes", id)
		if pane == nil || !strings.Contains(pane.command, `"--embedded"`) {
			t.Errorf("pane %s draws its own frame inside the one herdr draws: it needs --embedded: %+v", id, pane)
		}
	}
	for _, id := range []string{"overview", "attention", "machines"} {
		if pane := find(blocks, "panes", id); pane != nil && strings.Contains(pane.command, "--embedded") {
			t.Errorf("pane %s is plain text and has no frame to remove", id)
		}
	}
}
