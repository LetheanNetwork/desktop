// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the validator package — wraps
// Endpoint() for the WebView. Bound by application.NewService(
// validator.NewWailsService()) in pkg/desktop/desktop.go.

package validator

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type WailsService struct{}

func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "Validator" }
func (s *WailsService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	return nil
}
func (s *WailsService) ServiceShutdown() error { return nil }

// Endpoint probes <baseURL>/models and reports status. 2xx → OK.
func (s *WailsService) Endpoint(baseURL string) (EndpointInfo, error) {
	r := Endpoint(baseURL)
	if !r.OK {
		return EndpointInfo{}, errors.New(r.Error())
	}
	info, _ := r.Value.(EndpointInfo)
	return info, nil
}
