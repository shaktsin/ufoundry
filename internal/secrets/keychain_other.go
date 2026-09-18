//go:build !darwin

package secrets

func platformStore() Store { return nil }

// ReadLegacy is only supported on macOS.
func ReadLegacy(service, account string) (string, bool) { return "", false }
