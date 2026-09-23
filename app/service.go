package main

import "errors"

type serviceState int

const (
	serviceUnsupported serviceState = iota // not a bundled macOS 13+ app
	serviceNotRegistered
	serviceEnabled
	serviceRequiresApproval
)

var errServiceUnsupported = errors.New("background service is only available in the installed macOS app")
