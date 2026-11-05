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
	kind, id, platforms, title, text string
}

func readManifest(t *testing.T) []block {
	t.Helper()
	data, err := os.ReadFile("../../herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}
	var blocks []block
	for _, part := range regexp.MustCompile(`(?m)^\[\[(actions|panes)\]\]\s*$`).Split(string(data), -1)[1:] {
		blocks = append(blocks, block{text: part})
	}
	kinds := regexp.MustCompile(`(?m)^\[\[(actions|panes)\]\]\s*$`).FindAllStringSubmatch(string(data), -1)
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

func TestEveryActionOpensAPaneThatExistsForItsPlatform(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	entry := regexp.MustCompile(`--entrypoint \$?"?([a-z-]+)`)
	count := 0
	for _, b := range blocks {
		if b.kind != "actions" {
			continue
		}
		count++
		if strings.Contains(b.text, `"bin/hum"`) {
			// Acción directa (la paleta): no abre el pane con --entrypoint, lo hace
			// el propio hum; su pane se comprueba en TestPaletteStartsWithoutAnInterpreter.
			continue
		}
		m := entry.FindStringSubmatch(b.text)
		if m == nil {
			t.Errorf("action %q has no --entrypoint", b.id)
			continue
		}
		pane := find(blocks, "panes", m[1])
		if pane == nil {
			t.Errorf("action %q opens pane %q, which does not exist", b.id, m[1])
			continue
		}
		if pane.platforms != b.platforms {
			t.Errorf("action %q is for %s but its pane %q is for %s", b.id, b.platforms, pane.id, pane.platforms)
		}
	}
	if count == 0 {
		t.Fatal("no actions found: the manifest parser is broken")
	}
}

func TestSelectorAndAttentionAreAvailableOnEveryPlatform(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	for _, tt := range []struct {
		name, command, title string
	}{
		{"pick", " pick", "Talk to agents"},
		{"attention", " attention", "Who needs me"},
	} {
		for _, platform := range []string{"windows", "unix"} {
			id := tt.name + "-" + platform
			action, pane := find(blocks, "actions", id), find(blocks, "panes", id)
			if action == nil || pane == nil {
				t.Errorf("%s: missing action or pane", id)
				continue
			}
			if !strings.Contains(action.title, tt.title) {
				t.Errorf("%s: action title = %q, want it to mention %q", id, action.title, tt.title)
			}
			if !strings.Contains(pane.text, "hum") || !strings.Contains(pane.text, tt.command) {
				t.Errorf("%s: the pane must run hum%s:\n%s", id, tt.command, pane.text)
			}
		}
	}
}

func TestSelectorPopupIsBigEnoughToBeUseful(t *testing.T) {
	t.Parallel()

	for _, b := range readManifest(t) {
		if b.kind == "panes" && strings.HasPrefix(b.id, "pick-") {
			for _, want := range []string{`width = "90%"`, `height = "80%"`, `placement = "popup"`} {
				if !strings.Contains(b.text, want) {
					t.Errorf("pane %s lacks %s", b.id, want)
				}
			}
		}
	}
}

func TestExistingEntriesAreKept(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	for _, id := range []string{"overview-windows", "overview-unix", "machines-windows", "machines-unix"} {
		if find(blocks, "actions", id) == nil || find(blocks, "panes", id) == nil {
			t.Errorf("%s was removed from the manifest; configured shortcuts depend on it", id)
		}
	}
}

// El motivo de existir de este test: powershell.exe tarda cerca de un segundo en
// arrancar y la paleta lo lanzaba dos veces (acción y pane) antes de dibujar.
// Si alguien vuelve a poner un intérprete delante, la paleta vuelve a ser lenta.
func TestPaletteStartsWithoutAnInterpreter(t *testing.T) {
	t.Parallel()

	blocks := readManifest(t)
	for _, tt := range []struct {
		kind, want string
	}{
		{"actions", `command = ["bin/hum", "palette", "open"]`},
		{"panes", `command = ["bin/hum", "palette"]`},
	} {
		b := find(blocks, tt.kind, "palette")
		if b == nil {
			t.Fatalf("%s %q is missing", tt.kind, "palette")
		}
		if !strings.Contains(b.text, tt.want) {
			t.Errorf("%s palette must start hum directly (%s):\n%s", tt.kind, tt.want, b.text)
		}
		for _, banned := range []string{"powershell", "pwsh", `"sh"`, `"cmd`, "node"} {
			if strings.Contains(b.text, banned) {
				t.Errorf("%s palette must not go through %s:\n%s", tt.kind, banned, b.text)
			}
		}
		if b.platforms != "" {
			t.Errorf("one definition serves every platform, got platforms = %s", b.platforms)
		}
	}
}

func TestPalettePopupSizeAndPlacement(t *testing.T) {
	t.Parallel()

	pane := find(readManifest(t), "panes", "palette")
	if pane == nil {
		t.Fatal("pane palette is missing")
	}
	for _, want := range []string{`placement = "popup"`, `width = "70%"`, `height = "60%"`} {
		if !strings.Contains(pane.text, want) {
			t.Errorf("pane palette lacks %s", want)
		}
	}
}
