// SPDX-Licence-Identifier: EUPL-1.2

// Benchmarks for appconfig's resolve/read path: ApplicationOptions and
// WebviewWindowOptions (appconfig.go) walk the entire Wails options
// struct tree via reflection (resolver.go's resolveConfigValue) asking
// the CoreGO config.Service for every leaf field; Service.Settings
// (service.go) builds the user-facing control catalogue from the same
// config.Service on every Settings-panel read. All three need a real,
// started config.Service — calling ApplicationOptions()/
// WebviewWindowOptions() with zero *config.Service arguments short-
// circuits the whole resolve walk (firstConfig returns nil), which
// would silently benchmark nothing.
//
// Run:
//
//	go test ./pkg/appconfig/... -run '^$' -bench . -benchmem -benchtime=20x

package appconfig_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/go/config"

	"dappco.re/lthn/desktop/pkg/appconfig"
)

// benchConfigFixture mirrors newConfigFixture (appconfig_test.go) for
// *testing.B — core.T and testing.B aren't interchangeable, so the
// benchmark fixture is a short, deliberate duplicate rather than a
// reused helper.
func benchConfigFixture(b *testing.B) (*core.Core, *config.Service) {
	b.Helper()
	path := core.PathJoin(b.TempDir(), "lthn.yaml")
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path: path,
		})),
	)
	r := c.ServiceStartup(core.Background(), nil)
	if !r.OK {
		b.Fatalf("ServiceStartup: %s", r.Error())
	}
	b.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	if !ok || cfg == nil {
		b.Fatal("config service not registered")
	}
	return c, cfg
}

func BenchmarkApplicationOptions_Resolved(b *testing.B) {
	_, cfg := benchConfigFixture(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = appconfig.ApplicationOptions(cfg)
	}
}

func BenchmarkWebviewWindowOptions_Main_Resolved(b *testing.B) {
	_, cfg := benchConfigFixture(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = appconfig.WebviewWindowOptions("main", "app", "Lethean", "wails://index.html", cfg)
	}
}

func BenchmarkService_Settings(b *testing.B) {
	c, _ := benchConfigFixture(b)
	svc := appconfig.NewService(appconfig.Options{Core: c})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := svc.Settings()
		if !r.OK {
			b.Fatalf("Settings: %s", r.Error())
		}
	}
}
