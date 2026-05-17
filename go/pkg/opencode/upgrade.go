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
// Cerberus #22 MED-2 / Mantis #1621 — supply-chain hardening v1
// (digest pinning):
//
//   - Digest-pinned pulls. UpgradeInput.ImageDigest takes a
//     "sha256:<64 hex>" digest. UpgradeWithConsent pulls
//     "<repo>@sha256:<digest>" instead of "<repo>:latest" so the
//     registry CANNOT serve a different image under the same tag —
//     the daemon refuses any artefact whose content hash doesn't
//     match. After pull, the parsed Digest line is compared back to
//     the requested ImageDigest; any mismatch surfaces as
//     "upgrade.digest_mismatch" + fail-closed.
//   - Empty digest fail-closed by default with
//     "upgrade.digest_required". Operators MUST think about what
//     they are pulling. The pre-#1621 ":latest" fallback is gone;
//     callers (HTTP body, Wails param, future UpgradeRecord schema,
//     manual operator input) must thread a pinned digest through.
//     A follow-up ticket tracks the HTTP+Wails frontend wiring.
//
// Deferred to follow-up tickets:
//
//   - Image signature verification (cosign / notary integration —
//     #1622, bigger surface again — sits on top of the digest pin).
//   - HTTP + Wails callers learn to pass ImageDigest (frontend
//     wiring across the api/control + wails bindings, filed as a
//     #1621 follow-up).
//
// Parsing relies on docker's stable Status lines:
//   - "Status: Image is up to date for lthn/dev:latest"
//   - "Status: Downloaded newer image for lthn/dev:latest"

package opencode

import (

	core "dappco.re/go"
)

