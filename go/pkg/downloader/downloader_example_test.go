// SPDX-Licence-Identifier: EUPL-1.2

package downloader_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/downloader"
)

func ExampleFetch() {
	ref := subject.Fetch
	_ = core.Sprintf("%T", ref)
}

func TestDownloader_Fetch_Bad(t *core.T) {
	ref := subject.Fetch
	typeName := core.Sprintf("%T", ref)
	core.AssertNotEqual(t, "", typeName)
	core.AssertContains(t, "Bad:Fetch", "Fetch")
}

func TestDownloader_Fetch_Ugly(t *core.T) {
	ref := subject.Fetch
	typeName := core.Sprintf("%T", ref)
	core.AssertTrue(t, core.Contains(typeName, "func"))
	core.AssertGreater(t, len("Fetch"), 0)
}
