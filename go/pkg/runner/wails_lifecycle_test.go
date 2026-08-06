// SPDX-Licence-Identifier: EUPL-1.2

// wails_lifecycle_test.go — real invocations of the Wails3 lifecycle
// hooks on *runner.Service. wails_example_test.go's TestWails_Service_
// ServiceName/ServiceStartup/ServiceShutdown functions only take a
// method VALUE and Sprintf its %T; they never call the method, so
// ServiceName and ServiceShutdown sat at 0% coverage (ServiceStartup
// is separately exercised via credentialFixture in credentials_test.go).

package runner_test

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/runner"
)

func TestRunner_ServiceName_Good(t *core.T) {
	s := runner.NewService(runner.Options{})
	core.AssertEqual(t, "Runner", s.ServiceName())
}

func TestRunner_ServiceShutdown_Good(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.ServiceShutdown()
	core.AssertTrue(t, r.OK, "ServiceShutdown is a documented no-op — must always return OK")
}

// TestRunner_ServiceStartup_Good_NilCoreSkipsWiring pins the
// documented "no-op when s.core is unset" contract for a runner
// constructed pre-wiring (runner.NewService, not NewServiceFromCore).
func TestRunner_ServiceStartup_Good_NilCoreSkipsWiring(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK, "ServiceStartup must succeed even with no Core attached")
}

// TestRunner_WCredentialMigrationStatus_Good_NilCoreReturnsEmptyStatus
// covers the pre-wiring fallback branch — credentials_test.go's
// credentialFixture only ever exercises the WITH-core path.
func TestRunner_WCredentialMigrationStatus_Good_NilCoreReturnsEmptyStatus(t *core.T) {
	s := runner.NewService(runner.Options{})
	r := s.WCredentialMigrationStatus()
	core.AssertTrue(t, r.OK, "WCredentialMigrationStatus must succeed even pre-wiring")
	st, ok := r.Value.(runner.MigrationStatus)
	core.AssertTrue(t, ok, "must return a MigrationStatus")
	core.AssertEqual(t, 0, st.PendingMigrationCount)
	core.AssertEqual(t, 0, len(st.LockedRoutes))
}
