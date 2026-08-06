// SPDX-Licence-Identifier: EUPL-1.2

// Internal-package tests for tools.go's unexported marshalSchema
// helper.
package tools

import (
	core "dappco.re/go"
)

func TestMarshalSchema_Good(t *core.T) {
	got := marshalSchema(map[string]any{"type": "object"})
	core.AssertContains(t, got, "\"type\"")
	core.AssertContains(t, got, "\"object\"")
}

func TestMarshalSchema_Bad_Empty(t *core.T) {
	core.AssertEqual(t, "", marshalSchema(nil))
	core.AssertEqual(t, "", marshalSchema(map[string]any{}))
}

// TestMarshalSchema_Ugly_Unmarshalable — a map value JSON cannot
// encode (a channel) drives JSONMarshalIndent's own failure branch,
// which marshalSchema swallows into "" for the WebView's placeholder
// state rather than propagating an error.
func TestMarshalSchema_Ugly_Unmarshalable(t *core.T) {
	got := marshalSchema(map[string]any{"bad": make(chan int)})
	core.AssertEqual(t, "", got)
}
