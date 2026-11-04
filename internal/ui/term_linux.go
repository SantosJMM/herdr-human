//go:build linux

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

// tiocgwinsz es la petición ioctl TIOCGWINSZ de Linux.
const tiocgwinsz = 0x5413

type winsize struct{ Row, Col, Xpixel, Ypixel uint16 }

func terminalWidth(f *os.File) int {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), tiocgwinsz, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0
	}
	return int(ws.Col)
}

func enableVT(*os.File) {}

func terminalHeight(f *os.File) int {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), tiocgwinsz, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0
	}
	return int(ws.Row)
}
