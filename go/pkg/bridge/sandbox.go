// SPDX-Licence-Identifier: EUPL-1.2

// sandbox.go — bridge tools for spawning OCI containers via
// pkg/sandbox. Lets bridge.sh / curl exercise sandbox.Spawn
// without the WebView layer.

package bridge

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/imagetrust"
	"dappco.re/lthn/desktop/pkg/sandbox"
)

// sandboxSvc resolves the sandbox service. Retained for Detect access
// (D-2a adjudication — Detect is runtime introspection, different
// threat class from spawn-from-renderer, so it stays uppercase on
// *Service and is reached directly rather than via SpawnPort).
func (s *Service) sandboxSvc() *sandbox.Service {
	if s == nil || s.ServiceRuntime == nil {
		return nil
	}
	c := s.Core()
	if c == nil {
		return nil
	}
	sb, _ := core.ServiceFor[*sandbox.Service](c, "sandbox")
	return sb
}

// sandboxPort resolves the sandbox substrate via the SpawnPort
// closure-adapter. Mantis #1664 Phase B (Cerberus #55 ADD-2) — Spawn /
// SpawnLong / Kill / ListHandles / GetHandle / InstallID on *Service
// are now Wails-table shims that reject with ErrTierGoOnly; substrate
// access from in-Go consumers MUST route through the typed port. The
// lazy-resolver shape mirrors sandboxSvc so existing consumers keep
// their `if port == nil { ... }` guard pattern. D-3b adjudication — no
// composition-root refactor; port resolved at callsite.
func (s *Service) sandboxPort() sandbox.SpawnPort {
	return sandbox.NewSpawnPort(s.sandboxSvc())
}

// toolSandboxDetect surfaces what container runtimes the host has.
// Detect stays on *Service directly (D-2a — read-only runtime probe).
func (s *Service) toolSandboxDetect() map[string]any {
	sb := s.sandboxSvc()
	if sb == nil {
		return map[string]any{"ok": false, "error": "sandbox service unavailable"}
	}
	r := sb.Detect()
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "value": r.Value}
}

// toolSandboxSpawn fires a one-shot container via sandbox.Spawn.
// params: { image, command, args?, runtime?, timeout_seconds? }
//
// Cerberus Mantis #1667 + #1679 (RFC v1.1 §2.4 / ADD-3) — bridge is
// a PARALLEL runtime surface to wails-bound sandbox.Spawn. The gate
// fires HERE at the bridge entry-point BEFORE forwarding so any
// caller that reaches the runtime via the bridge MUST first clear
// the same imagetrust.IsAllowedImage check the sandbox tier enforces.
// Sandbox-tier defence-in-depth still fires inside Spawn — composing
// two independent gates is the security property, not redundant.
func (s *Service) toolSandboxSpawn(params map[string]any) map[string]any {
	port := s.sandboxPort()
	if port == nil {
		return map[string]any{"ok": false, "error": "sandbox service unavailable"}
	}
	input := sandbox.SpawnInput{
		Image:          paramString(params, "image", ""),
		Command:        paramString(params, "command", ""),
		Args:           stringSliceParam(params, "args"),
		Runtime:        paramString(params, "runtime", ""),
		TimeoutSeconds: paramInt(params, "timeout_seconds", 0),
	}
	// Skip the gate only when the caller explicitly passed an empty
	// image — sandbox.prepareSpawnInput substitutes the default and
	// then re-validates. Allow that default-substitution path to do
	// its own validation downstream so the bridge gate doesn't fire
	// a false ErrEmptyImageRef ahead of the substitution.
	if input.Image != "" {
		if err := imagetrust.IsAllowedImage(input.Image); err != nil {
			return map[string]any{
				"ok":         false,
				"error":      err.Error(),
				"error_code": imagetrust.ErrorCode(err),
			}
		}
	}
	r := port.Spawn(input)
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "value": r.Value}
}
