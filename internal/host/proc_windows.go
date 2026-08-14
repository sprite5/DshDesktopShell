//go:build windows

package host

import (
	"os/exec"
	"syscall"
)

// hideWindow sets CREATE_NO_WINDOW so a console-subsystem child (cmd.exe /
// node.exe / taskkill.exe) spawned from a GUI-subsystem app does not pop up
// a new console window. Without it, starting the managed dsh through
// `cmd /c npx.cmd ...` flashes a cmd box on every start/restart.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
