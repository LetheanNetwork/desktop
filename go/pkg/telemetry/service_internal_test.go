// SPDX-Licence-Identifier: EUPL-1.2

package telemetry

import (
	"errors"
	"runtime"
	"testing"
)

// TestSample_GoodCapturesLastGCPauseAfterAGC forces at least one completed
// GC cycle before sampling so Sample's ms.NumGC > 0 branch (which reads the
// last pause off the ring buffer) actually runs, instead of always taking
// the zero-pause default.
func TestSample_GoodCapturesLastGCPauseAfterAGC(t *testing.T) {
	runtime.GC()

	r := Sample()
	if !r.OK {
		t.Fatalf("sample: %s", r.Error())
	}
	reading, ok := r.Value.(Reading)
	if !ok {
		t.Fatalf("value type = %T, want Reading", r.Value)
	}
	if reading.NumGC == 0 {
		t.Fatal("expected NumGC > 0 after runtime.GC()")
	}
	if reading.LastGCPauseMs < 0 {
		t.Fatalf("LastGCPauseMs = %v, want >= 0", reading.LastGCPauseMs)
	}
}

// TestService_CurrentHostSnapshot_BadNilHostSampler drives the "no host
// sampler wired" fail-closed guard directly — a zero-value Service (as
// happens if a caller ever bypasses NewService) and a genuinely nil
// receiver both hit the same early return.
func TestService_CurrentHostSnapshot_BadNilHostSampler(t *testing.T) {
	svc := &Service{}
	if r := svc.CurrentHostSnapshot(); r.OK {
		t.Fatal("a service with no host sampler should fail closed")
	}

	var nilService *Service
	if r := nilService.CurrentHostSnapshot(); r.OK {
		t.Fatal("a nil service should fail closed")
	}
}

// TestService_CurrentHostSnapshot_BadPropagatesSourceFailure wires a host
// sampler backed by a source that always errors, so CurrentHostSnapshot's
// error-propagation branch runs instead of only ever seeing success.
func TestService_CurrentHostSnapshot_BadPropagatesSourceFailure(t *testing.T) {
	svc := &Service{host: newHostSampler(&sequenceHostSource{
		err: errors.New("host API unavailable"),
	})}

	r := svc.CurrentHostSnapshot()
	if r.OK {
		t.Fatal("a failing host source should surface as a Fail result")
	}
}
