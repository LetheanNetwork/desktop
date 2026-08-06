// SPDX-License-Identifier: EUPL-1.2

// Coverage-gap tests for wails.go. wails_test.go already proves the
// emit-on-success path (Status) and the nil-service Request guard;
// this file adds Status's own nil-service guard, a successful Request
// that reaches its emit+return tail, and emit()'s nil-core early
// return (a Service can have a real host but no core — Register()
// only wires core on the trusted path; nothing stops a hand-built
// Service{host: ...} reaching WailsService).
//
// Left out, traced not guessed: the "unexpected snapshot(s) shape"
// branches in both Status and Request (result.Value asserted to
// []Snapshot / Snapshot after result.OK is true). WailsService.service
// is a concrete *Service, not an interface, and Service.Status/Request
// only fail via their own nil-receiver guard — which WailsService
// already short-circuits on before calling them. There is no legitimate
// call path left that makes the embedded Service return OK with the
// wrong value type; forcing it would mean faking the Service type
// itself, which the house rules for this pass (real seams, no
// production changes) rule out.

package permissions

import (
	"testing"

	core "dappco.re/go"
)

func TestWailsService_Status_Bad_NilService(t *testing.T) {
	var s *WailsService
	if s.Status().OK {
		t.Fatal("Status on nil WailsService: want Fail, got OK")
	}
}

func TestWailsService_Request_Good_KnownPermissionEmitsAndReturns(t *testing.T) {
	host := &permissionHostProbe{states: map[ID]HostState{
		Notifications: HostGranted,
	}}
	c, service := permissionServiceFixture(t, host)
	emitted := 0
	c.Action("events.emit", func(_ core.Context, _ core.Options) core.Result {
		emitted++
		return core.Ok(nil)
	})

	result := NewWailsService(service).Request(string(Notifications))

	if !result.OK {
		t.Fatalf("Request: want OK, got %v", result.Error())
	}
	if emitted != 1 {
		t.Fatalf("Request: want exactly 1 emit, got %d", emitted)
	}
	snapshot, ok := result.Value.(Snapshot)
	if !ok {
		t.Fatalf("Request: want Snapshot value, got %T", result.Value)
	}
	if snapshot.Host != HostGranted {
		t.Fatalf("Request: got host %q want %q", snapshot.Host, HostGranted)
	}
}

// TestWailsService_Emit_Good_NilServiceCoreIsNoop hand-builds a
// Service with a working host but no core — a state Register() never
// produces, but nothing in the type system prevents it, and emit()
// explicitly guards for it.
func TestWailsService_Emit_Good_NilServiceCoreIsNoop(t *testing.T) {
	host := &permissionHostProbe{states: map[ID]HostState{
		Notifications: HostGranted,
	}}
	service := &Service{host: host}

	result := NewWailsService(service).Status()

	if !result.OK {
		t.Fatalf("Status: want OK, got %v", result.Error())
	}
	snapshots, ok := result.Value.([]Snapshot)
	if !ok {
		t.Fatalf("Status: want []Snapshot, got %T", result.Value)
	}
	if len(snapshots) != len(permissionIDs) {
		t.Fatalf("Status: want %d snapshots, got %d", len(permissionIDs), len(snapshots))
	}
}
