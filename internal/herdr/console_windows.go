package herdr

import (
	"os/exec"
	"syscall"
)

// createNoWindow es la bandera CREATE_NO_WINDOW de Windows.
const createNoWindow = 0x08000000

// HideConsole evita que el proceso hijo abra una ventana de consola. Si quien
// lo lanza no tiene consola (por ejemplo un proceso desacoplado), Windows le
// crearía una visible a cada programa de consola que arranque.
func HideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
	cmd.SysProcAttr.HideWindow = true
}
