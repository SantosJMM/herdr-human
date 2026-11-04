package ui

import "os"

// Width devuelve el ancho de la terminal de f; si f no es una terminal o no se
// puede consultar, usa COLUMNS y, en último caso, 80.
func Width(f *os.File, getenv func(string) string) int {
	return WidthFrom(terminalWidth(f), getenv)
}

// EnableVT activa en Windows el procesamiento de secuencias ANSI de la consola
// de f. En otras plataformas no hace nada. Un fallo se ignora: sin VT las
// secuencias saldrían como texto, pero la consola moderna ya lo trae activo.
func EnableVT(f *os.File) { enableVT(f) }

// Height devuelve las filas de la terminal de f, o 0 si no es una terminal o no
// se puede consultar (quien llama debe tratar 0 como "se desconoce").
func Height(f *os.File) int { return terminalHeight(f) }
