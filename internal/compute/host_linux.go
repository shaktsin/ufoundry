//go:build linux

package compute

import (
	"errors"
	"fmt"
	"os"
)

var kvmDevice = "/dev/kvm"

// CheckHostAvailability verifies that Linux KVM exists and is accessible to
// this app before a project enables isolated compute. KVM can still disappear
// after configuration, so callers also check immediately before launching.
func CheckHostAvailability() error {
	info, err := os.Stat(kvmDevice)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("isolated compute is unavailable: this Linux host has no /dev/kvm device")
		}
		return fmt.Errorf("isolated compute is unavailable: cannot inspect /dev/kvm: %w", err)
	}
	if info.Mode()&(os.ModeDevice|os.ModeCharDevice) != os.ModeDevice|os.ModeCharDevice {
		return errors.New("isolated compute is unavailable: /dev/kvm is not a character device")
	}
	device, err := os.OpenFile(kvmDevice, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("isolated compute is unavailable: /dev/kvm cannot be opened; enable KVM and grant this user access: %w", err)
	}
	return device.Close()
}
