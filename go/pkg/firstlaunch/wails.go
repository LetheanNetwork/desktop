// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the firstlaunch package — wraps Detect
// for the WebView. Bound by application.NewService(firstlaunch.
// NewWailsService()) in pkg/desktop/desktop.go.

package firstlaunch

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type WailsService struct{}

func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "FirstLaunch" }
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WailsService) ServiceShutdown() error { return nil }

// Detect inspects on-disk state and reports whether this is a
// fresh install.
func (s *WailsService) Detect() (State, error) {
	r := Detect(nil)
	if !r.OK {
		return State{}, errors.New(r.Error())
	}
	state, _ := r.Value.(State)
	return state, nil
}
