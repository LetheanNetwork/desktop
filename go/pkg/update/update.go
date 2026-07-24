// SPDX-Licence-Identifier: EUPL-1.2

// Package update wraps the upstream dappco.re/go/update service for
// lthn/desktop. The upstream package compares its build-embedded
// Version constant against the latest GitHub release on
// LetheanNetwork/desktop. lthn stamps dappco.re/lthn/desktop.Version
// at build time, and this wrapper syncs that single value into
// go-update before checks run.
//
// Today the service is registered with CheckOnStartup = NoCheck so no
// network traffic fires at boot; consumers call Service.Start() to
// kick off a check. A future tray "Check for updates" entry / UI
// affordance triggers Start; a future scheduled task can fire it on a
// cadence via the queue service.
//
// Usage example:
//
//	core.New(core.WithName("update", update.Register))
//	// later:
//	svc, _ := core.ServiceFor[*update.Service](c, "update")
//	if svc != nil { _ = svc.Start() }
package update

import (
	core "dappco.re/go"
	"dappco.re/go/config"
	upstream "dappco.re/go/update"
	lthn "dappco.re/lthn/desktop"
)

// Service is the lthn-side owner of the upstream update.UpdateService.
// inner is nil when the upstream constructor failed (typically a bad
// RepoURL) — methods nil-guard so Core boot stays clean and the
// failure surfaces only when something tries to use the service.
type Service struct {
	inner *upstream.UpdateService
	cfg   upstream.UpdateServiceConfig
	core  *core.Core
}

// DefaultRepoURL is the GitHub mirror that ships release artefacts.
// forge.lthn.sh / forge.lthn.ai are the canonical homes for source +
// agent work, but the release-asset surface lives on GitHub for now.
const DefaultRepoURL = "https://github.com/LetheanNetwork/desktop"

// DefaultChannel matches the GitHub release channel filter the
// upstream service understands. "stable" returns non-prerelease tags;
// "beta" returns prerelease tags too.
const DefaultChannel = "stable"

// New constructs the wrapper with the given upstream config. Returns
// Result with Value = *Service on OK; the lthn-side wrapper exists
// regardless so a degraded inner doesn't lose the registration
// (matches the pkg/fleet / pkg/ai lock-tolerant pattern).
//
// Usage example:
//
//	r := update.New(upstream.UpdateServiceConfig{
//	    RepoURL:        "https://github.com/owner/repo",
//	    Channel:        "stable",
//	    CheckOnStartup: upstream.NoCheck,
//	})
func New(cfg upstream.UpdateServiceConfig) core.Result {
	syncUpstreamVersion()
	r := upstream.NewUpdateService(cfg)
	if !r.OK {
		core.Warn("update.New: upstream construction failed, registering degraded Service", "err", r.Error())
		return core.Ok(&Service{cfg: cfg})
	}
	inner, _ := r.Value.(*upstream.UpdateService)
	return core.Ok(&Service{inner: inner, cfg: cfg})
}

// Register is the core.WithName-compatible factory. Configures the
// service to point at the canonical GitHub release mirror with the
// stable channel and CheckOnStartup = NoCheck so registering doesn't
// fire a network call. Callers (CLI verb / UI affordance) explicitly
// invoke Start() when they want to check.
//
// Usage example:
//
//	core.New(core.WithName("update", update.Register))
func Register(c *core.Core) core.Result {
	syncUpstreamVersion()
	result := New(upstream.UpdateServiceConfig{
		RepoURL:        DefaultRepoURL,
		Channel:        DefaultChannel,
		CheckOnStartup: upstream.NoCheck,
	})
	if !result.OK {
		return result
	}
	service := result.Value.(*Service)
	service.core = c
	return core.Ok(service)
}

// OnStartup reconfigures the updater from the registered desktop config
// service. The default remains no check; configured checks run in Core's
// tracked background pool so network availability cannot block desktop boot.
//
// Usage example:
//
//	_ = svc.OnStartup(core.Background())
func (s *Service) OnStartup(_ core.Context) core.Result {
	if s == nil {
		return core.Fail(core.E("update.Service.OnStartup", "service is nil", nil))
	}
	if s.core == nil {
		return core.Fail(core.E("update.Service.OnStartup", "core is nil", nil))
	}
	cfgSvc, ok := core.ServiceFor[*config.Service](s.core, "config")
	if !ok || cfgSvc == nil || cfgSvc.Config() == nil {
		return core.Ok(nil)
	}

	channel := DefaultChannel
	var configuredChannel string
	if cfgSvc.Get("desktop.updater.channel", &configuredChannel).OK {
		switch core.Lower(configuredChannel) {
		case "stable", "beta":
			channel = core.Lower(configuredChannel)
		}
	}

	mode := upstream.NoCheck
	var configuredMode string
	if cfgSvc.Get("desktop.updater.check_on_startup", &configuredMode).OK {
		switch core.Lower(configuredMode) {
		case "check":
			mode = upstream.CheckOnStartup
		case "check-and-install":
			mode = upstream.CheckAndUpdateOnStartup
		}
	}

	cfg := s.cfg
	cfg.Channel = channel
	cfg.CheckOnStartup = mode
	syncUpstreamVersion()
	result := upstream.NewUpdateService(cfg)
	if !result.OK {
		return core.Fail(core.E(
			"update.Service.OnStartup",
			"configure updater",
			result.Err(),
		))
	}
	s.inner = result.Value.(*upstream.UpdateService)
	s.cfg = cfg
	if mode != upstream.NoCheck {
		s.core.Go(func() {
			if start := s.Start(); !start.OK {
				core.Warn("update startup check failed", "err", start.Error())
			}
		})
	}
	return core.Ok(nil)
}

// Start fires the upstream check using the configured CheckOnStartup
// mode. With NoCheck the upstream returns OK without touching the
// network; CheckOnStartup returns the latest-release info; the
// CheckAndUpdateOnStartup mode applies the update too. Switch mode
// by reconstructing the service with a different cfg.
//
// Usage example:
//
//	r := svc.Start()
//	if !r.OK { core.Warn("update: check failed", "err", r.Error()) }
func (s *Service) Start() core.Result {
	if s.inner == nil {
		return core.Fail(core.NewError("update: service unavailable (construction failed at boot)"))
	}
	syncUpstreamVersion()
	return s.inner.Start()
}

// Version returns the build-embedded version string the upstream
// service compares against latest. Reads upstream.Version directly so
// the same value the release-check uses surfaces to the UI.
//
// Usage example:
//
//	core.Println("running:", svc.Version())
func (s *Service) Version() string {
	syncUpstreamVersion()
	return lthn.Version
}

func syncUpstreamVersion() {
	upstream.Version = lthn.Version
}

// Config returns the upstream config the service was registered with.
// Useful for inspection (UI: "checking github.com/foo/bar on the
// stable channel") and for the future case of mutating config at
// runtime via a Reconfigure call.
//
// Usage example:
//
//	cfg := svc.Config()
//	core.Println("repo:", cfg.RepoURL, "channel:", cfg.Channel)
func (s *Service) Config() upstream.UpdateServiceConfig { return s.cfg }
