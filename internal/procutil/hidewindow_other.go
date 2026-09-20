//go:build !windows

package procutil

import "os/exec"

// HideWindow is a no-op on platforms with no such thing as a console
// window popping up for a child process; see hidewindow_windows.go.
func HideWindow(cmd *exec.Cmd) {}

// LowerPriority runs cmd at reduced OS scheduling priority via the `nice`
// utility (present on Linux and macOS), so a maxed-out ffmpeg/deep-filter/
// auto-editor pass competes for CPU behind the desktop UI/OS instead of
// starving it - large edits pinning every core is still expected, but the
// machine should stay interactive while it happens. If `nice` isn't found
// on PATH, this is a no-op: lowering priority is a responsiveness nicety,
// not something a run should fail over. Must be called before cmd.Start().
func LowerPriority(cmd *exec.Cmd) {
	nicePath, err := exec.LookPath("nice")
	if err != nil {
		return
	}
	cmd.Args = append([]string{nicePath, "-n", "10"}, cmd.Args...)
	cmd.Path = nicePath
}
