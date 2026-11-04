//go:build !windows && !linux

package ui

import "os"

// En otras plataformas no se consulta la terminal: se usa COLUMNS o 80.
func terminalWidth(*os.File) int { return 0 }

func enableVT(*os.File) {}

func terminalHeight(*os.File) int { return 0 }
