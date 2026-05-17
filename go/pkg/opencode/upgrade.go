// SPDX-Licence-Identifier: EUPL-1.2

// Upgrade — pulls `lthn/dev:latest` from the configured registry +
// (optionally) restarts any running sandbox if the digest changed.
// Per RFC.opencode.md §7 "Image bump".
//
// v1 scope is user-driven, not auto-detected: the user clicks
// "Check for updates" / runs `lthn opencode upgrade`, lthn shells
// out to `docker pull`, parses the output for "newer image
// downloaded" vs "image is up to date", and (when explicitly
// permitted) restarts the container on a real update.
// Background-poll + on-card notification banner is a v2 — keeps
// this iteration small.
//
// Cerberus #22 MED-2 / Mantis #1619 — supply-chain hardening v0:
//
//   - User-accept gate. UpgradeWithConsent(UpgradeInput) refuses
//     with "upgrade.requires_confirmation" unless ConfirmedByUser
//     is true. The legacy parameterless Upgrade() is now equivalent
//     to UpgradeWithConsent(UpgradeInput{}) → fail-closed. Callers
//     that genuinely want to pull must opt in explicitly.
//   - No silent auto-restart. UpgradeInput.RestartSandboxes defaults
//     false; the pull happens but running sandboxes keep their old
//     image until the caller schedules a restart. A user-driven
//     "Pull AND restart" flow sets RestartSandboxes=true.
//
// Deferred to follow-up tickets (filed alongside #1619):
//
//   - Image digest pinning (per-release pinned digest in an
//     UpgradeRecord schema — bigger surface).
//   - Image signature verification (cosign / notary integration —
//     bigger surface again).
//
// Parsing relies on docker's stable Status lines:
//   - "Status: Image is up to date for lthn/dev:latest"
//   - "Status: Downloaded newer image for lthn/dev:latest"

package opencode

import (

	core "dappco.re/go"
)

// UpgradeInput governs a single Upgrade call. v0 carries only the
// user-accept gate + the explicit-restart opt-in (Cerberus #22 MED-2);
// future fields (PinnedDigest, RequireSignature, …) land here without
// breaking the call shape.
//
// Usage example:
//
//	in := opencode.UpgradeInput{ConfirmedByUser: true, RestartSandboxes: false}
//	r := svc.UpgradeWithConsent(in)
type UpgradeInput struct {
	// ConfirmedByUser MUST be true for the pull to proceed. The
	// caller is asserting that an actual human (not a cron / poll
	// loop / drive-by HTTP request) approved this specific pull.
	// Default false → Fail("upgrade.requires_confirmation").
	ConfirmedByUser bool `json:"confirmed_by_user"`

	// RestartSandboxes, when true, makes a successful pull that
	// produced a new digest also stop + respawn every running
	// sandbox on the new image. Default false → the pull lands
	// but running sandboxes keep their pre-pull image until the
	// caller schedules a restart out-of-band. The Restarted field
	// of UpgradeResult is empty when this is false.
	RestartSandboxes bool `json:"restart_sandboxes"`
}

// UpgradeResult captures the outcome of a pull + restart cycle.
type UpgradeResult struct {
	// Updated is true when the pull fetched a newer digest. False
	// means the image was already current.
	Updated bool `json:"updated"`
	// Digest is the resulting manifest digest (after pull).
	Digest string `json:"digest"`
	// Restarted lists sandbox ids that were stopped+respawned on
	// the new image. Empty when Updated is false, when
	// UpgradeInput.RestartSandboxes was false, or when nothing was
	// running at upgrade time.
	Restarted []string `json:"restarted"`
}

// Upgrade is the legacy entry point. It is now equivalent to
// UpgradeWithConsent(UpgradeInput{}) — i.e. always fails with
// "upgrade.requires_confirmation". Kept so existing call-sites
// (control.go HTTP handler, wails.go bridge) compile while the
// follow-up wiring tickets thread an explicit UpgradeInput through.
//
// Usage example:
//
//	r := svc.Upgrade()
//	// r.OK == false; r.Code() == "upgrade.requires_confirmation"
func (s *Service) Upgrade() core.Result {
	return s.UpgradeWithConsent(UpgradeInput{})
}

