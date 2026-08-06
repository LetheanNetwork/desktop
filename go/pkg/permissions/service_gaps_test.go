// SPDX-License-Identifier: EUPL-1.2

// Coverage-gap tests for service.go. service_test.go already proves
// Status/Request through a fake Host with a real config service; this
// file drives the branches that path never reaches: nil-receiver
// guards, the package-level Register() constructor function, the
// host-is-nil default-wiring branch, a Host that errors or returns an
// invalid HostState, policy()'s own guard/default branches, and the
// real corePermissionHost — the production Host implementation that
// every other test in this package replaces with a fake.

package permissions

import (
	"errors"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"
	guinotification "dappco.re/go/render/display/webkit/pkg/notification"
)

// --- package-level Register() ---------------------------------------------

func TestService_Register_GoodConstructsAndAttaches(t *testing.T) {
	c := core.New()
	result := Register(c)
	if !result.OK {
		t.Fatalf("Register: want OK, got %v", result.Error())
	}
	if _, ok := result.Value.(*Service); !ok {
		t.Fatalf("Register: want *Service value, got %T", result.Value)
	}
}

// --- Service.Register --------------------------------------------------

// TestService_Register_Bad_PropagatesRegisterFailure drives the
// package-level Register()'s own "!result.OK" propagation branch:
// NewService(Options{Core: nil}) constructs fine (a nil Core is
// legal there), but the subsequent service.Register(nil) call fails
// its own nil-core guard, and Register() must surface that failure
// rather than swallowing it.
func TestService_Register_Bad_PropagatesRegisterFailure(t *testing.T) {
	result := Register(nil)
	if result.OK {
		t.Fatal("Register(nil): want the inner Register failure propagated, got OK")
	}
}

func TestService_Register_Bad_NilService(t *testing.T) {
	var s *Service
	result := s.Register(core.New())
	if result.OK {
		t.Fatal("Register on nil service: want Fail, got OK")
	}
}

func TestService_Register_Bad_NilCore(t *testing.T) {
	s := &Service{}
	result := s.Register(nil)
	if result.OK {
		t.Fatal("Register with nil core: want Fail, got OK")
	}
}

// TestService_Register_Good_DefaultsHostWhenNil constructs a bare
// Service{} directly (unexported field, host left nil — NewService
// never does this) so Register's "s.host == nil" branch actually
// fires and wires the real corePermissionHost default.
func TestService_Register_Good_DefaultsHostWhenNil(t *testing.T) {
	s := &Service{}
	c := core.New()
	result := s.Register(c)
	if !result.OK {
		t.Fatalf("Register: want OK, got %v", result.Error())
	}
	if _, ok := s.host.(corePermissionHost); !ok {
		t.Fatalf("Register: want host defaulted to corePermissionHost, got %T", s.host)
	}
}

// --- Service.Status / Service.Request nil-receiver guards ------------------

func TestService_Status_Bad_NilService(t *testing.T) {
	var s *Service
	if s.Status().OK {
		t.Fatal("Status on nil service: want Fail, got OK")
	}
}

func TestService_Request_Bad_NilService(t *testing.T) {
	var s *Service
	if s.Request(string(Notifications)).OK {
		t.Fatal("Request on nil service: want Fail, got OK")
	}
}

// --- Host error / invalid-state branches ------------------------------

// erroringHost fails every Status/Request call — drives Service.Status
// and Service.Request's "host returned an error" branches, which the
// happy-path permissionHostProbe in service_test.go never triggers.
type erroringHost struct{}

func (erroringHost) Status(ID) (HostState, error)  { return HostUnknown, errors.New("status boom") }
func (erroringHost) Request(ID) (HostState, error) { return HostUnknown, errors.New("request boom") }

// invalidStateHost returns a HostState that isn't one of the six known
// constants — drives the validHostState(...)==false fallback branches.
type invalidStateHost struct{}

func (invalidStateHost) Status(ID) (HostState, error) { return HostState("made-up"), nil }
func (invalidStateHost) Request(ID) (HostState, error) {
	return HostState("made-up"), nil
}

func TestService_Status_Bad_HostErrorFallsBackToUnknown(t *testing.T) {
	s := &Service{host: erroringHost{}}
	result := s.Status()
	if !result.OK {
		t.Fatalf("Status: want OK envelope even with host errors, got %v", result.Error())
	}
	snapshots, ok := result.Value.([]Snapshot)
	if !ok {
		t.Fatalf("Status: want []Snapshot, got %T", result.Value)
	}
	for _, snap := range snapshots {
		if snap.Host != HostUnknown {
			t.Fatalf("snapshot %q: want HostUnknown on host error, got %q", snap.ID, snap.Host)
		}
	}
}

func TestService_Status_Ugly_HostInvalidStateFallsBackToUnknown(t *testing.T) {
	s := &Service{host: invalidStateHost{}}
	result := s.Status()
	if !result.OK {
		t.Fatalf("Status: want OK, got %v", result.Error())
	}
	snapshots := result.Value.([]Snapshot)
	for _, snap := range snapshots {
		if snap.Host != HostUnknown {
			t.Fatalf("snapshot %q: want HostUnknown for an invalid host state, got %q", snap.ID, snap.Host)
		}
	}
}

