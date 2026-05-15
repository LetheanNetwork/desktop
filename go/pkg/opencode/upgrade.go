// SPDX-Licence-Identifier: EUPL-1.2

// Upgrade — pulls `lthn/dev:latest` from the configured registry +
// restarts any running sandbox if the digest changed. Per
// RFC.opencode.md §7 "Image bump".
//
// v1 scope is user-driven, not auto-detected: the user clicks
// "Check for updates" / runs `lthn opencode upgrade`, lthn shells
// out to `docker pull`, parses the output for "newer image
// downloaded" vs "image is up to date", and restarts the container
// on a real update. Background-poll + on-card notification banner
// is a v2 — keeps this iteration small.
//
// Parsing relies on docker's stable Status lines:
//   - "Status: Image is up to date for lthn/dev:latest"
//   - "Status: Downloaded newer image for lthn/dev:latest"

package opencode

import (
	"strings"

	core "dappco.re/go"
)

// UpgradeResult captures the outcome of a pull + restart cycle.
type UpgradeResult struct {
	// Updated is true when the pull fetched a newer digest. False
	// means the image was already current.
	Updated bool `json:"updated"`
	// Digest is the resulting manifest digest (after pull).
	Digest string `json:"digest"`
	// Restarted lists sandbox ids that were stopped+respawned on
	// the new image. Empty when Updated is false or nothing was
	// running at upgrade time.
	Restarted []string `json:"restarted"`
}

// Upgrade pulls the configured image (defaults to lthn/dev:latest)
// + restarts any running sandbox on the new image when the pull
// produced a new digest.
//
// Returns Ok(UpgradeResult). Errors from the pull surface as Fail;
// errors from per-sandbox restart are logged but don't fail the
// overall upgrade (partial success is better than blocking).
//
// Usage example:
//
//	r := svc.Upgrade()
//	if r.OK { up := r.Value.(opencode.UpgradeResult); _ = up }
func (s *Service) Upgrade() core.Result {
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
	if strings.Contains(out, "Downloaded newer image") {
		res.Updated = true
	} else if strings.Contains(out, "Image is up to date") {
		res.Updated = false
	} else {
		// Unrecognised output — assume not-updated to avoid
		// unnecessary restarts. The Digest still surfaces so
		// callers can compare across calls.
		res.Updated = false
	}

	// If something updated AND something is running, restart on
	// the new image. Restart = stop (which preserves the enabled
	// flag) + spawn fresh.
	if res.Updated {
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
	for _, line := range strings.Split(pullOutput, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Digest:") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "Digest:"))
	}
	return ""
}
