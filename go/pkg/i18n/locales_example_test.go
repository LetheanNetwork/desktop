// SPDX-Licence-Identifier: EUPL-1.2

package i18n_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/i18n"
)

func ExampleSource() {
	ref := subject.Source
	_ = core.Sprintf("%T", ref)
}

func TestLocales_Source_Good(t *core.T) {
	ref := subject.Source
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Source")
}

func TestLocales_Source_Bad(t *core.T) {
	ref := subject.Source
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Source", "Source")
}

func TestLocales_Source_Ugly(t *core.T) {
	ref := subject.Source
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Source"), 0)
}
