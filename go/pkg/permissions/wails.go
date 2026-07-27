// SPDX-License-Identifier: EUPL-1.2

package permissions

import (
	core "dappco.re/go"
	gui "dappco.re/go/render/display/webkit"
)

// HostIntent is the renderer-facing envelope used by the desktop's single
// native event boundary.
type HostIntent struct {
	Kind       string    `json:"kind"`
	Permission *Snapshot `json:"permission"`
}

// WailsService exposes only the bounded status and explicit request methods to
// the renderer. Registration and host wiring remain trusted Go concerns.
type WailsService struct {
	service *Service
}

// NewWailsService wraps the registered permission service.
//
// Usage example:
//
//	binding := permissions.NewWailsService(service)
func NewWailsService(service *Service) *WailsService {
	return &WailsService{service: service}
}

// Status returns every known policy/host snapshot without prompting.
func (s *WailsService) Status() core.Result {
	if s == nil || s.service == nil {
		return core.Fail(core.E(
			"permissions.WailsService.Status",
			"permission service is unavailable",
			nil,
		))
	}
	result := s.service.Status()
	if !result.OK {
		return result
	}
	snapshots, ok := result.Value.([]Snapshot)
	if !ok {
		return core.Fail(core.E(
			"permissions.WailsService.Status",
			"unexpected permission snapshots",
			nil,
		))
	}
	for index := range snapshots {
		s.emit(snapshots[index])
	}
	return result
}

// Request forwards one explicit, allowlisted permission request.
func (s *WailsService) Request(id string) core.Result {
	if s == nil || s.service == nil {
		return core.Fail(core.E(
			"permissions.WailsService.Request",
			"permission service is unavailable",
			nil,
		))
	}
	result := s.service.Request(id)
	if !result.OK {
		return result
	}
	snapshot, ok := result.Value.(Snapshot)
	if !ok {
		return core.Fail(core.E(
			"permissions.WailsService.Request",
			"unexpected permission snapshot",
			nil,
		))
	}
	s.emit(snapshot)
	return result
}

func (s *WailsService) emit(snapshot Snapshot) {
	if s == nil || s.service == nil || s.service.core == nil {
		return
	}
	_ = gui.EmitEvent(s.service.core, "lthn:host:intent", HostIntent{
		Kind:       "permission",
		Permission: &snapshot,
	})
}
