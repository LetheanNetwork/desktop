// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus Mantis #1667 / #1679 (RFC.image-allowlist v1.1, ADD-3) —
// bridge is a parallel runtime surface to sandbox.Spawn. The bridge-
// tier gate fires BEFORE forwarding to sandbox.Spawn so callers that
// reach the runtime via bridge MUST clear the imagetrust allowlist.
//
// Tests live in package bridge (not bridge_test) so they can drive
// the unexported toolSandboxSpawn handler directly without needing a
// live HTTP loop.

package bridge

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/imagetrust"
	"dappco.re/lthn/desktop/pkg/sandbox"
)

// bridgeWithSandbox wires a minimal Core with both the bridge and
// sandbox services registered, returning the bridge for direct handler
// invocation.
func bridgeWithSandbox() *Service {
	c := core.New(
		core.WithName("bridge", RegisterService(Options{Port: 9878})),
		core.WithName("sandbox", sandbox.Register),
	)
	br, _ := core.ServiceFor[*Service](c, "bridge")
	return br
}

// Bridge gate REJECTS a disallowed image BEFORE forwarding to sandbox.
// The response carries the typed `error_code` short tag so an MCP
// client can render the rejection reason without parsing free-form text.
func TestBridge_Sandbox_GateFiresOnDisallowedImage_Bad(t *core.T) {
	br := bridgeWithSandbox()
	core.AssertNotNil(t, br)
	resp := br.toolSandboxSpawn(map[string]any{
		"image":   "evil.example.com/foo",
		"command": "sh",
	})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "image_registry_not_allowed", resp["error_code"])
}

// Bridge gate REJECTS an IMDS-shaped image with the granular Q-4
// `image_imds` tag (ADD-1 pre-allowlist check survives even hostile
// allowlist adds).
func TestBridge_Sandbox_GateRejectsIMDSImage_Bad(t *core.T) {
	br := bridgeWithSandbox()
	core.AssertNotNil(t, br)
	resp := br.toolSandboxSpawn(map[string]any{
		"image":   "169.254.169.254/v2/credentials",
		"command": "sh",
	})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "image_imds", resp["error_code"])
}

// Bridge gate REJECTS a flag-injection-shaped image with the
// `image_leading_dash` tag — defence in depth on top of the argv `--`
// terminator in sandbox.buildRunArgs.
func TestBridge_Sandbox_GateRejectsLeadingDash_Bad(t *core.T) {
	br := bridgeWithSandbox()
	core.AssertNotNil(t, br)
	resp := br.toolSandboxSpawn(map[string]any{
		"image":   "-v /etc/shadow:/etc/shadow",
		"command": "sh",
	})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "image_leading_dash", resp["error_code"])
}

// Bridge gate PASSES an allowlisted image through to sandbox (which
// then enforces its own gate as defence in depth + handles command
// validation). The container runtime is not present in CI so we
// assert the gate didn't short-circuit with image_* — any later
// failure shape is allowed.
func TestBridge_Sandbox_GatePassesAllowlistedImage_Good(t *core.T) {
	br := bridgeWithSandbox()
	core.AssertNotNil(t, br)
	resp := br.toolSandboxSpawn(map[string]any{
		"image":   "ghcr.io/owner/img:1.0",
		"command": "echo",
	})
	// If anything failed, the failure MUST NOT be an imagetrust
	// gate-reject — confirms the bridge gate let the allowlisted
	// image through to the downstream sandbox.Spawn surface.
	if ok, _ := resp["ok"].(bool); !ok {
		code, _ := resp["error_code"].(string)
		core.AssertNotEqual(t, "image_registry_not_allowed", code)
		core.AssertNotEqual(t, "image_imds", code)
		core.AssertNotEqual(t, "image_leading_dash", code)
		core.AssertNotEqual(t, "image_malformed", code)
		core.AssertNotEqual(t, "image_malformed_digest", code)
	}
}

// toolSandboxDetect surfaces available container runtimes. D-2a keeps
// Detect on *Service directly (read-only runtime probe, different
// threat class from spawn).
func TestBridge_Sandbox_ToolSandboxDetect_Good(t *core.T) {
	br := bridgeWithSandbox()
	resp := br.toolSandboxDetect()
	core.AssertEqual(t, true, resp["ok"])
	core.AssertNotNil(t, resp["value"])
}

func TestBridge_Sandbox_ToolSandboxDetect_Bad_ServiceUnavailable(t *core.T) {
	s := &Service{}
	resp := s.toolSandboxDetect()
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "sandbox service unavailable", resp["error"])
}

func TestBridge_Sandbox_SandboxSvc_Bad_NilService(t *core.T) {
	var s *Service
	core.AssertNil(t, s.sandboxSvc())
}

func TestBridge_Sandbox_SandboxSvc_Bad_NoServiceRuntime(t *core.T) {
	s := &Service{}
	core.AssertNil(t, s.sandboxSvc())
}

func TestBridge_Sandbox_SandboxSvc_Bad_ServiceNotRegistered(t *core.T) {
	c := core.New()
	s := &Service{ServiceRuntime: core.NewServiceRuntime[Options](c, Options{}), port: 9999}
	core.AssertNil(t, s.sandboxSvc())
}

func TestBridge_Sandbox_ToolSandboxSpawn_Bad_ServiceUnavailable(t *core.T) {
	s := &Service{}
	resp := s.toolSandboxSpawn(map[string]any{"image": "ghcr.io/owner/img:1.0", "command": "echo"})
	core.AssertEqual(t, false, resp["ok"])
	core.AssertEqual(t, "sandbox service unavailable", resp["error"])
}

// ErrorCode contract sanity from the bridge's perspective — the
// bridge response uses ErrorCode() exactly to derive the short tag
// surfaced as `error_code`.
func TestBridge_Sandbox_ErrorCodeStableAcrossBridgeBoundary_Good(t *core.T) {
	// Sanity: imagetrust.ErrorCode classifies the gate's own typed
	// error to the same short tag the bridge layer surfaces.
	err := imagetrust.IsAllowedImage("evil.example.com/foo")
	core.AssertNotNil(t, err)
	core.AssertEqual(t, "image_registry_not_allowed", imagetrust.ErrorCode(err))
}
