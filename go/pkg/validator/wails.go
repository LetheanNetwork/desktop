// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the validator package — wraps
// Endpoint() for the WebView. Bound by application.NewService(
// validator.NewWailsService()) in pkg/desktop/desktop.go.

package validator

import (
	"context"

	core "dappco.re/go"
)

type WailsService struct{}

func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "Validator" }
func (s *WailsService) ServiceStartup(_ context.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// Endpoint probes <baseURL>/models and reports status. 2xx → OK.
func (s *WailsService) Endpoint(baseURL string) core.Result {
	r := Endpoint(baseURL)
	if !r.OK {
		return core.Fail(core.E("validator.WailsService.Endpoint", "validate endpoint failed", r.Value.(error)))
	}
	info, _ := r.Value.(EndpointInfo)
	return core.Ok(info)
}
