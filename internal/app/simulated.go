package app

import "errors"

// ErrSimulated means the assessment is of named hardware, so nothing can be
// set up, tried or measured.
var ErrSimulated = errors.New("this assessment is of simulated hardware, so lo0 changes nothing for it; run lo0 without --gpu or --mac to set up this machine")

// onThisMachine refuses an assessment of simulated hardware.
func onThisMachine(a Assessment) error {
	if a.Scan.Machine.Simulated {
		return ErrSimulated
	}
	return nil
}
