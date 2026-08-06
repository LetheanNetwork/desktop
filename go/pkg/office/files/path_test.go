// SPDX-License-Identifier: EUPL-1.2

package files

import (
	core "dappco.re/go"
)

func TestHasUnsafePathRune_Bad(t *core.T) {
	for name, value := range map[string]string{
		"control character":  "a\x01b",
		"delete range":       "a\x7fb",
		"left-to-right mark": "a‎b",
		"right-to-left mark": "a‏b",
		"bidi override":      "a‮b",
		"bidi isolate":       "a⁦b",
	} {
		t.Run(name, func(t *core.T) {
			core.AssertTrue(t, hasUnsafePathRune(value))
		})
	}
}

func TestHasUnsafePathRune_GoodPlainTextIsSafe(t *core.T) {
	core.AssertFalse(t, hasUnsafePathRune("résumé 2026.md"))
}

func TestValidateMountID_Good(t *core.T) {
	core.AssertNoError(t, validateMountID("documents-2026"))
}

func TestValidateMountID_Bad(t *core.T) {
	for name, id := range map[string]string{
		"empty":         "",
		"too long":      core.Repeat("a", maxMountIDBytes+1),
		"uppercase":     "Documents",
		"unsafe symbol": "documents!",
	} {
		t.Run(name, func(t *core.T) {
			err := validateMountID(id)

			core.AssertError(t, err)
			core.AssertContains(t, err.Error(), string(ErrorInvalidMount))
		})
	}
}
