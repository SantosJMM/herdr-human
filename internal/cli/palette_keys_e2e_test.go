package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SantosJMM/herdr-human/internal/cli"
	"github.com/SantosJMM/herdr-human/internal/herdrtest"
)

func okCheck(context.Context, string) (string, error) { return "config: ok\n", nil }

func TestPaletteEndToEndChangesAShortcutInPlaceAndShowsIt(t *testing.T) {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	fake.Reply("", []string{"server", "reload-config"}, okReply)
	r := startPaletteWith(t, fake, nil, func(env *cli.Env) { env.CheckConfig = okCheck })
	config := filepath.Join(r.state, "config.toml") // ya tiene f2 -> x.plug.ping

	_, _ = r.in.Write([]byte("ping"))
	r.scr.waitFor(t, "Ping the thing")
	r.scr.waitFor(t, "ctrl+k shortcut") // la ayuda anuncia la tecla cuando hay servicio
	_, _ = r.in.Write([]byte("\x0b"))   // ctrl+k
	r.scr.waitFor(t, "now: f2")
	_, _ = r.in.Write([]byte("\x1b\x0b")) // alt+ctrl+k: la combinación a asignar
	r.scr.waitFor(t, "new: ctrl+alt+k")
	_, _ = r.in.Write([]byte("\r"))
	r.scr.waitFor(t, "Shortcut ctrl+alt+k saved for Ping the thing.")
	_, _ = r.in.Write([]byte("\x1b")) // esc: cierra la paleta

	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	want := "[[keys.command]]\nkey = \"ctrl+alt+k\"\ntype = \"plugin_action\"\ncommand = \"x.plug.ping\"\n"
	if string(data) != want {
		t.Errorf("only the key of the existing block changed:\n%s", data)
	}
	reloads := 0
	for _, c := range fake.Calls() {
		if len(c.Args) >= 2 && c.Args[0] == "server" && c.Args[1] == "reload-config" {
			reloads++
		}
	}
	if reloads != 1 {
		t.Errorf("herdr is asked to reload once, got %d", reloads)
	}
	if _, err := os.Stat(config + ".bak-hum-" + todayStamp()); err != nil {
		t.Errorf("the config was backed up first: %v", err)
	}
	select {
	case args := <-r.detach:
		t.Errorf("assigning a shortcut runs nothing: %v", args)
	default:
	}
}

func TestPaletteEndToEndRefusesAKeyAnotherActionUsesAndWritesNothing(t *testing.T) {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	r := startPaletteWith(t, fake, nil, func(env *cli.Env) { env.CheckConfig = okCheck })
	config := filepath.Join(r.state, "config.toml")
	before, _ := os.ReadFile(config)

	_, _ = r.in.Write([]byte("pick"))
	r.scr.waitFor(t, "Talk to agents")
	_, _ = r.in.Write([]byte("\x0b"))
	r.scr.waitFor(t, "now: none")
	_, _ = r.in.Write([]byte("\x1bOQ")) // F2, que el archivo ya usa para x.plug.ping
	r.scr.waitFor(t, "already uses f2")
	_, _ = r.in.Write([]byte("\r"))    // Enter no guarda una tecla con problema
	_, _ = r.in.Write([]byte("\x1b"))  // cierra la pantalla de atajo
	time.Sleep(150 * time.Millisecond) // dos Esc pegados se leerían como alt+esc
	_, _ = r.in.Write([]byte("\x1b"))  // y esta cierra la paleta

	if code := r.wait(t); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, r.stderr.String())
	}
	after, _ := os.ReadFile(config)
	if string(after) != string(before) {
		t.Errorf("a refused key leaves the file alone:\n%s", after)
	}
	if strings.Contains(r.scr.buf.String(), "saved for") {
		t.Error("nothing was saved")
	}
}

func TestPaletteHasNoShortcutScreenWithoutAWayToValidateChanges(t *testing.T) {
	fake := herdrtest.New()
	fake.Reply("", []string{"plugin", "list", "--json"}, pluginList)
	r := startPaletteUI(t, fake, nil) // sin CheckConfig
	r.scr.waitFor(t, "enter run")
	if strings.Contains(r.scr.buf.String(), "ctrl+k") {
		t.Error("the feature is not offered when changes cannot be checked with herdr")
	}
	_, _ = r.in.Write([]byte("\x1b"))
	r.wait(t)
}
