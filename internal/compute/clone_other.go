//go:build !darwin

package compute

import "os"

func cloneRegularFile(_, _ string, _ os.FileMode) (bool, error) { return false, nil }
