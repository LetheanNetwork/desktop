// SPDX-Licence-Identifier: EUPL-1.2

package desktop_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop"
)

func ExampleVersion() {
	ref := subject.Version
	_ = core.Sprintf("%s", ref)
}

func TestVersion_Good_Default(t *core.T) {
	core.AssertNotEmpty(t, subject.Version)
}

func TestVersion_Bad_AssignableForLinkerStamp(t *core.T) {
	original := subject.Version
	t.Cleanup(func() { subject.Version = original })

	subject.Version = "v9.8.7-test"

	core.AssertEqual(t, "v9.8.7-test", subject.Version)
}

func TestVersion_Ugly_DirtyGitDescribeValue(t *core.T) {
	original := subject.Version
	t.Cleanup(func() { subject.Version = original })

	subject.Version = "v9.8.7-4-gabc123-dirty"

	core.AssertContains(t, subject.Version, "dirty")
}
