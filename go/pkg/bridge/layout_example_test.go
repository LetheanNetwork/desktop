// SPDX-Licence-Identifier: EUPL-1.2

package bridge_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/bridge"
)

func ExampleWindowState() {
	_ = bridge.WindowState{Name: "chat", Width: 800, Height: 600, Visible: true}
}

func ExampleLayout() {
	_ = bridge.Layout{
		Name:    "autosave",
		SavedAt: core.Unix(100, 0).UTC(),
		Windows: []bridge.WindowState{
			{Name: "tray", Width: 320, Height: 240, Visible: true},
		},
	}
}
