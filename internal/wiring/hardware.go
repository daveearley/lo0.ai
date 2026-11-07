package wiring

import (
	"context"

	"lo0.ai/lo0/internal/app"
	"lo0.ai/lo0/internal/catalog"
	"lo0.ai/lo0/internal/machine"
	"lo0.ai/lo0/internal/match"
	"lo0.ai/lo0/internal/scan"
)

// HardwareOnly builds a service that assesses the named hardware alone, with
// no machine, history or setup to read, for front ends that run where lo0
// cannot see the user's files.
func HardwareOnly(sim match.Simulation) (*app.Service, error) {
	cat, err := catalog.Default()
	if err != nil {
		return nil, err
	}
	pc := machine.Profile{OS: machine.OS{GOOS: "linux", Arch: "amd64"}}
	return app.New(hardwareScanner{sim.Apply(pc)}, cat, Harnesses(), app.Migrator{})
}

type hardwareScanner struct {
	profile machine.Profile
}

func (h hardwareScanner) Run(context.Context, scan.Observer) (scan.Result, error) {
	return scan.Result{Machine: h.profile}, nil
}
