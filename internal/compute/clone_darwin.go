//go:build darwin

package compute

import (
	"golang.org/x/sys/unix"
	"os"
)

// cloneRegularFile uses APFS copy-on-write cloning so a toolchain-sized guest
// root can be prepared without copying hundreds of megabytes before each VM.
func cloneRegularFile(source, target string, mode os.FileMode) (bool, error) {
	if err := unix.Clonefile(source, target, 0); err != nil {
		return false, nil // Fall back to a portable byte copy.
	}
	if err := os.Chmod(target, mode); err != nil {
		return false, err
	}
	return true, nil
}
