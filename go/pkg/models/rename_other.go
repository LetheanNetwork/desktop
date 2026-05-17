// SPDX-Licence-Identifier: EUPL-1.2

//go:build !darwin && !linux

package models

import core "dappco.re/go"

// atomicRename is the non-POSIX fallback — falls back to core.Rename,
// which is the same Stat+Rename pair with the residual TOCTOU window.
// Lethean Desktop ships to macOS today; this branch exists so the
// package builds on Windows / WASM / etc. until those platforms get a
// proper atomic-rename primitive.
//
// Usage example:
//
//	if r := atomicRename(src, dst); !r.OK { return r }
func atomicRename(src, dst string) core.Result {
	return core.Rename(src, dst)
}
