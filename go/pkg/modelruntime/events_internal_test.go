// SPDX-Licence-Identifier: EUPL-1.2

// events_internal_test.go — Subscribe's guard clauses and fireEvent's
// nil-receiver / nil-core early returns. service_test.go only ever
// calls Subscribe with a real Core + a non-nil handler and fireEvent
// through a fully-wired fixture, so the defensive branches were dark.

package modelruntime

import core "dappco.re/go"

func TestSubscribe_NilCore_NoPanic_Bad(t *core.T) {
	Subscribe(nil, func(*core.Core, Event) {})
}

func TestSubscribe_NilHandler_NoPanic_Bad(t *core.T) {
	Subscribe(core.New(), nil)
}

func TestService_FireEvent_NilReceiver_Good(t *core.T) {
	var service *Service
	service.fireEvent("reason", StateReady)
}

func TestService_FireEvent_NilCore_Good(t *core.T) {
	service := NewService(Options{})
	// service.core is nil until Register runs — must not panic.
	service.fireEvent("reason", StateReady)
}
