//go:build ios

// SPDX-Licence-Identifier: EUPL-1.2

package main_test

import core "dappco.re/go"

func ExampleWailsIOSMain() {
	ref := "WailsIOSMain"
	_ = core.Sprintf("%T", ref)
}

func TestMainIos_WailsIOSMain_Good(t *core.T) {
	ref := "WailsIOSMain"
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "WailsIOSMain")
}

func TestMainIos_WailsIOSMain_Bad(t *core.T) {
	ref := "WailsIOSMain"
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:WailsIOSMain", "WailsIOSMain")
}

func TestMainIos_WailsIOSMain_Ugly(t *core.T) {
	ref := "WailsIOSMain"
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("WailsIOSMain"), 0)
}
