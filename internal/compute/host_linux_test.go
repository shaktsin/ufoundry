//go:build linux

package compute

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckHostAvailabilityExplainsMissingKVM(t *testing.T) {
	old := kvmDevice
	kvmDevice = filepath.Join(t.TempDir(), "kvm")
	t.Cleanup(func() { kvmDevice = old })
	if err := CheckHostAvailability(); err == nil || err.Error() != "isolated compute is unavailable: this Linux host has no /dev/kvm device" {
		t.Fatalf("missing KVM error = %v", err)
	}
}

func TestCheckHostAvailabilityRejectsNonDeviceKVM(t *testing.T) {
	old := kvmDevice
	kvmDevice = filepath.Join(t.TempDir(), "kvm")
	t.Cleanup(func() { kvmDevice = old })
	if err := os.WriteFile(kvmDevice, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckHostAvailability(); err == nil || err.Error() != "isolated compute is unavailable: /dev/kvm is not a character device" {
		t.Fatalf("non-device KVM error = %v", err)
	}
}
