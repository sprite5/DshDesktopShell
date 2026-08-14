//go:build !windows

package host

import "os/exec"

// hideWindow is a no-op on platforms where children don't create new
// console windows (macOS/Linux spawn without a console by default).
func hideWindow(cmd *exec.Cmd) {}
