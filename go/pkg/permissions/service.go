// SPDX-License-Identifier: EUPL-1.2

package permissions

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	guinotification "dappco.re/go/render/display/webkit/pkg/notification"
)

// ID is one renderer-visible operating-system capability.
type ID string

const (
	Microphone    ID = "microphone"
	Camera        ID = "camera"
	Geolocation   ID = "geolocation"
	Notifications ID = "notifications"
	ClipboardRead ID = "clipboard-read"
)

// Policy is the application policy applied before the host is consulted.
type Policy string

const (
	PolicyDefault Policy = "default"
	PolicyAllow   Policy = "allow"
	PolicyDeny    Policy = "deny"
)

// HostState is a state the native host can truthfully report.
type HostState string

const (
	HostGranted     HostState = "granted"
	HostDenied      HostState = "denied"
	HostPrompt      HostState = "prompt"
	HostRestricted  HostState = "restricted"
	HostUnsupported HostState = "unsupported"
	HostUnknown     HostState = "unknown"
)

var permissionIDs = [...]ID{
	Microphone,
	Camera,
	Geolocation,
	Notifications,
	ClipboardRead,
}

var permissionConfigKeys = map[ID]string{
	Microphone:    "desktop.permissions.microphone",
	Camera:        "desktop.permissions.camera",
	Geolocation:   "desktop.permissions.geolocation",
	Notifications: "desktop.permissions.notifications",
	ClipboardRead: "desktop.permissions.clipboard_read",
}

// Snapshot keeps the application policy separate from the verified host
// state. A permissive policy never turns an unknown host state into granted.
type Snapshot struct {
	ID     ID        `json:"id"`
	Policy Policy    `json:"policy"`
	Host   HostState `json:"host"`
}

// Host abstracts the small native permission surface available to the
// desktop. Status must not prompt; Request is called only from an explicit
// renderer action.
type Host interface {
	Status(ID) (HostState, error)
	Request(ID) (HostState, error)
}

// Options configures the permission service.
type Options struct {
	Core *core.Core
	Host Host
}

// Service exposes bounded permission snapshots without conflating application
// policy with operating-system authorisation.
type Service struct {
	core *core.Core
	host Host
}

// NewService constructs a permission service. A nil Host uses the registered
// CoreGUI notification service where available and reports other capabilities
// as unsupported.
//
// Usage example:
//
//	service := permissions.NewService(permissions.Options{Core: c})
func NewService(options Options) *Service {
	host := options.Host
	if host == nil {
		host = corePermissionHost{core: options.Core}
	}
	return &Service{core: options.Core, host: host}
}

// Register attaches the entitlement checker and returns the configured
// service for Core registration.
//
// Usage example:
//
//	core.WithName("permissions", permissions.Register)
func Register(c *core.Core) core.Result {
	service := NewService(Options{Core: c})
	if result := service.Register(c); !result.OK {
		return result
	}
	return core.Ok(service)
}

// Register attaches this service to a Core lifecycle without requesting any
// operating-system permission.
//
// Usage example:
//
//	result := service.Register(c)
func (s *Service) Register(c *core.Core) core.Result {
	if s == nil || c == nil {
		return core.Fail(core.E(
			"permissions.Service.Register",
			"core and service are required",
			nil,
		))
	}
	s.core = c
	if s.host == nil {
		s.host = corePermissionHost{core: c}
	}
	Install(c)
	return core.Ok(s)
}

// Status returns every known permission without prompting the operating
// system.
func (s *Service) Status() core.Result {
	if s == nil {
		return core.Fail(core.E(
			"permissions.Service.Status",
			"service is required",
			nil,
		))
	}
	snapshots := make([]Snapshot, 0, len(permissionIDs))
	for _, id := range permissionIDs {
		hostState := HostUnsupported
		if s.host != nil {
			state, err := s.host.Status(id)
			if err != nil {
				hostState = HostUnknown
			} else if validHostState(state) {
				hostState = state
			} else {
				hostState = HostUnknown
			}
		}
		snapshots = append(snapshots, Snapshot{
			ID:     id,
			Policy: s.policy(id),
			Host:   hostState,
		})
	}
	return core.Ok(snapshots)
}

