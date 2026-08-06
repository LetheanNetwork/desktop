// SPDX-Licence-Identifier: EUPL-1.2

package build_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/build"
)

func TestBuild_NewService_Good_BindsCore(t *core.T) {
	c := core.New()
	svc := subject.NewService(c)
	core.AssertNotNil(t, svc)
	core.AssertEqual(t, "Build", svc.ServiceName())
}

func TestBuild_NewService_Bad_NilCore(t *core.T) {
	svc := subject.NewService(nil)
	core.AssertNotNil(t, svc)
}

func TestBuild_Register_Good_ReturnsOKService(t *core.T) {
	c := core.New()
	r := subject.Register(c)
	core.AssertTrue(t, r.OK)
	svc, ok := r.Value.(*subject.Service)
	core.AssertTrue(t, ok)
	core.AssertEqual(t, "Build", svc.ServiceName())
}
