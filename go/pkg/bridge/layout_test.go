// SPDX-Licence-Identifier: EUPL-1.2

package bridge_test

import (

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/bridge"
)

func TestLayout_WindowStateJSON(t *core.T) {
	state := bridge.WindowState{Name: "chat", X: 10, Y: 20, Width: 800, Height: 600, Visible: true}
	raw := core.JSONMarshal(state)
	core.AssertTrue(t, raw.OK)
	var decoded bridge.WindowState
	r := core.JSONUnmarshal(raw.Value.([]byte), &decoded)
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, state.Name, decoded.Name)
	core.AssertEqual(t, state.Width, decoded.Width)
}

func TestLayout_LayoutJSON(t *core.T) {
	layout := bridge.Layout{
		Name:    "autosave",
		SavedAt: core.Unix(100, 0).UTC(),
		Windows: []bridge.WindowState{
			{Name: "tray", Width: 320, Height: 240, Visible: true},
		},
	}
	raw := core.JSONMarshal(layout)
	core.AssertTrue(t, raw.OK)
	var decoded bridge.Layout
	r := core.JSONUnmarshal(raw.Value.([]byte), &decoded)
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "autosave", decoded.Name)
	core.AssertEqual(t, 1, len(decoded.Windows))
}
