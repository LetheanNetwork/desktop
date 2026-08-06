// SPDX-Licence-Identifier: EUPL-1.2

// auth_test.go — coverage for the OPENCODE_SERVER_PASSWORD +
// install_id lifecycle (auth.go). Hermetic via resetKV (temp $HOME +
// rewound kv() singleton); the Bad-path tests force kv() itself to
// fail via breakKV (points $HOME at "" — exactly how
// core.UserHomeDir() fails on unix, no filesystem fixture needed).

package opencode

import (
	"testing"

	core "dappco.re/go"
)

// TestServerPassword_IdempotentAcrossCalls_Good — the first call
// generates + persists a password; subsequent calls (including a
// fresh Service value against the same kv() home) return the SAME
// password.
func TestServerPassword_IdempotentAcrossCalls_Good(t *testing.T) {
	resetKV(t)
	svc1 := &Service{}
	r1 := svc1.ServerPassword()
	if !r1.OK {
		t.Fatalf("ServerPassword failed: %s", r1.Error())
	}
	pw1, _ := r1.Value.(string)
	if pw1 == "" {
		t.Fatalf("ServerPassword returned empty string")
	}
	if len(pw1) != 48 { // 24 bytes hex-encoded
		t.Errorf("ServerPassword length = %d; want 48 (24-byte hex)", len(pw1))
	}

	svc2 := &Service{}
	r2 := svc2.ServerPassword()
	if !r2.OK {
		t.Fatalf("second ServerPassword failed: %s", r2.Error())
	}
	if r2.Value != pw1 {
		t.Errorf("ServerPassword not idempotent: got %v, want %v", r2.Value, pw1)
	}
}

// TestServerPassword_KVUnavailable_Bad — when kv() itself cannot
// resolve $HOME, ServerPassword must Fail rather than panic.
func TestServerPassword_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)

	svc := &Service{}
	r := svc.ServerPassword()
	if r.OK {
		t.Fatalf("ServerPassword with $HOME unset returned OK; want Fail")
	}
}

// TestAuthHeader_ComposesBasicAuth_Good — authHeader is "Basic
// base64(opencode:<password>)".
func TestAuthHeader_ComposesBasicAuth_Good(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	h := svc.authHeader()
	if h == "" {
		t.Fatalf("authHeader() returned empty string")
	}
	if h[:6] != "Basic " {
		t.Errorf("authHeader() = %q; want to start with 'Basic '", h)
	}
}

// TestAuthHeader_KVUnavailable_Bad — authHeader degrades to "" rather
// than propagating an error — callers treat empty as "skip injection".
func TestAuthHeader_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)

	svc := &Service{}
	if h := svc.authHeader(); h != "" {
		t.Errorf("authHeader() with unavailable kv = %q; want empty", h)
	}
}

// TestApplyAuth_SetsHeaderWhenPresent_Good / SkipsWhenEmpty_Bad —
// applyAuth only mutates the request when authHeader() is non-empty.
func TestApplyAuth_SetsHeaderWhenPresent_Good(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	rr := core.NewHTTPRequest(core.MethodGet, "http://example.invalid/", nil)
	if !rr.OK {
		t.Fatalf("NewHTTPRequest failed: %s", rr.Error())
	}
	req := rr.Value.(*core.Request)
	svc.applyAuth(req)
	if got := req.Header.Get("Authorization"); got == "" {
		t.Errorf("applyAuth did not set Authorization header")
	}
}

func TestApplyAuth_SkipsWhenEmpty_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)

	svc := &Service{}
	rr := core.NewHTTPRequest(core.MethodGet, "http://example.invalid/", nil)
	if !rr.OK {
		t.Fatalf("NewHTTPRequest failed: %s", rr.Error())
	}
	req := rr.Value.(*core.Request)
	svc.applyAuth(req)
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("applyAuth set Authorization = %q despite kv() unavailable; want unset", got)
	}
}

// TestInstallID_IdempotentAcrossCalls_Good mirrors ServerPassword's
// idempotency contract — the docker adoption-gate label depends on
// this being stable across process restarts (same $HOME).
func TestInstallID_IdempotentAcrossCalls_Good(t *testing.T) {
	resetKV(t)
	svc := &Service{}
	r1 := svc.InstallID()
	if !r1.OK {
		t.Fatalf("InstallID failed: %s", r1.Error())
	}
	id1, _ := r1.Value.(string)
	if len(id1) != 32 { // 16 bytes hex-encoded
		t.Errorf("InstallID length = %d; want 32 (16-byte hex)", len(id1))
	}
	r2 := svc.InstallID()
	if r2.Value != id1 {
		t.Errorf("InstallID not idempotent: got %v, want %v", r2.Value, id1)
	}
}

// TestInstallID_KVUnavailable_Bad mirrors ServerPassword's failure
// shape.
func TestInstallID_KVUnavailable_Bad(t *testing.T) {
	resetKV(t)
	breakKV(t)

	svc := &Service{}
	r := svc.InstallID()
	if r.OK {
		t.Fatalf("InstallID with $HOME unset returned OK; want Fail")
	}
}