func TestService_Request_Bad_HostErrorFails(t *testing.T) {
	s := &Service{host: erroringHost{}}
	result := s.Request(string(Notifications))
	if result.OK {
		t.Fatal("Request: want Fail when host errors, got OK")
	}
}

func TestService_Request_Ugly_HostInvalidStateFallsBackToUnknown(t *testing.T) {
	s := &Service{host: invalidStateHost{}}
	result := s.Request(string(Notifications))
	if !result.OK {
		t.Fatalf("Request: want OK, got %v", result.Error())
	}
	snapshot := result.Value.(Snapshot)
	if snapshot.Host != HostUnknown {
		t.Fatalf("Request: want HostUnknown for an invalid host state, got %q", snapshot.Host)
	}
}

// --- policy() ------------------------------------------------------------

func TestService_Policy_Bad_NilService(t *testing.T) {
	var s *Service
	if got := s.policy(Notifications); got != PolicyDefault {
		t.Fatalf("policy on nil service: got %q want %q", got, PolicyDefault)
	}
}

func TestService_Policy_Bad_NilCore(t *testing.T) {
	s := &Service{}
	if got := s.policy(Notifications); got != PolicyDefault {
		t.Fatalf("policy with nil core: got %q want %q", got, PolicyDefault)
	}
}

func TestService_Policy_Bad_UnknownIDHasNoConfigKey(t *testing.T) {
	s := &Service{core: core.New()}
	if got := s.policy(ID("not-a-real-permission")); got != PolicyDefault {
		t.Fatalf("policy for an id with no config key: got %q want %q", got, PolicyDefault)
	}
}

func TestService_Policy_Bad_NoConfigServiceRegistered(t *testing.T) {
	s := &Service{core: core.New()}
	if got := s.policy(Notifications); got != PolicyDefault {
		t.Fatalf("policy with no config service: got %q want %q", got, PolicyDefault)
	}
}

func TestService_Policy_Ugly_UnrecognisedConfigValueDefaults(t *testing.T) {
	c := core.New(core.WithName(
		"config",
		config.NewConfigServiceWith(config.ServiceOptions{
			Path: core.PathJoin(t.TempDir(), "lthn.yaml"),
		}),
	))
	if !c.ServiceStartup(core.Background(), nil).OK {
		t.Fatal("ServiceStartup failed")
	}
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok {
		t.Fatal("config service not found")
	}
	if !cfg.Set("desktop.permissions.notifications", "banana").OK {
		t.Fatal("cfg.Set failed")
	}
	s := &Service{core: c}
	if got := s.policy(Notifications); got != PolicyDefault {
		t.Fatalf("policy for an unrecognised config value: got %q want %q", got, PolicyDefault)
	}
}

// --- corePermissionHost.Status -----------------------------------------

func TestCorePermissionHost_Status_Good_NonNotificationIsUnsupported(t *testing.T) {
	h := corePermissionHost{core: core.New()}
	state, err := h.Status(Camera)
	if err != nil {
		t.Fatalf("Status(Camera): want nil error, got %v", err)
	}
	if state != HostUnsupported {
		t.Fatalf("Status(Camera): got %q want %q", state, HostUnsupported)
	}
}

func TestCorePermissionHost_Status_Good_NilCoreIsUnsupported(t *testing.T) {
	h := corePermissionHost{}
	state, err := h.Status(Notifications)
	if err != nil {
		t.Fatalf("Status: want nil error, got %v", err)
	}
	if state != HostUnsupported {
		t.Fatalf("Status with nil core: got %q want %q", state, HostUnsupported)
	}
}

func TestCorePermissionHost_Status_Good_NoNotificationServiceIsUnsupported(t *testing.T) {
	h := corePermissionHost{core: core.New()}
	state, err := h.Status(Notifications)
	if err != nil {
		t.Fatalf("Status: want nil error, got %v", err)
	}
	if state != HostUnsupported {
		t.Fatalf("Status with no notification service: got %q want %q", state, HostUnsupported)
	}
}

func TestCorePermissionHost_Status_Good_QueryGrantedTrue(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.RegisterQuery(func(_ *core.Core, q core.Query) core.Result {
		if _, ok := q.(guinotification.QueryPermission); !ok {
			return core.Result{}
		}
		return core.Result{Value: guinotification.PermissionStatus{Granted: true}, OK: true}
	})
	h := corePermissionHost{core: c}

	state, err := h.Status(Notifications)
	if err != nil {
		t.Fatalf("Status: want nil error, got %v", err)
	}
	if state != HostGranted {
		t.Fatalf("Status with Granted=true: got %q want %q", state, HostGranted)
	}
}

