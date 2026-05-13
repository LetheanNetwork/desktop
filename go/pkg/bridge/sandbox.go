// SPDX-Licence-Identifier: EUPL-1.2

// sandbox.go — bridge tools for spawning OCI containers via
// pkg/sandbox. Lets bridge.sh / curl exercise sandbox.Spawn
// without the WebView layer.

package bridge

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sandbox"
)

// sandboxSvc resolves the sandbox service.
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

// toolSandboxDetect surfaces what container runtimes the host has.
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
func (s *Service) toolSandboxSpawn(params map[string]any) map[string]any {
	sb := s.sandboxSvc()
	if sb == nil {
		return map[string]any{"ok": false, "error": "sandbox service unavailable"}
	}
	input := sandbox.SpawnInput{
		Image:          paramString(params, "image", ""),
		Command:        paramString(params, "command", ""),
		Args:           stringSliceParam(params, "args"),
		Runtime:        paramString(params, "runtime", ""),
		TimeoutSeconds: paramInt(params, "timeout_seconds", 0),
	}
	r := sb.Spawn(input)
	if !r.OK {
		return map[string]any{"ok": false, "error": r.Error()}
	}
	return map[string]any{"ok": true, "value": r.Value}
}
