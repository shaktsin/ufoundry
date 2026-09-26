//go:build !libkrun || (!darwin && !linux)

package compute

import "errors"

func RunGuest(GuestRequest) error {
	return errors.New("this UMCode compute bridge was built without its bundled libkrun runtime")
}
