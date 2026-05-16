// SPDX-Licence-Identifier: EUPL-1.2

//go:build linux

package paths

func defaultPipeBufLimit() int { return pipeBufLinux }
