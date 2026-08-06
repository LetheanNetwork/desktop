// SPDX-Licence-Identifier: EUPL-1.2

// upgrade_pull_test.go — coverage for UpgradeWithConsent's substrate
// pull path (both consent + digest gates already covered in
// upgrade_test.go / upgrade_wire_test.go, which deliberately stopped
// at "gate passed, substrate unavailable"). `docker pull` is faked via
// Options.Runtime, echoing the canonical Status/Digest lines the real
// docker CLI emits — parsePullDigest / equalDigest / the
// digest_mismatch guard / the restart-sandboxes sweep are all only
// reachable past that call.

package opencode

import (
	"testing"

	core "dappco.re/go"
)

const pullTestDigest = "sha256:ca59eb28d5ea6a1f50c45a1f1df5c1a9286343e41b389fe89fb4ffac96dbeb84"

// dockerPullScript builds a fake-runtime script answering `pull <ref>`
// with the given canned stdout (mirroring real docker pull output).
func dockerPullScript(stdout string) string {
	return `cat <<'EOF'
` + stdout + `
EOF
exit 0
`
}

// TestParsePullDigest_Good / NoMatch_Good — pure parser coverage.
func TestParsePullDigest_Good(t *testing.T) {
	out := "latest: Pulling from lthn/dev\n" +
		"Digest: " + pullTestDigest + "\n" +
		"Status: Downloaded newer image for lthn/dev:latest\n"
	if got := parsePullDigest(out); got != pullTestDigest {
		t.Errorf("parsePullDigest = %q; want %q", got, pullTestDigest)
	}
}

func TestParsePullDigest_NoDigestLine_Good(t *testing.T) {
	if got := parsePullDigest("Status: Image is up to date for lthn/dev:latest\n"); got != "" {
		t.Errorf("parsePullDigest with no Digest line = %q; want empty", got)
	}
}

// TestUpgradeWithConsent_PullUpdated_Good — docker pull reports a new
// digest that matches the requested pin; Updated=true, no restart
// (RestartSandboxes defaults false).
func TestUpgradeWithConsent_PullUpdated_Good(t *testing.T) {
	out := "Digest: " + pullTestDigest + "\n" +
		"Status: Downloaded newer image for lthn/dev:latest\n"
	rt := fakeRuntime(t, dockerPullScript(out))
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: true, ImageDigest: pullTestDigest})
	if !r.OK {
		t.Fatalf("UpgradeWithConsent failed: %s", r.Error())
	}
	res, _ := r.Value.(UpgradeResult)
	if !res.Updated {
		t.Errorf("Updated = false; want true")
	}
	if res.Digest != pullTestDigest {
		t.Errorf("Digest = %q; want %q", res.Digest, pullTestDigest)
	}
	if len(res.Restarted) != 0 {
		t.Errorf("Restarted = %+v; want empty (RestartSandboxes was false)", res.Restarted)
	}
}

// TestUpgradeWithConsent_PullUpToDate_Good — "Image is up to date"
// status → Updated=false.
func TestUpgradeWithConsent_PullUpToDate_Good(t *testing.T) {
	out := "Digest: " + pullTestDigest + "\n" +
		"Status: Image is up to date for lthn/dev:latest\n"
	rt := fakeRuntime(t, dockerPullScript(out))
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: true, ImageDigest: pullTestDigest})
	if !r.OK {
		t.Fatalf("UpgradeWithConsent failed: %s", r.Error())
	}
	res, _ := r.Value.(UpgradeResult)
	if res.Updated {
		t.Errorf("Updated = true; want false (image was already current)")
	}
}

// TestUpgradeWithConsent_PullUnrecognisedOutput_Ugly — neither the
// "Downloaded newer image" nor "Image is up to date" marker is
// present; the function must default Updated=false rather than
// guessing, so an ambiguous docker CLI output never triggers an
// unwanted restart sweep.
func TestUpgradeWithConsent_PullUnrecognisedOutput_Ugly(t *testing.T) {
	out := "Digest: " + pullTestDigest + "\nsome-future-docker-output-shape\n"
	rt := fakeRuntime(t, dockerPullScript(out))
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: true, ImageDigest: pullTestDigest})
	if !r.OK {
		t.Fatalf("UpgradeWithConsent failed: %s", r.Error())
	}
	res, _ := r.Value.(UpgradeResult)
	if res.Updated {
		t.Errorf("Updated = true against unrecognised docker output; want false (safe default)")
	}
}

