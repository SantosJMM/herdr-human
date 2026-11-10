package main

import "testing"

func TestDetachedChildHasHiddenConsoleNotNone(t *testing.T) {
	// DETACHED_PROCESS (0x8) dejaba al hijo sin consola, y Windows abría una
	// visible por cada programa de consola que lanzaba (herdr).
	a := detachedAttr()
	if a.CreationFlags&0x00000008 != 0 {
		t.Errorf("DETACHED_PROCESS must not be used: %#x", a.CreationFlags)
	}
	if a.CreationFlags&0x08000000 == 0 {
		t.Errorf("CREATE_NO_WINDOW missing: %#x", a.CreationFlags)
	}
	if a.CreationFlags&0x00000200 == 0 {
		t.Errorf("must keep its own process group: %#x", a.CreationFlags)
	}
}
