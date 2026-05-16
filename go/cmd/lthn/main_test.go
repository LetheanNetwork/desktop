// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	core "dappco.re/go"
	lthn "dappco.re/lthn/desktop"
)

func TestMain_CmdVersionLabel_Good_PreservesTagPrefix(t *core.T) {
	original := lthn.Version
	t.Cleanup(func() { lthn.Version = original })
	lthn.Version = "v1.2.3"

	core.AssertEqual(t, "v1.2.3", cmdVersionLabel())
}

func TestMain_CmdVersionLabel_Bad_AddsMissingTagPrefix(t *core.T) {
	original := lthn.Version
	t.Cleanup(func() { lthn.Version = original })
	lthn.Version = "1.2.3"

	core.AssertEqual(t, "v1.2.3", cmdVersionLabel())
}

func TestMain_CmdVersionLabel_Ugly_PreservesDirtyTag(t *core.T) {
	original := lthn.Version
	t.Cleanup(func() { lthn.Version = original })
	lthn.Version = "v1.2.3-4-gabc123-dirty"

	core.AssertEqual(t, "v1.2.3-4-gabc123-dirty", cmdVersionLabel())
}
