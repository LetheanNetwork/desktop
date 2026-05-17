// SPDX-Licence-Identifier: EUPL-1.2

// errorcode_test.go — bounded-keyspace promise expressed as test code
// per plans/code/lthn/desktop/audit/RFC.error-code-cascade.md §1.2.
// The Ugly tests ARE the security promise — they pin the contract
// that caller-controlled prose can NEVER bleed through ErrorCode into
// audit Event Meta.
//
// Wave 1 done-criteria:
//
//   - TestErrorCode_Good covers the four happy-shape rows (OK, Code,
//     Operation, fallback-on-unknown).
//   - TestErrorCode_Bad covers the two guard rows (bare error,
//     empty-operation *core.Err).
//   - TestErrorCode_Ugly covers the four prose-leak vectors:
//     NoProseLeak via bare error, NoProseLeak via *core.Err message,
//     HTTPBodyJSON wrapping (Cerberus #62 FOLD), 4KB stress with
//     bounded-output assertion (Cerberus #62 FOLD), depth-2 cause
//     chain (Cerberus #62 FOLD).

package audit

import (
	"testing"

	core "dappco.re/go"
)

// TestErrorCode_Good — the four happy-shape rows from RFC §1.2.
// Validates the resolution order from the canonical happy paths.
func TestErrorCode_Good(t *testing.T) {
	// Row 1: r.OK == true → "".
	if got := ErrorCode(core.Ok(nil)); got != "" {
		t.Errorf("ErrorCode(OK) = %q, want \"\"", got)
	}

	// Row 1b: r.OK with non-nil value still returns "".
	if got := ErrorCode(core.Ok("anything")); got != "" {
		t.Errorf("ErrorCode(Ok(anything)) = %q, want \"\"", got)
	}

	// Row 2: r.Code() populated → canonical codespace win.
	codedErr := &core.Err{
		Operation: "ai.openai.complete",
		Message:   "should not leak",
		Code:      "ai.openai.provider",
	}
	if got := ErrorCode(core.Fail(codedErr)); got != "ai.openai.provider" {
		t.Errorf("ErrorCode(Fail(coded)) = %q, want \"ai.openai.provider\"", got)
	}

	// Row 3: Operation fallback when Code is empty.
	opErr := core.E("marketplace.install", "spawn failed", nil)
	if got := ErrorCode(core.Fail(opErr)); got != "marketplace.install" {
		t.Errorf("ErrorCode(Fail(E(op))) = %q, want \"marketplace.install\"", got)
	}
}

// TestErrorCode_Bad — guard rows that exercise the canonical fallback
// for "neither Code nor Operation" shapes. Confirms FOLD-1 — the
// substrate owns the literal "unknown_error" cluster-wide.
func TestErrorCode_Bad(t *testing.T) {
	// Bare error (not a *core.Err) → "unknown_error".
	bare := core.NewError("plain old error")
	if got := ErrorCode(core.Fail(bare)); got != "unknown_error" {
		t.Errorf("ErrorCode(Fail(bare)) = %q, want \"unknown_error\"", got)
	}

	// *core.Err with empty Operation AND empty Code → "unknown_error".
	emptyOp := core.E("", "message with no op", nil)
	if got := ErrorCode(core.Fail(emptyOp)); got != "unknown_error" {
		t.Errorf("ErrorCode(Fail(E(\"\"))) = %q, want \"unknown_error\"", got)
	}

	// Non-error r.Value (Result.Value is a string, not an error) →
	// "unknown_error" because the type-assertion to error fails and
	// the Operation extraction branch is skipped.
	weirdShape := core.Result{Value: "some string value", OK: false}
	if got := ErrorCode(weirdShape); got != "unknown_error" {
		t.Errorf("ErrorCode(non-error Value) = %q, want \"unknown_error\"", got)
	}
}

