// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the models package — wraps the free
// List function so Wails generates a TS binding at
// frontend/bindings/dappco.re/lthn/desktop/pkg/models/. Bound by
// application.NewService(models.NewWailsService()) in
// pkg/desktop/desktop.go; the package-level List() stays for
// non-WebView callers.

package models

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type WailsService struct{}

func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "Models" }
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WailsService) ServiceShutdown() error { return nil }

// List scans the local model snapshot directory and returns one
// Entry per direct child.
func (s *WailsService) List() ([]Entry, error) {
	r := List()
	if !r.OK {
		return nil, errors.New(r.Error())
	}
	entries, _ := r.Value.([]Entry)
	return entries, nil
}