// UpgradeInput governs a single Upgrade call. v0 carried the user-
// accept gate + the explicit-restart opt-in (Cerberus #22 MED-2 /
// Mantis #1619); v1 (Mantis #1621) adds ImageDigest for sha256-pinned
// pulls. Future fields (RequireSignature, …) land here without
// breaking the call shape.
//
// Usage example:
//
//	in := opencode.UpgradeInput{
//	    ConfirmedByUser:  true,
//	    ImageDigest:      "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84",
//	    RestartSandboxes: false,
//	}
//	r := svc.UpgradeWithConsent(in)
type UpgradeInput struct {
	// ConfirmedByUser MUST be true for the pull to proceed. The
	// caller is asserting that an actual human (not a cron / poll
	// loop / drive-by HTTP request) approved this specific pull.
	// Default false → Fail("upgrade.requires_confirmation").
	ConfirmedByUser bool `json:"confirmed_by_user"`

	// ImageDigest pins the pull to a specific sha256 manifest digest
	// of the form "sha256:<64 lowercase hex>". When set, the pull
	// targets "<repo>@<ImageDigest>" instead of "<repo>:<tag>" — the
	// runtime daemon (docker / podman / nerdctl) refuses any artefact
	// whose content hash doesn't match, blocking a compromised
	// registry from substituting a different image under the same
	// tag. After the pull, the parsed Digest line is compared back
	// to ImageDigest; any mismatch surfaces as
	// "upgrade.digest_mismatch" + fail-closed.
	//
	// Default empty → Fail("upgrade.digest_required"). Mantis #1621
	// ships fail-closed-by-default: operators MUST think about what
	// they are pulling. Callers (HTTP body, Wails param, future
	// UpgradeRecord schema, manual operator input) thread a pinned
	// digest through; pulling "<repo>:latest" without a digest is
	// no longer reachable.
	//
	// Source of the digest is out-of-scope here — caller
	// responsibility (release manifest, signed UpgradeRecord,
	// operator paste-in). #1622 layers cosign / notary verification
	// on top.
	ImageDigest string `json:"image_digest"`

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

// UpgradeWithConsent pulls the configured image pinned to the
// requested in.ImageDigest when the caller has explicitly confirmed,
// and — when in.RestartSandboxes is true — restarts any running
// sandbox on the new image after a digest change.
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
// Cerberus #22 MED-2 / Mantis #1621: when in.ImageDigest is empty
// or malformed, the function refuses with "upgrade.digest_required"
// (or "upgrade.digest_invalid") — no network call, no side
// effects. Operators MUST commit to a specific manifest digest. The
// pull then targets "<repo>@<digest>"; the runtime daemon refuses
// any artefact whose content hash doesn't match, and the post-pull
// "Digest:" line is compared back to in.ImageDigest — a divergence
// surfaces as "upgrade.digest_mismatch" + fail-closed.
//
// Usage example:
//
//	in := opencode.UpgradeInput{
//	    ConfirmedByUser: true,
//	    ImageDigest:     "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84",
//	}
//	r := svc.UpgradeWithConsent(in)
//	if r.OK { up := r.Value.(opencode.UpgradeResult); _ = up }
func (s *Service) UpgradeWithConsent(in UpgradeInput) core.Result {
	if !in.ConfirmedByUser {
		return core.Fail(core.E("opencode.Upgrade",
			"upgrade.requires_confirmation: user has not approved this image pull (Cerberus #22 MED-2 / Mantis #1619)",
			nil))
	}

	// Digest gate — pre-empts proc lookup + pull side effects. An
	// empty or malformed digest is a "caller forgot to think about
	// what they are pulling" case; surface as a distinct error code
	// so the frontend can render "pick a release digest" rather
	// than "the upgrade substrate broke".
	if in.ImageDigest == "" {
		return core.Fail(core.E("opencode.Upgrade",
			"upgrade.digest_required: ImageDigest is empty — pin a sha256:<64 hex> manifest digest (Cerberus #22 MED-2 / Mantis #1621)",
			nil))
	}
	if !validSHA256Digest(in.ImageDigest) {
		return core.Fail(core.E("opencode.Upgrade",
			"upgrade.digest_invalid: ImageDigest must be sha256:<64 lowercase hex> (Mantis #1621)",
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

	pullR := ps.Run(ctx, s.runtime(), "pull", pinnedPullRef(s.image(), in.ImageDigest))
	if !pullR.OK {
		return pullR
	}
	out, _ := pullR.Value.(string)

	res := UpgradeResult{
		Digest: parsePullDigest(out),
	}

	// Belt-and-braces verification: the runtime daemon SHOULD have
	// already refused a mismatched artefact at the wire level for
	// a digest-pinned pull. We re-compare the parsed Digest line
	// against the requested ImageDigest so a runtime bug, an
	// intermediary cache MITM, or a future change to docker pull's
	// digest-pin enforcement can't silently land the wrong image.
	// Fail-closed: do NOT restart sandboxes on a mismatched pull.
	if res.Digest != "" && !equalDigest(res.Digest, in.ImageDigest) {
		return core.Fail(core.E("opencode.Upgrade",
			"upgrade.digest_mismatch: registry served digest "+res.Digest+
				" but caller pinned "+in.ImageDigest+" (Mantis #1621)",
			nil))
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

// validSHA256Digest returns true when s is exactly "sha256:" + 64
// lowercase hex characters — the canonical OCI manifest digest shape.
//
//	validSHA256Digest("sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84") // true
//	validSHA256Digest("sha256:CA59EB28")                                                          // false (wrong length, uppercase)
//	validSHA256Digest("md5:abcd")                                                                 // false (wrong algorithm)
//	validSHA256Digest("")                                                                         // false
func validSHA256Digest(s string) bool {
	const prefix = "sha256:"
	if !core.HasPrefix(s, prefix) {
		return false
	}
	hex := s[len(prefix):]
	if len(hex) != 64 {
		return false
	}
	for i := 0; i < len(hex); i++ {
		b := hex[i]
		switch {
		case b >= '0' && b <= '9':
		case b >= 'a' && b <= 'f':
		default:
			return false
		}
	}
	return true
}

// pinnedPullRef builds the digest-pinned pull reference from a
// configured image string and a validated sha256 digest. Strips any
// trailing ":<tag>" so the result is the canonical "<repo>@sha256:..."
// form regardless of whether the caller's configured image carries a
// tag.
//
//	pinnedPullRef("lthn/dev:latest", "sha256:abc…") // "lthn/dev@sha256:abc…"
//	pinnedPullRef("lthn/dev",        "sha256:abc…") // "lthn/dev@sha256:abc…"
//	pinnedPullRef("registry.example.com:5000/lthn/dev:latest", "sha256:abc…")
//	    // "registry.example.com:5000/lthn/dev@sha256:abc…"
//
// Registry-port colons (e.g. "registry.example.com:5000/…") are
// preserved — only a tag colon AFTER the last slash is stripped.
func pinnedPullRef(image string, digest string) string {
	repo := image
	slash := core.LastIndex(image, "/")
	tail := image
	if slash >= 0 {
		tail = image[slash+1:]
	}
	if colon := core.Index(tail, ":"); colon >= 0 {
		// Strip "<tag>" portion from the tail.
		if slash >= 0 {
			repo = image[:slash+1] + tail[:colon]
		} else {
			repo = tail[:colon]
		}
	}
	return repo + "@" + digest
}

// equalDigest compares two sha256 digests case-insensitively on the
// hex portion. OCI canonicalises to lowercase but defensive
// case-fold keeps us safe if a future runtime reports uppercase.
//
//	equalDigest("sha256:ABCDEF…", "sha256:abcdef…") // true
//	equalDigest("sha256:abc",     "sha512:abc")     // false (algorithm differs)
func equalDigest(a string, b string) bool {
	return core.Lower(a) == core.Lower(b)
}
