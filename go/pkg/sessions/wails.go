// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the sessions package — wraps the free
// Create / Append / Read / List functions so Wails generates a TS
// binding at frontend/bindings/dappco.re/lthn/desktop/pkg/sessions/.
// Bound by application.NewService(sessions.NewWailsService(core)) in
// pkg/desktop/desktop.go; the package-level functions stay for
// non-WebView callers (CLI verbs, Action bus consumers).

package sessions

import (
	"context"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WailsService is the WebView-facing handle on the sessions store.
// Carries the *core.Core reference so methods can dispatch through
// the same store / stream / process services the package functions
// use without re-resolving the Core per call.
type WailsService struct {
	core *core.Core
}

// NewWailsService binds the WebView surface to c. c must be the
// same Core the sessions package was registered against.
//
// Usage example:
//
//	application.NewService(sessions.NewWailsService(coreInstance))
func NewWailsService(c *core.Core) *WailsService { return &WailsService{core: c} }

func (s *WailsService) ServiceName() string { return "Sessions" }
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WailsService) ServiceShutdown() error { return nil }

// Create starts a new session with the given title and returns its
// id. The id is a content-derived string suitable for filesystem
// paths.
func (s *WailsService) Create(title string) (string, error) {
	r := Create(s.core, title)
	if !r.OK {
		return "", core.E("sessions.WailsService.Create", "create session failed", r.Value.(error))
	}
	id, _ := r.Value.(string)
	return id, nil
}

// Append adds a single message to an existing session's history.
func (s *WailsService) Append(id, role, content string) error {
	r := Append(s.core, id, role, content)
	if !r.OK {
		return core.E("sessions.WailsService.Append", "append session message failed", r.Value.(error))
	}
	return nil
}

// Read returns every message in the session in chronological order.
func (s *WailsService) Read(id string) ([]inference.Message, error) {
	r := Read(s.core, id)
	if !r.OK {
		return nil, core.E("sessions.WailsService.Read", "read session failed", r.Value.(error))
	}
	msgs, _ := r.Value.([]inference.Message)
	return msgs, nil
}

// List returns the session catalogue — one entry per stored
// session, with id / title / timestamps / message count.
func (s *WailsService) List() ([]SessionInfo, error) {
	r := List(s.core)
	if !r.OK {
		return nil, core.E("sessions.WailsService.List", "list sessions failed", r.Value.(error))
	}
	infos, _ := r.Value.([]SessionInfo)
	return infos, nil
}