// Request asks the native host for one known permission. Merely constructing,
// registering, or reading the service never calls this path.
func (s *Service) Request(rawID string) core.Result {
	if s == nil {
		return core.Fail(core.E(
			"permissions.Service.Request",
			"service is required",
			nil,
		))
	}
	id, ok := permissionID(rawID)
	if !ok {
		return core.Fail(core.E(
			"permissions.Service.Request",
			"unknown permission",
			nil,
		))
	}
	state := HostUnsupported
	if s.host != nil {
		requested, err := s.host.Request(id)
		if err != nil {
			return core.Fail(core.E(
				"permissions.Service.Request",
				"native permission request failed",
				err,
			))
		}
		if validHostState(requested) {
			state = requested
		} else {
			state = HostUnknown
		}
	}
	return core.Ok(Snapshot{
		ID:     id,
		Policy: s.policy(id),
		Host:   state,
	})
}

func (s *Service) policy(id ID) Policy {
	key := permissionConfigKeys[id]
	if s == nil || s.core == nil || key == "" {
		return PolicyDefault
	}
	cfg, ok := core.ServiceFor[*config.Service](s.core, "config")
	if !ok || cfg == nil {
		return PolicyDefault
	}
	var raw string
	if result := cfg.Get(key, &raw); !result.OK {
		return PolicyDefault
	}
	switch core.Lower(core.Trim(raw)) {
	case string(PolicyAllow):
		return PolicyAllow
	case string(PolicyDeny):
		return PolicyDeny
	default:
		return PolicyDefault
	}
}

func permissionID(raw string) (ID, bool) {
	candidate := ID(core.Lower(core.Trim(raw)))
	for _, id := range permissionIDs {
		if candidate == id {
			return id, true
		}
	}
	return "", false
}

func validHostState(state HostState) bool {
	switch state {
	case HostGranted, HostDenied, HostPrompt, HostRestricted,
		HostUnsupported, HostUnknown:
		return true
	default:
		return false
	}
}

type corePermissionHost struct {
	core *core.Core
}

func (h corePermissionHost) Status(id ID) (HostState, error) {
	if id != Notifications {
		return HostUnsupported, nil
	}
	if h.core == nil || !h.core.Service("notification").OK {
		return HostUnsupported, nil
	}
	result := h.core.QUERY(guinotification.QueryPermission{})
	if !result.OK {
		return HostUnknown, core.E(
			"permissions.corePermissionHost.Status",
			"notification status unavailable",
			result.Err(),
		)
	}
	status, ok := result.Value.(guinotification.PermissionStatus)
	if !ok {
		return HostUnknown, core.E(
			"permissions.corePermissionHost.Status",
			"unexpected notification status",
			nil,
		)
	}
	if status.Granted {
		return HostGranted, nil
	}
	// CoreGUI's current boolean query cannot distinguish an untouched prompt
	// from a denial, so fail closed to unknown instead of inventing a state.
	return HostUnknown, nil
}

func (h corePermissionHost) Request(id ID) (HostState, error) {
	if id != Notifications {
		return HostUnsupported, nil
	}
	if h.core == nil || !h.core.Service("notification").OK {
		return HostUnsupported, nil
	}
	result := h.core.Action("notification.request_permission").Run(
		core.Background(),
		core.NewOptions(),
	)
	if !result.OK {
		return HostUnknown, core.E(
			"permissions.corePermissionHost.Request",
			"notification authorisation unavailable",
			result.Err(),
		)
	}
	granted, ok := result.Value.(bool)
	if !ok {
		return HostUnknown, core.E(
			"permissions.corePermissionHost.Request",
			"unexpected notification authorisation",
			nil,
		)
	}
	if granted {
		return HostGranted, nil
	}
	return HostDenied, nil
}
