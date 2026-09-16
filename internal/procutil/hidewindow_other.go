//go:build !windows

package procutil

import "os/exec"

// HideWindow is a no-op on platforms with no such thing as a console
// window popping up for a child process; see hidewindow_windows.go.
func HideWindow(cmd *exec.Cmd) {}
