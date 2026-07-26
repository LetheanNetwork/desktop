// SPDX-License-Identifier: EUPL-1.2

// Wails service surface for pkg/lemma. Exposes the Admin client
// methods to the WebView so Angular surfaces can drive model picker,
// downloader, and live status without leaking the Bearer token to JS.
//
// Bound by application.NewService(lemma.NewWailsService(...)) in
// pkg/desktop/desktop.go; the package-level Admin stays available
// for non-WebView callers (CLI verbs, Core actions, MCP tools).
//
// Wails generates the TypeScript binding under
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/lemma/.

package lemma

import (
	"context"
	"errors"
	"sync"
	"time"

	core "dappco.re/go"
)

// WailsService is the Wails-bound facade. Carries an AdminConfig
// describing how to talk to the local lthn-mlx driver, plus a
// HostConfig for the lthn-ai orchestration host (:9100). Reconstructs
// the Admin / Host handles lazily on each method call so a token
// rotation or a freshly-started lthn-mlx that wasn't running at desktop
// boot still "just works" without re-binding.
//
//	application.NewService(lemma.NewWailsService(lemma.AdminConfig{}))
type WailsService struct {
	cfg     AdminConfig
	hostCfg HostConfig
	mu      sync.RWMutex
}

// NewWailsService constructs the WailsService. Zero AdminConfig uses
// the default driver endpoint (127.0.0.1:11434) and token path
// (~/Lethean/data/admin.token); the host endpoint defaults to lthn-ai
// on 127.0.0.1:9100.
//
//	application.NewService(lemma.NewWailsService(lemma.AdminConfig{}))
func NewWailsService(cfg AdminConfig) *WailsService {
	return &WailsService{cfg: cfg}
}

// errLemmaNotConfigured is the user-facing message mutation methods
// return when the admin token file is missing (lthn-mlx never run).
// Reads return zero-value + nil for the same condition; mutations
// fail loudly so the operator sees why their button-click was a
// no-op instead of getting silent success.
var errLemmaNotConfigured = core.E("lemma.WailsService",
	"Lemma is not configured — start lthn-mlx serve to enable the admin surface", nil)

// ServiceName labels the binding namespace exposed to JS as "Lemma".
// Wails-generated TS lives at
// frontend-ng/bindings/dappco.re/lthn/desktop/pkg/lemma/.
func (s *WailsService) ServiceName() string { return "Lemma" }

// ConfigureEndpoint lets the UI redirect at runtime — useful when the
// user starts lthn-mlx on a non-default port or runs against a paired
// remote endpoint. Empty baseURL resets to the default.
//
//	await Lemma.ConfigureEndpoint("http://192.168.1.50:11434")
func (s *WailsService) ConfigureEndpoint(baseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.BaseURL = baseURL
}

// Status returns the boot-time snapshot of the running serve instance.
// First failure-mode the UI hits is "lthn-mlx not running" — surfaces
// as an error string the Angular surface renders into the status pill.
//
//	const st = await Lemma.Status()
func (s *WailsService) Status(ctx context.Context) (ServeStatus, error) {
	a, err := s.admin()
	if err != nil {
		return ServeStatus{}, err
	}
	if a == nil {
		return ServeStatus{}, nil
	}
	return a.Status(ctx)
}

// Machine returns the machine identity used by lthn.ai pairing. The
// frontend uses this hash as the --confirm value when calling Reload.
//
//	const mi = await Lemma.Machine()
func (s *WailsService) Machine(ctx context.Context) (MachineInfo, error) {
	a, err := s.admin()
	if err != nil {
		return MachineInfo{}, err
	}
	if a == nil {
		return MachineInfo{}, nil
	}
	return a.Machine(ctx)
}

