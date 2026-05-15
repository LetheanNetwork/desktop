// SPDX-Licence-Identifier: EUPL-1.2

// Provider enumeration — opencode-serve loads providers from its
// own config (host credentials, lthn-injected provider.lthn, etc.)
// and exposes them at GET /provider. This file wraps that endpoint
// so callers don't have to round-trip through the reverse-proxy
// themselves + parse + auth-inject.
//
// Per RFC.opencode.md §4.3 method list + §5.1: the Fleet → Agents
// page renders one card per provider returned here. Each card shows
// model list + an in-Fleet toggle. Source of truth = opencode's own
// /provider response, not a local mirror.

package opencode

import (
	goio "io"
	"net/http"

	core "dappco.re/go"
)

// ProviderList returns opencode-serve's /provider response for the
// named sandbox as a string (caller decodes the JSON shape). Returns
// Fail when the sandbox isn't running or the upstream call errors.
//
// Usage example:
//
//	r := svc.ProviderList("oc-1735843891234")
//	if r.OK { raw := r.Value.(string); _ = raw }
func (s *Service) ProviderList(id string) core.Result {
	if core.Trim(id) == "" {
		return core.Fail(core.E("opencode.ProviderList", "id is required", nil))
	}
	target, r := s.targetFor(id)
	if !r.OK {
		return r
	}
	body, code, err := s.callOpenCode(http.MethodGet, target+"/provider", nil)
	if err != nil {
		return core.Fail(core.E("opencode.ProviderList", "call failed", err))
	}
	if code >= 400 {
		return core.Fail(core.E("opencode.ProviderList",
			core.Sprintf("upstream returned %d: %s", code, body), nil))
	}
	return core.Ok(body)
}

// targetFor resolves the in-process reverse-proxy target URL for a
// sandbox id by re-reading the orm record. Returns Fail when the
// sandbox isn't running.
//
// We resolve through the orm record (NOT the proxy's targets map)
// so the call works even when the proxy isn't holding a forwarder
// — i.e. for direct internal calls from inside the same process.
func (s *Service) targetFor(id string) (string, core.Result) {
	infoR := s.Inspect(id)
	if !infoR.OK {
		return "", infoR
	}
	sb, ok := infoR.Value.(Sandbox)
	if !ok {
		return "", core.Fail(core.E("opencode.targetFor", "inspect returned unexpected shape", nil))
	}
	if sb.Status != StatusRunning {
		return "", core.Fail(core.E("opencode.targetFor",
			"sandbox is not running (status="+sb.Status+")", nil))
	}
	return core.Sprintf("http://127.0.0.1:%d", sb.HostPort), core.Ok(nil)
}

// callOpenCode is the shared internal HTTP client for direct calls
// to opencode-serve (bypassing the reverse-proxy because we ARE the
// reverse-proxy). Auto-injects the Basic Auth header and returns
// (body, status, err).
func (s *Service) callOpenCode(method, url string, body goio.Reader) (string, int, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return "", 0, err
	}
	s.applyAuth(req)
	client := &http.Client{Timeout: 10 * core.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := goio.ReadAll(resp.Body)
	return string(raw), resp.StatusCode, nil
}
