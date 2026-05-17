// SPDX-Licence-Identifier: EUPL-1.2

// wails_readdealstage_test.go — internal (`package pipeline`) regression
// for Cerberus #24 Finding 1. readDealStage now uses paths.JoinAndCheck
// to enforce traversal-rejection independently of caller discipline —
// the unit test below pins that defence so a future refactor cannot
// silently revert to the bare core.PathJoin shape that relied on every
// caller running paths.IsValidID upstream.

package pipeline

import (
	"strings"
	"testing"
)

// TestReadDealStage_TraversalRejected_Bad — direct call with a
// traversal-shaped id must return the paths.escape error from
// paths.JoinAndCheck without ever touching the filesystem. This is
// the defence-in-depth assertion: even if a future caller forgets
// the upstream paths.IsValidID gate (as MoveDeal applies today),
// readDealStage's own JoinAndCheck refuses to resolve the path.
func TestReadDealStage_TraversalRejected_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := readDealStage("../../etc")
	if err == nil {
		t.Fatalf("readDealStage(\"../../etc\") must reject, got nil error")
	}
	if !strings.Contains(err.Error(), "paths.escape") {
		t.Fatalf("readDealStage(\"../../etc\") error = %q, want substring %q",
			err.Error(), "paths.escape")
	}
}
