// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the serverkey service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3 generate
// bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/serverkey/service.
//
// Stage B exposes:
//   - AccountStatus() — drives the auth-gate's setup-vs-auth fork
//   - IssueBootstrapToken() — mints the short-lived token the frontend
//     attaches to /v1/account/create requests
//
// Methods kept INTENTIONALLY narrow — the verify path is server-side
// only (middleware territory), and rotation / inspection surfaces are
// out of scope for Stage B.

package serverkey

import core "dappco.re/go"

// AccountStatus is the Wails-bound probe the auth-gate calls on mount.
// Returns AccountStatusOutput.HasUserAccount=false when no account
// exists yet (gate transitions to `setup`), true when at least one
// account is provisioned (gate transitions to `auth`).
//
// Usage example (TS):
//
//	import { AccountStatus } from "@desktop/serverkey/service";
//	const s = await AccountStatus();
//	if (!s.has_user_account) showSetupGate();
func (s *Service) WAccountStatus() core.Result {
	return s.AccountStatus()
}

// IssueBootstrapToken is the Wails-bound mint. The frontend MUST hold
// the returned token ONLY in the closure scope of the click handler
// that consumes it (Cerberus #1465) — never persist to localStorage /
// sessionStorage / cookie / IndexedDB / module cache.
//
// The TTL is 60s issuer-side specifically because the gate is
// permitted to mint per-attempt. On retry the gate calls this again
// instead of re-using a prior token.
//
// Usage example (TS):
//
//	import { IssueBootstrapToken } from "@desktop/serverkey/service";
//	const { token } = await IssueBootstrapToken();
//	await fetch("/v1/account/create", {
//	    method: "POST",
//	    headers: { "Authorization": "Bootstrap " + token },
//	    body: JSON.stringify(input),
//	});
func (s *Service) WIssueBootstrapToken() core.Result {
	return s.IssueBootstrapToken()
}