// TestErrorCode_Ugly — the bounded-keyspace SECURITY PROMISE. Every
// row in this function pins the contract that caller-controlled prose
// CANNOT leak through ErrorCode into the audit substrate. These are
// the rows that justify the substrate's existence per RFC §0 PROMOTE-
// AT-N=6 override of the DEFER-TO-N=7 cluster-canon heuristic.
func TestErrorCode_Ugly(t *testing.T) {
	// Ugly 1 — NoProseLeak via bare error. The classic STRIDE-I leak
	// shape: provider echoes user prompt into error body, caller wraps
	// with errors.New, audit substrate receives prose.
	t.Run("NoProseLeak", func(t *testing.T) {
		leakage := "user-prompt-12345-LEAKED"
		got := ErrorCode(core.Fail(core.NewError(leakage)))
		if core.Contains(got, "user-prompt-12345") {
			t.Errorf("ErrorCode leaked caller-controlled prose: got %q (contains %q)", got, leakage)
		}
		if got != "unknown_error" {
			t.Errorf("ErrorCode(Fail(bare-leak)) = %q, want \"unknown_error\"", got)
		}
	})

	// Ugly 2 — NoProseLeak via *core.Err message field. The Message
	// field of a *core.Err never escapes — only the Operation does.
	t.Run("NoProseLeakViaCoreErrMessage", func(t *testing.T) {
		leakage := "user-prompt-12345-LEAKED"
		errWithLeak := core.E("scope.bounded", leakage, nil)
		got := ErrorCode(core.Fail(errWithLeak))
		if core.Contains(got, "user-prompt-12345") {
			t.Errorf("ErrorCode leaked *core.Err.Message: got %q (contains %q)", got, leakage)
		}
		if got != "scope.bounded" {
			t.Errorf("ErrorCode(Fail(E(op, leak))) = %q, want \"scope.bounded\"", got)
		}
	})

	// Ugly 3 — HTTPBodyJSON wrapping (Cerberus #62 FOLD). Realistic
	// leak shape: provider returns a JSON error body, caller wraps the
	// body in a bare errors.New, audit substrate receives upstream-
	// secret-xyz.
	t.Run("HTTPBodyJSON", func(t *testing.T) {
		body := `{"error":{"message":"upstream-secret-xyz"}}`
		got := ErrorCode(core.Fail(core.NewError(body)))
		if core.Contains(got, "upstream-secret-xyz") {
			t.Errorf("ErrorCode leaked HTTP body: got %q (contains \"upstream-secret-xyz\")", got)
		}
		if got != "unknown_error" {
			t.Errorf("ErrorCode(Fail(HTTPBody)) = %q, want \"unknown_error\"", got)
		}
	})

	// Ugly 4 — 4KB stress (Cerberus #62 FOLD). Verify bounded output
	// regardless of input size — output MUST stay ≤ 64 chars and MUST
	// NOT contain the tail marker buried 4KB deep.
	t.Run("FourKBStress", func(t *testing.T) {
		// Build 4KB of 'A' + a sentinel tail marker without using
		// strings.Repeat (banned per AX-6). core.Concat is safe.
		buf := make([]byte, 4096)
		for i := range buf {
			buf[i] = 'A'
		}
		stress := string(buf) + "TAIL-MARKER"
		got := ErrorCode(core.Fail(core.NewError(stress)))
		if len(got) > 64 {
			t.Errorf("ErrorCode(4KB) returned %d chars, want <= 64", len(got))
		}
		if core.Contains(got, "TAIL-MARKER") {
			t.Errorf("ErrorCode leaked 4KB tail marker: got %q", got)
		}
		if got != "unknown_error" {
			t.Errorf("ErrorCode(Fail(4KB)) = %q, want \"unknown_error\"", got)
		}
	})

	// Ugly 5 — depth-2 cause chain (Cerberus #62 FOLD). Nested
	// Error() formatting concatenates the inner Message + Cause prose;
	// verify the outer Operation wins extraction and no inner prose
	// bleeds through.
	t.Run("DepthTwoCauseChain", func(t *testing.T) {
		base := core.NewError("base-SECRET")
		inner := core.E("scope.inner", "inner-SECRET", base)
		outer := core.E("scope.outer", "outer", inner)
		got := ErrorCode(core.Fail(outer))
		if core.Contains(got, "inner-SECRET") {
			t.Errorf("ErrorCode leaked inner cause Message: got %q", got)
		}
		if core.Contains(got, "base-SECRET") {
			t.Errorf("ErrorCode leaked base cause prose: got %q", got)
		}
		if got != "scope.outer" {
			t.Errorf("ErrorCode(Fail(depth-2)) = %q, want \"scope.outer\"", got)
		}
	})
}