// TestUpgradeWithConsent_DigestMismatch_Bad — the registry serves a
// digest different from the one the caller pinned; fail-closed, no
// restart, regardless of the Status line.
func TestUpgradeWithConsent_DigestMismatch_Bad(t *testing.T) {
	served := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	out := "Digest: " + served + "\nStatus: Downloaded newer image for lthn/dev:latest\n"
	rt := fakeRuntime(t, dockerPullScript(out))
	svc := newTestService(t, Options{Runtime: rt})

	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: true, ImageDigest: pullTestDigest})
	if r.OK {
		t.Fatalf("UpgradeWithConsent with a mismatched served digest returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "upgrade.digest_mismatch") {
		t.Errorf("error = %q; want upgrade.digest_mismatch", r.Error())
	}
}

// TestUpgradeWithConsent_PullCommandFails_Bad — the runtime's `pull`
// invocation itself fails (non-zero exit); the failure propagates
// verbatim.
func TestUpgradeWithConsent_PullCommandFails_Bad(t *testing.T) {
	rt := fakeRuntime(t, "exit 1")
	svc := newTestService(t, Options{Runtime: rt})
	r := svc.UpgradeWithConsent(UpgradeInput{ConfirmedByUser: true, ImageDigest: pullTestDigest})
	if r.OK {
		t.Fatalf("UpgradeWithConsent with a failing pull returned OK; want Fail")
	}
}

// TestUpgradeWithConsent_RestartSweep_Good — a running sandbox is
// stopped + respawned when the pull reports a new digest AND the
// caller opted into RestartSandboxes. The fake runtime answers BOTH
// `pull` and `run`/`rm` (Start/Stop's own calls) generically.
func TestUpgradeWithConsent_RestartSweep_Good(t *testing.T) {
	fake := newFakeOpencodeServe(t)
	pinPortAllocation(t, fake.Server)

	out := "Digest: " + pullTestDigest + "\nStatus: Downloaded newer image for lthn/dev:latest\n"
	rt := fakeRuntime(t, `
case "$1" in
  pull)
`+dockerPullBody(out)+`
    ;;
  *)
    exit 0
    ;;
esac
`)
	svc := newTestService(t, Options{Runtime: rt})

	startR := svc.Start("")
	if !startR.OK {
		t.Fatalf("seed Start failed: %s", startR.Error())
	}
	originalID, _ := startR.Value.(string)

	r := svc.UpgradeWithConsent(UpgradeInput{
		ConfirmedByUser:  true,
		ImageDigest:      pullTestDigest,
		RestartSandboxes: true,
	})
	if !r.OK {
		t.Fatalf("UpgradeWithConsent (restart sweep) failed: %s", r.Error())
	}
	res, _ := r.Value.(UpgradeResult)
	if !res.Updated {
		t.Fatalf("Updated = false; want true")
	}
	if len(res.Restarted) != 1 {
		t.Fatalf("Restarted = %+v; want exactly 1 respawned sandbox", res.Restarted)
	}

	// Original sandbox record is Stopped; a NEW id was spawned.
	origInspect := svc.Inspect(originalID)
	if !origInspect.OK {
		t.Fatalf("Inspect(original) failed: %s", origInspect.Error())
	}
	origSb, _ := origInspect.Value.(Sandbox)
	if origSb.Status != StatusStopped {
		t.Errorf("original sandbox status = %q; want %q", origSb.Status, StatusStopped)
	}
	if res.Restarted[0] == originalID {
		t.Errorf("restarted id == original id; want a freshly spawned sandbox")
	}
}

// dockerPullBody returns the heredoc body used inside a `case "$1"`
// arm to answer a `pull` subcommand with stdout, reused by
// TestUpgradeWithConsent_RestartSweep_Good's combined pull+run+rm
// fake runtime.
func dockerPullBody(stdout string) string {
	return `cat <<'EOF'
` + stdout + `
EOF`
}
