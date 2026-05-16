// SPDX-Licence-Identifier: EUPL-1.2

//go:build !darwin && !linux

package paths

func defaultPipeBufLimit() int { return pipeBufOther }