// Profiles lists tuning profiles in the engine's standard dir. Names
// map 1:1 to the Reload() ProfilePath argument.
//
//	const pl = await Lemma.Profiles()
//	for (const p of pl.profiles) { ... }
func (s *WailsService) Profiles(ctx context.Context) (ProfilesList, error) {
	a, err := s.admin()
	if err != nil {
		return ProfilesList{}, err
	}
	if a == nil {
		return ProfilesList{}, nil
	}
	return a.Profiles(ctx)
}

// Reload makes the picked (model, profile) live. The binding signature
// is unchanged — the frontend still calls Lemma.Reload(ReloadRequest) —
// but the routing now depends on whether an adapter overlay is asked
// for:
//
//   - No adapter (the common "Use this model" case): routes through the
//     lthn-ai host's POST /v1/driver/serve. The host hot-swaps the model
//     in place AND persists the (model, profile) to
//     ~/Lethean/data/lthn-ai-serve.json, so the next boot restores the
//     operator's last pick. ConfirmMachine is ignored here — the host
//     supervises a single local driver, so the machine-foot-gun gate the
//     driver's admin/reload enforces is moot.
//
//   - Adapter set (the Fine-tune A/B overlay case): routes through the
//     driver's Bearer-gated /v1/admin/serve/reload, because the host's
//     serve surface carries no adapter field. This preserves the adapter
//     overlay rather than silently dropping it — but the choice does NOT
//     persist across a host restart (the admin/reload path doesn't write
//     the serve file). That is acceptable: an adapter overlay is a
//     transient A/B test, not a default-model selection.
//
//     await Lemma.Reload({ ModelPath: picked, ProfilePath: prof })          // → host serve (persists)
//     await Lemma.Reload({ ModelPath: picked, AdapterPath: adapterDir })    // → driver admin/reload (overlay)
func (s *WailsService) Reload(ctx context.Context, req ReloadRequest) error {
	// Adapter overlay → driver admin/reload (host serve has no adapter field).
	if core.Trim(req.AdapterPath) != "" {
		a, err := s.admin()
		if err != nil {
			return err
		}
		if a == nil {
			return errLemmaNotConfigured
		}
		return a.Reload(ctx, req)
	}
	// No adapter → lthn-ai host serve: hot-swap in place + persist the pick.
	return s.host().Serve(ctx, HostServeRequest{
		Runtime: "mlx",
		Model:   req.ModelPath,
		Profile: req.ProfilePath,
		Context: req.ContextLength,
	})
}

// Download kicks a verified HF repo fetch through the driver and returns the
// job id for follow-up polling via DownloadJob. The model-browser catalogue's
// Download button calls this with a repo id.
//
// The driver's download surface is allowlist-gated
// (~/Lethean/data/allowed-models.json, fail-closed) — a non-allowed repo is
// refused with 403. Clicking Download in the catalogue is the operator's
// explicit intent to permit that repo, so this ensures the repo is on the
// allowlist before kicking the job. Without that step every catalogue download
// would dead-end on a 403 the user can't resolve from the UI. The repo stays
// permitted for future fetches (an append, not a replace), matching the
// operator-curated nature of the file.
//
//	const id = await Lemma.Download({ repo: "mlx-community/gemma-4-e2b-it-mxfp8" })
func (s *WailsService) Download(ctx context.Context, req DownloadRequest) (string, error) {
	if core.Trim(req.Repo) == "" {
		return "", core.E("lemma.WailsService.Download", "repo required", nil)
	}
	a, err := s.admin()
	if err != nil {
		return "", err
	}
	if a == nil {
		return "", errLemmaNotConfigured
	}
	// Permit the repo before the driver's allowlist gate sees it. Best-effort
	// on the read side (a parse error there is surfaced), but the write must
	// succeed or the kick below will 403 — so a write failure is returned.
	if err := ensureRepoAllowed(req.Repo); err != nil {
		return "", core.E("lemma.WailsService.Download", "permit repo for download", err)
	}
	return a.Download(ctx, req)
}

