// SPDX-License-Identifier: EUPL-1.2

// Wails service surface for pkg/lemma. Exposes the Admin client
// methods to the WebView so Lit elements can drive model picker,
// downloader, and live status without leaking the Bearer token to JS.
//
// Bound by application.NewService(lemma.NewWailsService(...)) in
// pkg/desktop/desktop.go; the package-level Admin stays available
// for non-WebView callers (CLI verbs, Core actions, MCP tools).
//
// Wails generates the TypeScript binding under
// frontend/bindings/dappco.re/lthn/desktop/pkg/lemma/.

package lemma

import (
	"context"
	"sync"
	"time"

	core "dappco.re/go"
)

// WailsService is the Wails-bound facade. Carries an AdminConfig
// describing how to talk to the local lthn-mlx; reconstructs the
// Admin handle lazily on each method call so a token rotation or a
// freshly-started lthn-mlx that wasn't running at desktop boot still
// "just works" without re-binding.
//
//	application.NewService(lemma.NewWailsService(lemma.AdminConfig{}))
type WailsService struct {
	cfg AdminConfig
	mu  sync.RWMutex
}

// NewWailsService constructs the WailsService. Zero AdminConfig uses
// the default endpoint (127.0.0.1:11434) and token path
// (~/Lethean/data/admin.token).
//
//	application.NewService(lemma.NewWailsService(lemma.AdminConfig{}))
func NewWailsService(cfg AdminConfig) *WailsService {
	return &WailsService{cfg: cfg}
}

// ServiceName labels the binding namespace exposed to JS as "Lemma".
// Wails-generated TS lives at
// frontend/bindings/dappco.re/lthn/desktop/pkg/lemma/.
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
// as an error string the Lit element renders into the status pill.
//
//	const st = await Lemma.Status()
func (s *WailsService) Status(ctx context.Context) (ServeStatus, error) {
	a, err := s.admin()
	if err != nil {
		return ServeStatus{}, err
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
	return a.Profiles(ctx)
}

// Reload hot-swaps the loaded model. Caller must supply the machine
// hash via the same struct field — frontend reads it from Machine()
// first, then echoes it back as ConfirmMachine to prove the operator
// intended this instance specifically.
//
//	await Lemma.Reload({ ConfirmMachine: mi.hash, ModelPath: picked })
func (s *WailsService) Reload(ctx context.Context, req ReloadRequest) error {
	a, err := s.admin()
	if err != nil {
		return err
	}
	return a.Reload(ctx, req)
}

// Download queues an HF-allowlisted model fetch. Returns the job_id
// for follow-up polling via DownloadJob.
//
//	const jobID = await Lemma.Download({ RepoID: "lthn/lemer-lite" })
func (s *WailsService) Download(ctx context.Context, req DownloadRequest) (string, error) {
	a, err := s.admin()
	if err != nil {
		return "", err
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
	return a.SFTAdapters(ctx)
}

// admin lazily builds the Admin client. Per-call rather than per-
// service-instance so token rotation + late-start lthn-mlx both work
// without re-binding the Wails service.
func (s *WailsService) admin() (*Admin, error) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	a, err := NewAdmin(cfg)
	if err != nil {
		return nil, core.E("lemma.WailsService.admin", "admin client unavailable", err)
	}
	return a, nil
}
