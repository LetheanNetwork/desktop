// SPDX-Licence-Identifier: EUPL-1.2

package services_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/services"
)

func ExampleRegistry() {
	ref := subject.Registry
	_ = core.Sprintf("%T", ref)
}

func ExampleLookup() {
	ref := subject.Lookup
	_ = core.Sprintf("%T", ref)
}

func ExampleNames() {
	ref := subject.Names
	_ = core.Sprintf("%T", ref)
}

func TestRegistry_Registry_Good(t *core.T) {
	ref := subject.Registry
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Registry")
}

func TestRegistry_Registry_Bad(t *core.T) {
	ref := subject.Registry
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Registry", "Registry")
}

func TestRegistry_Registry_Ugly(t *core.T) {
	ref := subject.Registry
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Registry"), 0)
}

func TestRegistry_Lookup_Good(t *core.T) {
	ref := subject.Lookup
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Lookup")
}

func TestRegistry_Lookup_Bad(t *core.T) {
	ref := subject.Lookup
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Lookup", "Lookup")
}

func TestRegistry_Lookup_Ugly(t *core.T) {
	ref := subject.Lookup
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Lookup"), 0)
}

func TestRegistry_Names_Good(t *core.T) {
	ref := subject.Names
	typeName := core.Sprintf("%T", ref)
	core.AssertContains(t, typeName, "func")
	core.AssertNotEmpty(t, "Names")
}

func TestRegistry_Names_Bad(t *core.T) {
	ref := subject.Names
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Names", "Names")
}

func TestRegistry_Names_Ugly(t *core.T) {
	ref := subject.Names
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Names"), 0)
}
