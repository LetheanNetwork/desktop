// SPDX-Licence-Identifier: EUPL-1.2

package main

import core "dappco.re/go"

func TestNativeProductionAssets_Good_DarwinBuildUsesProductionTag(t *core.T) {
	taskfile := readPackagingFixture(t, "../../../build/darwin/Taskfile.yml")

	core.AssertContains(t, taskfile, `-tags production{{if .EXTRA_TAGS}},{{.EXTRA_TAGS}}{{end}}`)
}
