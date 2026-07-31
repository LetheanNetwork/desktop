// SPDX-Licence-Identifier: EUPL-1.2

//go:build !darwin || !cgo

package telemetry

import core "dappco.re/go"

type portableHostSource struct{}

func newPlatformHostSource() hostSource {
	return portableHostSource{}
}

func (portableHostSource) read() (hostReading, error) {
	return hostReading{
		observedAt:   core.Now(),
		source:       "Portable Go runtime",
		platform:     core.OS(),
		architecture: core.Arch(),
		logicalCores: core.NumCPU(),
	}, nil
}
