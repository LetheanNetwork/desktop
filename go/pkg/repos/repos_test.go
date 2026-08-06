// SPDX-Licence-Identifier: EUPL-1.2

// NewService / Register behavioural tests for pkg/repos's external
// test package. sources_test.go (in package repos) already covers
// RegisterSource/collectSourcePaths hermetically; this file and its
// siblings (wails_test.go, and the internal repos_internal_test.go)
// cover the rest of the surface that repos_example_test.go and
// wails_example_test.go used to fake — see wails_test.go's doc
// comment for the mechanism writeup.
package repos_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/repos"
)

func TestRepos_NewService_Good(t *core.T) {
	svc := subject.NewService(core.New())
	core.AssertNotNil(t, svc)
}

func TestRepos_NewService_Bad(t *core.T) {
	// A nil *core.Core must not panic construction.
	svc := subject.NewService(nil)
	core.AssertNotNil(t, svc)
}

func TestRepos_NewService_Ugly(t *core.T) {
	a := subject.NewService(core.New())
	b := subject.NewService(core.New())
	core.AssertTrue(t, a != b, "each call constructs a distinct instance")
}

func TestRepos_Register_Good(t *core.T) {
	r := subject.Register(core.New())
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok)
	core.AssertNotNil(t, svc)
}

func TestRepos_Register_Bad(t *core.T) {
	r := subject.Register(nil)
	core.AssertTrue(t, r.OK)
	_, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok)
}

func TestRepos_Register_Ugly(t *core.T) {
	c := core.New()
	r1 := subject.Register(c)
	r2 := subject.Register(c)
	core.AssertTrue(t, r1.OK)
	core.AssertTrue(t, r2.OK)
	core.AssertTrue(t, r1.Value.(*subject.Service) != r2.Value.(*subject.Service))
}
