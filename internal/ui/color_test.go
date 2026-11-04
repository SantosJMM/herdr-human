package ui_test

import (
	"testing"

	"github.com/SantosJMM/herdr-human/internal/ui"
)

func TestParseColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want ui.Color
		ok   bool
	}{
		{"#a6e3a1", ui.Color{R: 0xa6, G: 0xe3, B: 0xa1}, true},
		{"#A6E3A1", ui.Color{R: 0xa6, G: 0xe3, B: 0xa1}, true},
		{"rgb(1, 2,3)", ui.Color{R: 1, G: 2, B: 3}, true},
		{"red", ui.Color{R: 205, G: 49, B: 49}, true},
		{" Blue ", ui.Color{R: 36, G: 114, B: 200}, true},
		{"reset", ui.Color{}, false},
		{"default", ui.Color{}, false},
		{"none", ui.Color{}, false},
		{"transparent", ui.Color{}, false},
		{"#abc", ui.Color{}, false},
		{"rgb(300,0,0)", ui.Color{}, false},
		{"rgb(1,2)", ui.Color{}, false},
		{"notacolor", ui.Color{}, false},
		{"", ui.Color{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, ok := ui.ParseColor(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Errorf("ParseColor(%q) = (%+v, %v), want (%+v, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
