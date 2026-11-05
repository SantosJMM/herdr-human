//go:build !windows

package main

import "syscall"

// detachedAttr pone al hijo en una sesión nueva, para que no muera con la
// terminal ni con el grupo de procesos del padre.
func detachedAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
