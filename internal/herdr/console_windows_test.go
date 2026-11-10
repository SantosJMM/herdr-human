package herdr_test

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/SantosJMM/herdr-human/internal/herdr"
)

func TestHideConsoleAddsNoWindowAndKeepsOtherFlags(t *testing.T) {
	const newProcessGroup = 0x00000200
	cmd := exec.Command("herdr")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: newProcessGroup}
	herdr.HideConsole(cmd)

	got := cmd.SysProcAttr
	if got.CreationFlags&0x08000000 == 0 {
		t.Errorf("CREATE_NO_WINDOW missing: %#x", got.CreationFlags)
	}
	if got.CreationFlags&newProcessGroup == 0 {
		t.Errorf("existing flags must stay: %#x", got.CreationFlags)
	}
	if !got.HideWindow {
		t.Error("HideWindow must be set")
	}
}

func TestHideConsoleWorksWithoutPreviousAttributes(t *testing.T) {
	cmd := exec.Command("herdr")
	herdr.HideConsole(cmd)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
		t.Errorf("%+v", cmd.SysProcAttr)
	}
}
