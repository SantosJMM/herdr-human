package main

import "syscall"

// Banderas de creación de procesos de Windows.
const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// detachedAttr separa al hijo del grupo de procesos del padre y evita que abra
// una ventana. Se usa CREATE_NO_WINDOW y no DETACHED_PROCESS: con este último el
// hijo no tiene consola y Windows crea una visible para cada programa de consola
// que lance (herdr), que se veía como una ventana que parpadea.
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNoWindow | createNewProcessGroup,
		HideWindow:    true,
	}
}
