// SPDX-Licence-Identifier: EUPL-1.2

// Behavioural tests for the Wails3 Service wrapper — drives the real
// WailsService methods (not just reflection over their signatures).
// wails_example_test.go already covers the %T-shape checks; this file
// exercises actual behaviour: lifecycle no-ops + the Endpoint success
// and error branches.

package validator_test

import (
	"net/http/httptest"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/validator"
)

func TestWails_WailsService_ServiceName_GoodReturnsName(t *core.T) {
	s := validator.NewWailsService()
	core.AssertEqual(t, "Validator", s.ServiceName())
}

func TestWails_WailsService_ServiceStartup_GoodNoop(t *core.T) {
	s := validator.NewWailsService()
	r := s.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)
}

func TestWails_WailsService_ServiceShutdown_GoodNoop(t *core.T) {
	s := validator.NewWailsService()
	r := s.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestWails_WailsService_Endpoint_Good_2xx(t *core.T) {
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(core.StatusOK)
	}))
	defer srv.Close()

	s := validator.NewWailsService()
	r := s.Endpoint(srv.URL)
	core.AssertTrue(t, r.OK)
	info := r.Value.(validator.EndpointInfo)
	core.AssertTrue(t, info.OK)
}

func TestWails_WailsService_Endpoint_Bad_EmptyURL(t *core.T) {
	s := validator.NewWailsService()
	r := s.Endpoint("")
	core.AssertFalse(t, r.OK, "empty URL must fail through the wrapper")
	core.AssertNotEmpty(t, r.Error())
}
