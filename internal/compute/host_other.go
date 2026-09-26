//go:build !linux

package compute

// Linux KVM preflight is not applicable on macOS, where libkrun uses Apple's
// Hypervisor framework. The bundled bridge performs its runtime checks there.
func CheckHostAvailability() error { return nil }
