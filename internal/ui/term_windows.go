//go:build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	procGetBuffer = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procGetMode   = kernel32.NewProc("GetConsoleMode")
	procSetMode   = kernel32.NewProc("SetConsoleMode")
)

// vtProcessing es ENABLE_VIRTUAL_TERMINAL_PROCESSING.
const vtProcessing = 0x0004

type coord struct{ X, Y int16 }

type smallRect struct{ Left, Top, Right, Bottom int16 }

// consoleInfo es CONSOLE_SCREEN_BUFFER_INFO.
type consoleInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

func terminalWidth(f *os.File) int {
	var info consoleInfo
	if r, _, _ := procGetBuffer.Call(f.Fd(), uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	return int(info.Window.Right-info.Window.Left) + 1
}

func enableVT(f *os.File) {
	var mode uint32
	if r, _, _ := procGetMode.Call(f.Fd(), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return
	}
	procSetMode.Call(f.Fd(), uintptr(mode|vtProcessing))
}

func terminalHeight(f *os.File) int {
	var info consoleInfo
	if r, _, _ := procGetBuffer.Call(f.Fd(), uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	return int(info.Window.Bottom-info.Window.Top) + 1
}
