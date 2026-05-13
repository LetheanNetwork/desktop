// SPDX-Licence-Identifier: EUPL-1.2

//go:build !darwin && !linux

package models

// diskFreeBytes is a no-op fallback on platforms we don't ship to
// yet — Windows / WASM / etc. The WebView's footer slot stays on
// its design literal when zero is returned.
func diskFreeBytes(_ string) int64 { return 0 }
