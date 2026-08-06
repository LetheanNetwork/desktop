// SPDX-Licence-Identifier: EUPL-1.2

// Real tests for wails.go's WailsService (Reveal / Masked / WRotate /
// lifecycle). Companion to apikey_test.go's diagnosis: the pre-
// existing wails_example_test.go only reflects on method VALUES and
// never invokes them, so this file exercises the real call paths
// including the nil-receiver / nil-core / config-service-missing
// defensive branches.

package apikey_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	subject "dappco.re/lthn/desktop/pkg/apikey"
)

func TestWails_NewWailsService_Good_CapturesCore(t *core.T) {
	c := apikeyFixture(t)
	svc := subject.NewWailsService(c)
	core.AssertNotNil(t, svc)
}

func TestWails_WailsService_ServiceLifecycle_Good(t *core.T) {
	svc := subject.NewWailsService(nil)
	core.AssertEqual(t, "ApiKey", svc.ServiceName())

	r := svc.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK)

	r = svc.ServiceShutdown()
	core.AssertTrue(t, r.OK)
}

func TestWails_WailsService_Reveal_Good_ReturnsFullKey(t *core.T) {
	c := apikeyFixture(t)
	svc := subject.NewWailsService(c)

	want := subject.GenerateOrLoad(c)
	core.RequireTrue(t, want.OK, want.Error())

	r := svc.Reveal()
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, want.Value.(string), r.Value.(string))
}

func TestWails_WailsService_Reveal_Bad_NilReceiver(t *core.T) {
	var svc *subject.WailsService
	r := svc.Reveal()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "", r.Value.(string))
}

func TestWails_WailsService_Reveal_Ugly_NilCoreDegradesToEmptyOK(t *core.T) {
	svc := subject.NewWailsService(nil)
	r := svc.Reveal()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "", r.Value.(string))
}

func TestWails_WailsService_Reveal_Ugly_ConfigServiceMissingDegradesToEmptyOK(t *core.T) {
	c := core.New() // no "config" service registered
	svc := subject.NewWailsService(c)
	r := svc.Reveal()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "", r.Value.(string))
}

func TestWails_WailsService_Masked_Good_ReturnsMaskedForm(t *core.T) {
	c := apikeyFixture(t)
	svc := subject.NewWailsService(c)

	full := subject.GenerateOrLoad(c)
	core.RequireTrue(t, full.OK, full.Error())

	r := svc.Masked()
	core.RequireTrue(t, r.OK)
	masked := r.Value.(string)
	core.AssertEqual(t, subject.Mask(full.Value.(string)), masked)
	core.AssertNotEqual(t, full.Value.(string), masked)
}

func TestWails_WailsService_Masked_Bad_NilReceiver(t *core.T) {
	var svc *subject.WailsService
	r := svc.Masked()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "", r.Value.(string))
}

func TestWails_WailsService_Masked_Ugly_ConfigServiceMissingDegradesToEmptyOK(t *core.T) {
	c := core.New()
	svc := subject.NewWailsService(c)
	r := svc.Masked()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "", r.Value.(string))
}

func TestWails_WailsService_WRotate_Good_ReturnsFreshKey(t *core.T) {
	c := apikeyFixture(t)
	svc := subject.NewWailsService(c)

	before := subject.GenerateOrLoad(c)
	core.RequireTrue(t, before.OK, before.Error())

	r := svc.WRotate()
	core.RequireTrue(t, r.OK)
	rotated := r.Value.(string)
	core.AssertNotEqual(t, before.Value.(string), rotated)

	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	var persisted string
	core.RequireTrue(t, cfg.Get(subject.ConfigKey, &persisted).OK)
	core.AssertEqual(t, rotated, persisted)
}

func TestWails_WailsService_WRotate_Bad_NilReceiver(t *core.T) {
	var svc *subject.WailsService
	r := svc.WRotate()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "", r.Value.(string))
}

func TestWails_WailsService_WRotate_Ugly_ConfigServiceMissingDegradesToEmptyOK(t *core.T) {
	c := core.New()
	svc := subject.NewWailsService(c)
	r := svc.WRotate()
	core.AssertTrue(t, r.OK)
	core.AssertEqual(t, "", r.Value.(string))
}
