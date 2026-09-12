// Package browseropen opens a URL in the user's default browser, best
// effort, across platforms.
package browseropen

import (
	"os/exec"
	"runtime"
)

// Open launches the user's default browser at target. It returns an
// error if the platform command couldn't even be started (e.g. no
// display/browser available); callers should treat that as non-fatal and
// fall back to printing the URL for the user to open manually.
func Open(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
