// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin

package paths

func defaultPipeBufLimit() int { return pipeBufDarwin }
