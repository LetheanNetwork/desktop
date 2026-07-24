// SPDX-Licence-Identifier: EUPL-1.2

package desktop

import core "dappco.re/go"

func TestDesktop_TrayStatusTooltip_Good_CustomPrefixAndModel(t *core.T) {
	core.AssertEqual(
		t,
		"My Lethean — gemma.gguf — ready",
		trayStatusTooltip("My Lethean", "gemma.gguf"),
	)
}

func TestDesktop_TrayStatusTooltip_Bad_EmptyPrefixRemainsReadable(t *core.T) {
	core.AssertEqual(t, "no model loaded", trayStatusTooltip("", ""))
}

func TestDesktop_TrayStatusTooltip_Ugly_CustomPrefixWithoutModel(t *core.T) {
	core.AssertEqual(
		t,
		"My Lethean — no model loaded",
		trayStatusTooltip("My Lethean", ""),
	)
}
