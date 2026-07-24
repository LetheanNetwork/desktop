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
