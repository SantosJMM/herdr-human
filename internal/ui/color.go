// Paquete ui es el kit de render compartido: tema, glifos, ancho y recorte de
// texto. No tiene dependencias externas; los colores salen como secuencias ANSI.
package ui

import (
	"strconv"
	"strings"
)

// Color es un color de 24 bits.
type Color struct{ R, G, B uint8 }

// namedColors son los colores con nombre que herdr acepta en [theme.custom];
// los valores son los del Windows Terminal/VS Code clásico y solo son una
// aproximación.
var namedColors = map[string]Color{
	"black":   {0, 0, 0},
	"red":     {205, 49, 49},
	"green":   {13, 188, 121},
	"yellow":  {229, 229, 16},
	"blue":    {36, 114, 200},
	"magenta": {188, 63, 188},
	"cyan":    {17, 168, 205},
	"white":   {229, 229, 229},
	"gray":    {128, 128, 128},
	"grey":    {128, 128, 128},
}

// ParseColor interpreta un valor de color de la configuración de herdr: hex
// (#rrggbb), rgb(r,g,b) o un nombre. Los alias de "sin color" (reset, default,
// none, transparent) y todo lo inválido devuelven ok = false.
func ParseColor(s string) (c Color, ok bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "#") && len(s) == 7:
		v, err := strconv.ParseUint(s[1:], 16, 32)
		if err != nil {
			return Color{}, false
		}
		return Color{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
	case strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")"):
		parts := strings.Split(s[4:len(s)-1], ",")
		if len(parts) != 3 {
			return Color{}, false
		}
		var rgb [3]uint8
		for i, part := range parts {
			n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 8)
			if err != nil {
				return Color{}, false
			}
			rgb[i] = uint8(n)
		}
		return Color{rgb[0], rgb[1], rgb[2]}, true
	}
	c, ok = namedColors[s]
	return c, ok
}
