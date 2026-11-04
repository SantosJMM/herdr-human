package main

import (
	"errors"
	"reflect"
	"testing"
)

func lookPathOf(found ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, f := range found {
			if f == name {
				return "/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestClipboardCommandPerPlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		goos     string
		found    []string
		wantName string
		wantArgs []string
	}{
		{"windows", "windows", []string{"clip"}, "clip", nil},
		{"macos", "darwin", []string{"pbcopy"}, "pbcopy", nil},
		{"wayland", "linux", []string{"wl-copy", "xclip"}, "wl-copy", nil},
		{"x11 with xclip", "linux", []string{"xclip", "xsel"}, "xclip", []string{"-selection", "clipboard"}},
		{"x11 with xsel", "linux", []string{"xsel"}, "xsel", []string{"--clipboard", "--input"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			name, args, err := clipboardCommand(tt.goos, lookPathOf(tt.found...))
			if err != nil || name != tt.wantName || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("got %q %v (%v), want %q %v", name, args, err, tt.wantName, tt.wantArgs)
			}
		})
	}
}

func TestClipboardCommandWithoutAnyToolExplainsWhat(t *testing.T) {
	t.Parallel()

	if _, _, err := clipboardCommand("linux", lookPathOf()); err == nil {
		t.Errorf("want an error when no clipboard tool exists")
	}
	if _, _, err := clipboardCommand("plan9", lookPathOf("clip")); err == nil {
		t.Errorf("want an error on an unsupported platform")
	}
}