func TestCorePermissionHost_Status_Good_QueryGrantedFalse(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.RegisterQuery(func(_ *core.Core, q core.Query) core.Result {
		if _, ok := q.(guinotification.QueryPermission); !ok {
			return core.Result{}
		}
		return core.Result{Value: guinotification.PermissionStatus{Granted: false}, OK: true}
	})
	h := corePermissionHost{core: c}

	state, err := h.Status(Notifications)
	if err != nil {
		t.Fatalf("Status: want nil error, got %v", err)
	}
	if state != HostUnknown {
		t.Fatalf("Status with Granted=false: got %q want %q (fail closed, not denied)", state, HostUnknown)
	}
}

func TestCorePermissionHost_Status_Bad_QueryFails(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.RegisterQuery(func(_ *core.Core, q core.Query) core.Result {
		if _, ok := q.(guinotification.QueryPermission); !ok {
			return core.Result{}
		}
		return core.Result{Value: errors.New("query boom"), OK: false}
	})
	h := corePermissionHost{core: c}

	state, err := h.Status(Notifications)
	if err == nil {
		t.Fatal("Status: want error when the query fails, got nil")
	}
	if state != HostUnknown {
		t.Fatalf("Status on query failure: got %q want %q", state, HostUnknown)
	}
}

func TestCorePermissionHost_Status_Bad_QueryWrongValueType(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.RegisterQuery(func(_ *core.Core, q core.Query) core.Result {
		if _, ok := q.(guinotification.QueryPermission); !ok {
			return core.Result{}
		}
		return core.Result{Value: "not a PermissionStatus", OK: true}
	})
	h := corePermissionHost{core: c}

	state, err := h.Status(Notifications)
	if err == nil {
		t.Fatal("Status: want error on unexpected query value type, got nil")
	}
	if state != HostUnknown {
		t.Fatalf("Status on unexpected value type: got %q want %q", state, HostUnknown)
	}
}

// --- corePermissionHost.Request -----------------------------------------

func TestCorePermissionHost_Request_Good_NonNotificationIsUnsupported(t *testing.T) {
	h := corePermissionHost{core: core.New()}
	state, err := h.Request(Camera)
	if err != nil {
		t.Fatalf("Request(Camera): want nil error, got %v", err)
	}
	if state != HostUnsupported {
		t.Fatalf("Request(Camera): got %q want %q", state, HostUnsupported)
	}
}

func TestCorePermissionHost_Request_Good_NilCoreIsUnsupported(t *testing.T) {
	h := corePermissionHost{}
	state, err := h.Request(Notifications)
	if err != nil {
		t.Fatalf("Request: want nil error, got %v", err)
	}
	if state != HostUnsupported {
		t.Fatalf("Request with nil core: got %q want %q", state, HostUnsupported)
	}
}

func TestCorePermissionHost_Request_Good_NoNotificationServiceIsUnsupported(t *testing.T) {
	h := corePermissionHost{core: core.New()}
	state, err := h.Request(Notifications)
	if err != nil {
		t.Fatalf("Request: want nil error, got %v", err)
	}
	if state != HostUnsupported {
		t.Fatalf("Request with no notification service: got %q want %q", state, HostUnsupported)
	}
}

func TestCorePermissionHost_Request_Good_ActionGrantedTrue(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.Action("notification.request_permission", func(_ core.Context, _ core.Options) core.Result {
		return core.Result{Value: true, OK: true}
	})
	h := corePermissionHost{core: c}

	state, err := h.Request(Notifications)
	if err != nil {
		t.Fatalf("Request: want nil error, got %v", err)
	}
	if state != HostGranted {
		t.Fatalf("Request granted=true: got %q want %q", state, HostGranted)
	}
}

func TestCorePermissionHost_Request_Good_ActionGrantedFalse(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.Action("notification.request_permission", func(_ core.Context, _ core.Options) core.Result {
		return core.Result{Value: false, OK: true}
	})
	h := corePermissionHost{core: c}

	state, err := h.Request(Notifications)
	if err != nil {
		t.Fatalf("Request: want nil error, got %v", err)
	}
	if state != HostDenied {
		t.Fatalf("Request granted=false: got %q want %q", state, HostDenied)
	}
}

func TestCorePermissionHost_Request_Bad_ActionFails(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.Action("notification.request_permission", func(_ core.Context, _ core.Options) core.Result {
		return core.Result{Value: errors.New("request boom"), OK: false}
	})
	h := corePermissionHost{core: c}

	state, err := h.Request(Notifications)
	if err == nil {
		t.Fatal("Request: want error when the action fails, got nil")
	}
	if state != HostUnknown {
		t.Fatalf("Request on action failure: got %q want %q", state, HostUnknown)
	}
}

func TestCorePermissionHost_Request_Bad_ActionWrongValueType(t *testing.T) {
	c := core.New()
	c.RegisterService("notification", struct{}{})
	c.Action("notification.request_permission", func(_ core.Context, _ core.Options) core.Result {
		return core.Result{Value: "not a bool", OK: true}
	})
	h := corePermissionHost{core: c}

	state, err := h.Request(Notifications)
	if err == nil {
		t.Fatal("Request: want error on unexpected action value type, got nil")
	}
	if state != HostUnknown {
		t.Fatalf("Request on unexpected value type: got %q want %q", state, HostUnknown)
	}
}
