// SPDX-Licence-Identifier: EUPL-1.2

package update_test

import (
	core "dappco.re/go"
	upstream "dappco.re/go/update"
	lthn "dappco.re/lthn/desktop"
	"dappco.re/lthn/desktop/pkg/update"
)

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
