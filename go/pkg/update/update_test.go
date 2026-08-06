// SPDX-Licence-Identifier: EUPL-1.2

package update_test

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	upstream "dappco.re/go/update"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/update"
)

func updateConfigFixture(
	t *core.T,
	values map[string]any,
) (*core.Core, *update.Service) {
	t.Helper()
	path := core.PathJoin(t.TempDir(), "lthn.yaml")
	c := core.New(
		core.WithName("config", config.NewConfigServiceWith(config.ServiceOptions{
			Path: path,
		})),
		core.WithName("update", update.Register),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	cfg, ok := core.ServiceFor[*config.Service](c, "config")
	core.RequireTrue(t, ok)
	for key, value := range values {
		core.RequireTrue(t, cfg.Set(key, value).OK)
	}
	svc, ok := core.ServiceFor[*update.Service](c, "update")
	core.RequireTrue(t, ok)
	core.AssertNotNil(t, svc)
	core.RequireTrue(t, svc.OnStartup(core.Background()).OK)
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	return c, svc
}

func TestUpdate_Service_Version_Good_UsesSharedDesktopVersion(t *core.T) {
	originalDesktop := lthn.Version
	originalUpstream := upstream.Version
	t.Cleanup(func() {
		lthn.Version = originalDesktop
		upstream.Version = originalUpstream
	})
	lthn.Version = "v7.6.5-test"
	upstream.Version = "stale-upstream"

	r := update.New(upstream.UpdateServiceConfig{
		RepoURL:        update.DefaultRepoURL,
		Channel:        update.DefaultChannel,
		CheckOnStartup: upstream.NoCheck,
	})

	core.AssertTrue(t, r.OK)
	svc := r.Value.(*update.Service)
	core.AssertEqual(t, "v7.6.5-test", svc.Version())
	core.AssertEqual(t, "v7.6.5-test", upstream.Version)
}

func TestUpdate_Service_Version_Bad_DegradedServiceStillReportsSharedVersion(t *core.T) {
	originalDesktop := lthn.Version
	originalUpstream := upstream.Version
	t.Cleanup(func() {
		lthn.Version = originalDesktop
		upstream.Version = originalUpstream
	})
	lthn.Version = "v7.6.6-test"
	upstream.Version = "stale-upstream"

	r := update.New(upstream.UpdateServiceConfig{
		RepoURL: "https://github.com/owner-only",
	})

	core.AssertTrue(t, r.OK)
	svc := r.Value.(*update.Service)
	core.AssertEqual(t, "v7.6.6-test", svc.Version())
	core.AssertEqual(t, "v7.6.6-test", upstream.Version)
}

func TestUpdate_Service_Version_Ugly_TracksRuntimeVersionMutation(t *core.T) {
	originalDesktop := lthn.Version
	originalUpstream := upstream.Version
	t.Cleanup(func() {
		lthn.Version = originalDesktop
		upstream.Version = originalUpstream
	})
	lthn.Version = "v7.6.7-test"
	r := update.New(upstream.UpdateServiceConfig{
		RepoURL:        update.DefaultRepoURL,
		Channel:        update.DefaultChannel,
		CheckOnStartup: upstream.NoCheck,
	})
	core.AssertTrue(t, r.OK)
	svc := r.Value.(*update.Service)

	lthn.Version = "v7.6.8-test"

	core.AssertEqual(t, "v7.6.8-test", svc.Version())
	core.AssertEqual(t, "v7.6.8-test", upstream.Version)
}

func TestUpdate_Service_OnStartup_Good_UsesDesktopConfig(t *core.T) {
	_, svc := updateConfigFixture(t, map[string]any{
		"desktop.updater.channel":          "beta",
		"desktop.updater.check_on_startup": "never",
	})

	cfg := svc.Config()
	core.AssertEqual(t, "beta", cfg.Channel)
	core.AssertEqual(t, upstream.NoCheck, cfg.CheckOnStartup)
}

func TestUpdate_Service_OnStartup_Bad_NilService(t *core.T) {
	var svc *update.Service

	r := svc.OnStartup(core.Background())

	core.AssertFalse(t, r.OK)
	core.AssertContains(t, r.Error(), "service is nil")
}

func TestUpdate_Service_OnStartup_Ugly_InvalidValuesUseFallbacks(t *core.T) {
	_, svc := updateConfigFixture(t, map[string]any{
		"desktop.updater.channel":          "nightly",
		"desktop.updater.check_on_startup": "occasionally",
	})

	cfg := svc.Config()
	core.AssertEqual(t, update.DefaultChannel, cfg.Channel)
	core.AssertEqual(t, upstream.NoCheck, cfg.CheckOnStartup)
}

// TestUpdate_Service_OnStartup_Bad_NilCore — a Service built via
// update.New() directly (not through Register) never gets its core
// field wired. OnStartup must refuse rather than nil-deref the
// service lookup.
func TestUpdate_Service_OnStartup_Bad_NilCore(t *core.T) {
	r := update.New(upstream.UpdateServiceConfig{
		RepoURL:        update.DefaultRepoURL,
		Channel:        update.DefaultChannel,
		CheckOnStartup: upstream.NoCheck,
	})
	core.RequireTrue(t, r.OK)
	svc := r.Value.(*update.Service)

	out := svc.OnStartup(core.Background())

	core.AssertFalse(t, out.OK)
	core.AssertContains(t, out.Error(), "core is nil")
}

// TestUpdate_Service_OnStartup_Bad_NoConfigService — a Core that
// registers "update" but never registers "config". OnStartup degrades
// to a silent core.Ok(nil) rather than erroring, since a missing
// config service is a legitimate boot shape (config is optional).
func TestUpdate_Service_OnStartup_Bad_NoConfigService(t *core.T) {
	c := core.New(
		core.WithName("update", update.Register),
	)
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		_ = c.ServiceShutdown(core.Background())
	})
	svc, ok := core.ServiceFor[*update.Service](c, "update")
	core.RequireTrue(t, ok)

	r := svc.OnStartup(core.Background())

	core.AssertTrue(t, r.OK)
}

// TestUpdate_Service_Start_Bad_NilInner — New() with a RepoURL upstream
// rejects (missing repo path) degrades to inner=nil. Start() must
// refuse with the "service unavailable" message rather than nil-deref
// s.inner.
func TestUpdate_Service_Start_Bad_NilInner(t *core.T) {
	r := update.New(upstream.UpdateServiceConfig{
		RepoURL: "https://github.com/owner-only",
	})
	core.RequireTrue(t, r.OK)
	svc := r.Value.(*update.Service)

	out := svc.Start()

	core.AssertFalse(t, out.OK)
	core.AssertContains(t, out.Error(), "service unavailable")
}

// TestUpdate_Service_Start_Good_NoCheckNoNetwork — a validly
// constructed Service with CheckOnStartup=NoCheck. Start() delegates
// to the upstream service, which for NoCheck returns OK without
// touching the network (per upstream startGitHubCheck's NoCheck case)
// — hermetic, no httptest server required.
func TestUpdate_Service_Start_Good_NoCheckNoNetwork(t *core.T) {
	r := update.New(upstream.UpdateServiceConfig{
		RepoURL:        update.DefaultRepoURL,
		Channel:        update.DefaultChannel,
		CheckOnStartup: upstream.NoCheck,
	})
	core.RequireTrue(t, r.OK)
	svc := r.Value.(*update.Service)

	out := svc.Start()

	core.AssertTrue(t, out.OK)
}
