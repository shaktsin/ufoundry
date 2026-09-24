// Package procutil makes subprocesses stop cleanly: a cancelled or timed-out
// command kills its whole process group, and output pipes held open by
// grandchildren do not block Wait.
package procutil

import (
	"os/exec"
	"time"
)

// Prepare configures cmd (from exec.CommandContext) so cancellation kills the
// process group and Wait returns within a few seconds.
func Prepare(cmd *exec.Cmd) {
	setGroup(cmd)
	cmd.Cancel = func() error { return killGroup(cmd) }
	cmd.WaitDelay = 3 * time.Second
}