// DownloadJob polls a download. UI typically calls this on a ~2s
// interval until status == "done" or "failed".
//
//	const js = await Lemma.DownloadJob(jobID)
func (s *WailsService) DownloadJob(ctx context.Context, jobID string) (DownloadJobStatus, error) {
	a, err := s.admin()
	if err != nil {
		return DownloadJobStatus{}, err
	}
	if a == nil {
		return DownloadJobStatus{}, nil
	}
	return a.DownloadJob(ctx, jobID)
}

// SFTStart kicks a native LoRA fine-tune. Single-flight upstream —
// returns an error when another job is already running. The returned
// SFTJob carries the JobID the UI uses for follow-up polls.
//
//	const job = await Lemma.SFTStart({ ModelPath, DatasetPath, Epochs: 3 })
func (s *WailsService) SFTStart(ctx context.Context, req SFTStartRequest) (SFTJob, error) {
	a, err := s.admin()
	if err != nil {
		return SFTJob{}, err
	}
	if a == nil {
		return SFTJob{}, errLemmaNotConfigured
	}
	return a.SFTStart(ctx, req)
}

// SFTStatus polls a job. UI typically calls on a ~2s interval while
// the run is active; SFTJob carries the latest step/epoch/loss/samples
// + the rolling loss-curve ring buffer.
//
//	const job = await Lemma.SFTStatus(jobID)
func (s *WailsService) SFTStatus(ctx context.Context, jobID string) (SFTJob, error) {
	a, err := s.admin()
	if err != nil {
		return SFTJob{}, err
	}
	if a == nil {
		return SFTJob{}, nil
	}
	return a.SFTStatus(ctx, jobID)
}

// SFTStop cancels the in-flight job. Checkpoints already written to
// the adapter dir survive — only the gradient loop stops.
//
//	await Lemma.SFTStop(jobID)
func (s *WailsService) SFTStop(ctx context.Context, jobID string) (SFTJob, error) {
	a, err := s.admin()
	if err != nil {
		return SFTJob{}, err
	}
	if a == nil {
		return SFTJob{}, errLemmaNotConfigured
	}
	return a.SFTStop(ctx, jobID)
}

// SFTAdapters lists completed adapter directories. UI renders these
// as a "Recent Adapters" rail; sort by ModifiedAt descending for
// freshness order.
//
//	const list = await Lemma.SFTAdapters()
//	for (const a of list.Adapters) { ... }
func (s *WailsService) SFTAdapters(ctx context.Context) (SFTAdaptersList, error) {
	a, err := s.admin()
	if err != nil {
		return SFTAdaptersList{}, err
	}
	if a == nil {
		return SFTAdaptersList{}, nil
	}
	return a.SFTAdapters(ctx)
}

// admin lazily builds the Admin client. Per-call rather than per-
// service-instance so token rotation + late-start lthn-mlx both work
// without re-binding the Wails service.
//
// "Token file does not exist" is treated as a benign "Lemma not
// configured" state — returns (nil, nil) so the tray's per-second
// Status / SFTStatus poll doesn't spam the dev log with a
// not-found error on every iteration. Callers detect nil admin and
// return zero-value snapshots; the frontend already handles those
// (.then(s => s, () => null) collapses both into the no-engine UI).
func (s *WailsService) admin() (*Admin, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	a, err := NewAdmin(cfg)
	if err != nil {
		if errors.Is(err, ErrNoTokenFile) {
			return nil, nil
		}
		return nil, core.E("lemma.WailsService.admin", "admin client unavailable", err)
	}
	return a, nil
}

// host lazily builds the lthn-ai Host client. Per-call (matching admin())
// so a late-started lthn-ai works without re-binding; there is no token to
// load, so this never fails — an unreachable host surfaces as a transport
// error when Serve is actually called.
func (s *WailsService) host() *Host {
	s.mu.RLock()
	cfg := s.hostCfg
	s.mu.RUnlock()
	return NewHost(cfg)
}