// UpgradeWithConsent pulls the configured image (defaults to
// lthn/dev:latest) when the caller has explicitly confirmed, and —
// when in.RestartSandboxes is true — restarts any running sandbox
// on the new image after a digest change.
//
// Returns Ok(UpgradeResult). Errors from the pull surface as Fail;
// errors from per-sandbox restart are logged but don't fail the
// overall upgrade (partial success is better than blocking).
//
// Cerberus #22 MED-2 / Mantis #1619: when in.ConfirmedByUser is
// false, the function refuses immediately with
// "upgrade.requires_confirmation" — no network call, no side
// effects. This closes the silent supply-chain-pull attack vector
// where a compromised registry could have RCE-shaped impact on
// every running sandbox without the operator approving the swap.
//
// Usage example:
//
//	in := opencode.UpgradeInput{ConfirmedByUser: true}  // pull only
//	r := svc.UpgradeWithConsent(in)
//	if r.OK { up := r.Value.(opencode.UpgradeResult); _ = up }
func (s *Service) UpgradeWithConsent(in UpgradeInput) core.Result {
	if !in.ConfirmedByUser {
		return core.Fail(core.E("opencode.Upgrade",
			"upgrade.requires_confirmation: user has not approved this image pull (Cerberus #22 MED-2 / Mantis #1619)",
			nil))
	}

	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("opencode.Upgrade", "process service unavailable", nil))
	}

	// docker pull is potentially slow on a real update — 60s is
	// generous for any image we'd realistically ship.
	ctx, cancel := core.WithTimeout(core.Background(), 60*core.Second)
	defer cancel()

	pullR := ps.Run(ctx, s.runtime(), "pull", s.image())
	if !pullR.OK {
		return pullR
	}
	out, _ := pullR.Value.(string)

	res := UpgradeResult{
		Digest: parsePullDigest(out),
	}
	if core.Contains(out, "Downloaded newer image") {
		res.Updated = true
	} else if core.Contains(out, "Image is up to date") {
		res.Updated = false
	} else {
		// Unrecognised output — assume not-updated to avoid
		// unnecessary restarts. The Digest still surfaces so
		// callers can compare across calls.
		res.Updated = false
	}

	// Restart only when (a) the pull produced a new image AND
	// (b) the caller explicitly asked for in-place restart. v0
	// default is to leave running sandboxes alone so the
	// behaviour matches operator expectation ("I pulled, I did
	// not redeploy"). See Cerberus #22 MED-2 / Mantis #1619.
	if res.Updated && in.RestartSandboxes {
		statusR := s.Status()
		if statusR.OK {
			running, _ := statusR.Value.([]Sandbox)
			for _, sb := range running {
				if r := s.Stop(sb.ID); !r.OK {
					core.Print(core.Stderr(),
						"opencode.Upgrade: stop %s failed: %s\n", sb.ID, r.Error())
					continue
				}
				if r := s.Start(""); r.OK {
					if newID, ok := r.Value.(string); ok {
						res.Restarted = append(res.Restarted, newID)
					}
				}
			}
		}
	}

	return core.Ok(res)
}

// parsePullDigest scans `docker pull` output for the "Digest: sha256:..."
// line and returns the bare digest. Empty string when not present.
//
// The shape is stable across docker / podman / nerdctl:
//
//	Digest: sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84
func parsePullDigest(pullOutput string) string {
	for _, line := range core.Split(pullOutput, "\n") {
		line = core.Trim(line)
		if !core.HasPrefix(line, "Digest:") {
			continue
		}
		return core.Trim(core.TrimPrefix(line, "Digest:"))
	}
	return ""
}
