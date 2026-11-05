package main

import "syscall"

// Banderas de creación de procesos de Windows.
const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// detachedAttr separa al hijo de la consola y del grupo de procesos del padre,
// y evita que abra una ventana.
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
		HideWindow:    true,
	}
}
