//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: don't allocate a console for the
// child at all.
const createNoWindow = 0x08000000

// HideWindow prevents cmd from popping up a visible console window when
// run from vidpolish-gui. vidpolish-gui is a Windows GUI-subsystem
// binary with no console of its own; every console-subsystem child it
// spawns (ffmpeg, ffprobe, deep-filter, auto-editor, resvg - all of
// pipeline/thumbnail's exec.Command calls) would otherwise get a brand
// new console window allocated for it, which Windows shows on screen and
// then closes the instant that one command exits. A multi-stage edit run
// invokes several of these in sequence, so this looks exactly like a
// window rapidly opening and closing over and over - which is what it
// is, cosmetically, but it also steals focus repeatedly and makes a
// long-running stage look "stuck" behind its own flickering console.
// The CLI (a console-subsystem binary itself) already has a console
// attached, so this is a no-op there in practice, but it's fine to call
// unconditionally since it only affects newly-created child consoles.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
