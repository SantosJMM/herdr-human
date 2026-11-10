package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

// copyToClipboard copia text al portapapeles con la herramienta de la plataforma.
// El texto va por la entrada estándar, nunca como argumento.
func copyToClipboard(text string) error {
	name, args, err := clipboardCommand(runtime.GOOS, exec.LookPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	herdr.HideConsole(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// clipboardCommand elige el programa que escribe en el portapapeles; lookPath
// se inyecta para poder probarlo.
func clipboardCommand(goos string, lookPath func(string) (string, error)) (string, []string, error) {
	have := func(name string) bool {
		_, err := lookPath(name)
		return err == nil
	}
	switch goos {
	case "windows":
		if have("clip") {
			return "clip", nil, nil
		}
	case "darwin":
		if have("pbcopy") {
			return "pbcopy", nil, nil
		}
	case "linux":
		switch {
		case have("wl-copy"):
			return "wl-copy", nil, nil
		case have("xclip"):
			return "xclip", []string{"-selection", "clipboard"}, nil
		case have("xsel"):
			return "xsel", []string{"--clipboard", "--input"}, nil
		}
	default:
		return "", nil, fmt.Errorf("no clipboard support on %s", goos)
	}
	return "", nil, errors.New("no clipboard tool found (clip, pbcopy, wl-copy, xclip or xsel)")
}
