// SPDX-License-Identifier: EUPL-1.2

package desktopstate

import core "dappco.re/go"

// WailsService is the renderer-safe typed desktop-state surface.
type WailsService struct {
	service *Service
}

// NewWailsService wraps the Core-owned desktop-state service.
func NewWailsService(service *Service) *WailsService {
	return &WailsService{service: service}
}

// LoadShellSession delegates to the Medium-backed service.
func (service *WailsService) LoadShellSession() core.Result {
	if service == nil || service.service == nil {
		return unavailableService("LoadShellSession")
	}
	return service.service.LoadShellSession()
}

// SaveShellSession delegates to the Medium-backed service.
func (service *WailsService) SaveShellSession(
	input SaveShellSessionInput,
) core.Result {
	if service == nil || service.service == nil {
		return unavailableService("SaveShellSession")
	}
	return service.service.SaveShellSession(input)
}

// LoadTerminalWorkspace delegates to the Medium-backed service.
func (service *WailsService) LoadTerminalWorkspace() core.Result {
	if service == nil || service.service == nil {
		return unavailableService("LoadTerminalWorkspace")
	}
	return service.service.LoadTerminalWorkspace()
}

// SaveTerminalWorkspace delegates to the Medium-backed service.
func (service *WailsService) SaveTerminalWorkspace(
	input SaveTerminalWorkspaceInput,
) core.Result {
	if service == nil || service.service == nil {
		return unavailableService("SaveTerminalWorkspace")
	}
	return service.service.SaveTerminalWorkspace(input)
}
