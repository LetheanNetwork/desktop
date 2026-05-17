// SPDX-Licence-Identifier: EUPL-1.2

// Tests for Mantis #1617 HIGH (Cerberus #22 HIGH-2) — the wire response
// returned by hostConfigMerge (POST /v1/api/opencode/host-config) MUST
// NOT include the merged opencode.json bytes, because those bytes
// preserve any pre-existing user provider apiKey blocks (OpenAI,
// Anthropic, etc.) verbatim across the merge.
//
// The fix is structural: MergeHostConfigResult.Bytes carries `json:"-"`,
// so every caller that JSON-encodes the struct silently drops the field.
// These tests pin that boundary at the type-system level — if a future
// refactor swaps the tag back to `json:"bytes"`, the leak test fails
// before the change ships.

package opencode

import (
	"testing"

	core "dappco.re/go"
)

// TestHostConfigMerge_WireResponse_NoApiKeyLeak_Bad — the wire response
// returned by JSON-encoding a MergeHostConfigResult must NOT contain the
// merged file bytes, even when those bytes carry a pre-existing user
// apiKey from a provider block the merge left untouched.
//
// Construction mirrors the real success path: the user already had an
// OpenAI apiKey configured; the merge added our lthn provider; the
// pretty-printed result preserves the OpenAI block verbatim. The wire
// response MUST sanitise that bytes field out — Option B (`json:"-"`)
// catches it at the marshaller, no per-handler view-struct required.
func TestHostConfigMerge_WireResponse_NoApiKeyLeak_Bad(t *testing.T) {
	const userApiKey = "sk-proj-VICTIM-OPENAI-KEY-DO-NOT-LEAK"
	res := MergeHostConfigResult{
		Path:    "/home/user/.config/opencode/opencode.json",
		Profile: "default",
		Bytes: `{
  "provider": {
    "openai": {
      "options": {
        "apiKey": "` + userApiKey + `"
      }
    },
    "lthn": {
      "options": {
        "baseURL": "http://localhost:8000/v1"
      }
    }
  }
}`,
		Created: false,
	}
	r := core.JSONMarshal(res)
	core.AssertTrue(t, r.OK, "JSONMarshal failed: must encode MergeHostConfigResult cleanly")
	wire, _ := r.Value.([]byte)
	if core.Contains(string(wire), userApiKey) {
		t.Fatalf("hostConfigMerge wire response leaked user apiKey "+
			"(Mantis #1617). Got: %s", string(wire))
	}
	if core.Contains(string(wire), `"bytes"`) {
		t.Fatalf("hostConfigMerge wire response carries `bytes` field "+
			"— Option B requires `json:\"-\"` to suppress at marshal. "+
			"Got: %s", string(wire))
	}
}

// TestHostConfigMerge_WireResponse_RetainsPathProfileCreated_Good — the
// frontend / CLI still need the file path, applied profile name, and
// created flag in the wire response. Suppressing Bytes must not collapse
// the rest of the success-shape.
func TestHostConfigMerge_WireResponse_RetainsPathProfileCreated_Good(t *testing.T) {
	res := MergeHostConfigResult{
		Path:    "/home/user/.config/opencode/opencode.json",
		Profile: "default",
		Bytes:   `{"provider":{}}`,
		Created: true,
	}
	r := core.JSONMarshal(res)
	core.AssertTrue(t, r.OK, "JSONMarshal failed: must encode MergeHostConfigResult cleanly")
	wire := string(r.Value.([]byte))
	core.AssertTrue(t, core.Contains(wire, `"path":"/home/user/.config/opencode/opencode.json"`),
		"wire response must retain path field")
	core.AssertTrue(t, core.Contains(wire, `"profile":"default"`),
		"wire response must retain profile field")
	core.AssertTrue(t, core.Contains(wire, `"created":true`),
		"wire response must retain created field")
}

// TestMergeHostConfigResult_BytesAvailableInProcess_Good — Option B
// drops Bytes from the WIRE shape only; in-process Go callers (audit
// suppression decisions, reconciliation, internal diff display) still
// see the field by direct struct access. The point of `json:"-"` is to
// stop accidental leakage at the marshaller boundary, not to hide the
// field from the language. Regression guard: if someone "cleans up" by
// deleting the Bytes field entirely, this test fails.
func TestMergeHostConfigResult_BytesAvailableInProcess_Good(t *testing.T) {
	const merged = `{"provider":{"lthn":{}}}`
	res := MergeHostConfigResult{
		Path:    "/tmp/oc.json",
		Profile: "default",
		Bytes:   merged,
		Created: true,
	}
	if res.Bytes != merged {
		t.Fatalf("MergeHostConfigResult.Bytes lost its value through "+
			"struct construction: got %q want %q", res.Bytes, merged)
	}
}
