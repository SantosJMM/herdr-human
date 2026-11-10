//go:build !windows

package herdr

import "os/exec"

// HideConsole no hace nada fuera de Windows: no hay consolas que se abran.
func HideConsole(*exec.Cmd) {}
