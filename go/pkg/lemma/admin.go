// SPDX-License-Identifier: EUPL-1.2

// Admin client for the lthn-mlx serve /v1/admin/* surface. Mirrors
// core/go-mlx/go/cmd/mlx/admin.go endpoint shapes (RFC §6.5,
// Mantis #73/#77/#78/#80/#96). Bearer-auth gated; token loads from
// ~/Lethean/data/admin.token by default (mode 0600 enforced upstream).
//
// Surface:
//
//	admin, _ := lemma.NewAdmin(lemma.AdminConfig{})  // default endpoint + token
//	st, _   := admin.Status(ctx)                     // GET /v1/admin/serve/status
//	mi, _   := admin.Machine(ctx)                    // GET /v1/admin/machine
//	pl, _   := admin.Profiles(ctx)                   // GET /v1/admin/profiles
//	_       := admin.Reload(ctx, lemma.ReloadRequest{...})
//	jobID, _ := admin.Download(ctx, lemma.DownloadRequest{...})
//	js, _   := admin.DownloadJob(ctx, jobID)
//
// Per the binary-is-the-model rule (feedback_binary_is_model_package_is_everything_else)
// this stays in-process — no subprocess of lthn-mlx, just an OpenAI-
// over-HTTP loopback client.

package lemma

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	core "dappco.re/go"
)

const (
	// DefaultAdminBaseURL — host:port for the admin API (no /v1).
	// Admin endpoints are at /v1/admin/* relative to this base.
	DefaultAdminBaseURL = "http://127.0.0.1:11434"

	// DefaultAdminTokenRelPath — path under $HOME where lthn-mlx
	// writes the Bearer token (mode 0600, lthn-mlx_ prefix).
	DefaultAdminTokenRelPath = "Lethean/data/admin.token"

	// DefaultAdminTimeout — most admin ops are quick (status, machine).
	// Reload/Download trigger longer-running work but return job ids
	// immediately, so the HTTP timeout stays modest.
	DefaultAdminTimeout = 30 * time.Second
)

// AdminConfig configures the Admin client. Zero-value loads token
// from DefaultAdminTokenRelPath under $HOME, targets DefaultAdminBaseURL.
type AdminConfig struct {
	BaseURL   string
	Token     string // if set, used verbatim; else loaded from TokenPath
	TokenPath string // absolute path to the admin.token file; empty = default
	Client    *http.Client
	Timeout   time.Duration
}

// Admin is the typed handle on lthn-mlx /v1/admin/*. Goroutine-safe;
// one per process is the usual shape.
type Admin struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewAdmin resolves config (loading token from disk when Token empty)
// and returns the handle. Errors when token can't be loaded — the
// admin surface is unusable without it.
//
//	admin, err := lemma.NewAdmin(lemma.AdminConfig{})
func NewAdmin(cfg AdminConfig) (*Admin, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultAdminBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultAdminTimeout
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: cfg.Timeout}
	}
	token := cfg.Token
	if token == "" {
		path := cfg.TokenPath
		if path == "" {
			homeR := core.UserHomeDir()
			if !homeR.OK {
				return nil, core.E("lemma.NewAdmin", "home dir unavailable: "+homeR.Error(), nil)
			}
			home, _ := homeR.Value.(string)
			path = core.JoinPath(home, DefaultAdminTokenRelPath)
		}
		loaded, err := loadTokenFromFile(path)
		if err != nil {
			return nil, core.E("lemma.NewAdmin", "load admin token", err)
		}
		token = loaded
	}
	return &Admin{
		baseURL: cfg.BaseURL,
		token:   token,
		client:  cfg.Client,
	}, nil
}

// ServeStatus mirrors cmd/mlx adminServeStatus. Snapshot of what
// serve was started with — config is post-profile, post-override.
type ServeStatus struct {
	ModelPath    string            `json:"model_path"`
	ProfilePath  string            `json:"profile_path,omitempty"`
	Runtime      string            `json:"runtime"`
	LoadedAtUnix int64             `json:"loaded_at_unix"`
	Config       ServeStatusConfig `json:"config"`
}

// ServeStatusConfig mirrors the cross-backend LoadConfig fields.
type ServeStatusConfig struct {
	ContextLength        int    `json:"context_length,omitempty"`
	ParallelSlots        int    `json:"parallel_slots,omitempty"`
	PromptCache          bool   `json:"prompt_cache"`
	PromptCacheMinTokens int    `json:"prompt_cache_min_tokens,omitempty"`
	CachePolicy          string `json:"cache_policy,omitempty"`
	CacheMode            string `json:"cache_mode,omitempty"`
	BatchSize            int    `json:"batch_size,omitempty"`
	PrefillChunkSize     int    `json:"prefill_chunk_size,omitempty"`
	ExpectedQuantization int    `json:"expected_quantization,omitempty"`
	MemoryLimitBytes     uint64 `json:"memory_limit_bytes,omitempty"`
	CacheLimitBytes      uint64 `json:"cache_limit_bytes,omitempty"`
	WiredLimitBytes      uint64 `json:"wired_limit_bytes,omitempty"`
	AdapterPath          string `json:"adapter_path,omitempty"`
}

// MachineInfo mirrors cmd/mlx adminMachineInfo. The pairing handshake
// target (RFC §3.1.2) — Mod\Pairing on lthn.ai hits exactly this.
type MachineInfo struct {
	Hash      string                 `json:"hash"`
	Hostname  string                 `json:"hostname,omitempty"`
	Runtime   string                 `json:"runtime"`
	GoVersion string                 `json:"go_version,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

// ProfilesList mirrors cmd/mlx adminProfilesList. Lists tuning
// profiles in the standard dir (cmd/mlx adminPathProfiles).
type ProfilesList struct {
	Dir      string    `json:"dir"`
	Profiles []Profile `json:"profiles"`
}

// Profile carries the minimal fields the picker needs.
type Profile struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Model    string `json:"model,omitempty"`
	Backend  string `json:"backend,omitempty"`
	Modified int64  `json:"modified_unix,omitempty"`
}

// ReloadRequest is the body for POST /v1/admin/serve/reload. ConfirmMachine
// is the machine hash from Status/Machine; reload rejects if it doesn't
// match the running instance (operator-foot-gun gate per #77).
type ReloadRequest struct {
	ConfirmMachine string `json:"confirm_machine"`
	ModelPath      string `json:"model_path,omitempty"`
	ProfilePath    string `json:"profile_path,omitempty"`
	ContextLength  int    `json:"context_length,omitempty"`
}

// DownloadRequest is the body for POST /v1/admin/models/download.
// RepoID is the HF repo (allowlist-gated upstream); Revision optional.
type DownloadRequest struct {
	RepoID   string `json:"repo_id"`
	Revision string `json:"revision,omitempty"`
}

// DownloadJobStatus is the response for GET /v1/admin/models/download?job=ID
// + the kick response from POST. Status transitions: pending → running →
// done | failed.
type DownloadJobStatus struct {
	JobID    string `json:"job_id"`
	Status   string `json:"status"`
	RepoID   string `json:"repo_id,omitempty"`
	Revision string `json:"revision,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
	Error    string `json:"error,omitempty"`
	Path     string `json:"path,omitempty"`
}

// Status returns the boot-time snapshot of the running serve instance.
//
//	st, err := admin.Status(ctx)
//	if err != nil { return err }
//	fmt.Println(st.ModelPath, st.Config.ContextLength)
func (a *Admin) Status(ctx context.Context) (ServeStatus, error) {
	var out ServeStatus
	if err := a.doJSON(ctx, http.MethodGet, "/v1/admin/serve/status", nil, &out); err != nil {
		return ServeStatus{}, core.E("lemma.Admin.Status", "request failed", err)
	}
	return out, nil
}

// Machine returns the machine identity used by the pairing handshake.
//
//	mi, err := admin.Machine(ctx)
//	fmt.Println(mi.Hash)
func (a *Admin) Machine(ctx context.Context) (MachineInfo, error) {
	var out MachineInfo
	if err := a.doJSON(ctx, http.MethodGet, "/v1/admin/machine", nil, &out); err != nil {
		return MachineInfo{}, core.E("lemma.Admin.Machine", "request failed", err)
	}
	return out, nil
}

// Profiles lists tuning profiles in the standard dir.
//
//	pl, err := admin.Profiles(ctx)
//	for _, p := range pl.Profiles { fmt.Println(p.Name) }
func (a *Admin) Profiles(ctx context.Context) (ProfilesList, error) {
	var out ProfilesList
	if err := a.doJSON(ctx, http.MethodGet, "/v1/admin/profiles", nil, &out); err != nil {
		return ProfilesList{}, core.E("lemma.Admin.Profiles", "request failed", err)
	}
	return out, nil
}

// Reload hot-swaps the loaded model. Caller must supply ConfirmMachine
// (from Status() or Machine()) — server-side gate stops accidental
// reload of the wrong instance.
//
//	if err := admin.Reload(ctx, lemma.ReloadRequest{
//	    ConfirmMachine: mi.Hash,
//	    ModelPath:      "/Lethean/models/lemer-lite-2026-05",
//	}); err != nil { return err }
func (a *Admin) Reload(ctx context.Context, req ReloadRequest) error {
	if core.Trim(req.ConfirmMachine) == "" {
		return core.E("lemma.Admin.Reload", "confirm_machine required (run Machine() first)", nil)
	}
	if err := a.doJSON(ctx, http.MethodPost, "/v1/admin/serve/reload", req, nil); err != nil {
		return core.E("lemma.Admin.Reload", "request failed", err)
	}
	return nil
}

// Download kicks off an async HF repo fetch. Returns the job_id;
// caller polls DownloadJob(jobID) to monitor.
//
//	jobID, err := admin.Download(ctx, lemma.DownloadRequest{
//	    RepoID: "lthn/lemer-lite", Revision: "main",
//	})
func (a *Admin) Download(ctx context.Context, req DownloadRequest) (string, error) {
	if core.Trim(req.RepoID) == "" {
		return "", core.E("lemma.Admin.Download", "repo_id required", nil)
	}
	var out DownloadJobStatus
	if err := a.doJSON(ctx, http.MethodPost, "/v1/admin/models/download", req, &out); err != nil {
		return "", core.E("lemma.Admin.Download", "request failed", err)
	}
	if core.Trim(out.JobID) == "" {
		return "", core.E("lemma.Admin.Download", "server omitted job_id", nil)
	}
	return out.JobID, nil
}

// DownloadJob polls the status of an in-flight download job.
//
//	for {
//	    js, _ := admin.DownloadJob(ctx, jobID)
//	    if js.Status == "done" || js.Status == "failed" { break }
//	    time.Sleep(2 * time.Second)
//	}
func (a *Admin) DownloadJob(ctx context.Context, jobID string) (DownloadJobStatus, error) {
	if core.Trim(jobID) == "" {
		return DownloadJobStatus{}, core.E("lemma.Admin.DownloadJob", "job id required", nil)
	}
	var out DownloadJobStatus
	url := "/v1/admin/models/download?job=" + jobID
	if err := a.doJSON(ctx, http.MethodGet, url, nil, &out); err != nil {
		return DownloadJobStatus{}, core.E("lemma.Admin.DownloadJob", "request failed", err)
	}
	return out, nil
}

// doJSON is the one-liner verb helper. Marshals body when non-nil,
// adds Bearer header + Accept JSON, parses response into out when
// non-nil. 4xx/5xx returns an error carrying the upstream body so
// the caller (CLI or UI) can surface the user-visible reason.
func (a *Admin) doJSON(ctx context.Context, method, path string, body, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return core.E("lemma.Admin.doJSON", "marshal request body", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, reqBody)
	if err != nil {
		return core.E("lemma.Admin.doJSON", "build request", err)
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return core.E("lemma.Admin.doJSON", "transport", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		// Upstream returns text/plain for http.Error, JSON for our
		// own emitters. Caller just needs the bytes either way.
		return core.E("lemma.Admin.doJSON",
			"status "+core.Itoa(resp.StatusCode)+": "+string(respBody), nil)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return core.E("lemma.Admin.doJSON", "decode response", err)
	}
	return nil
}

// loadTokenFromFile reads + trims an admin token from disk. Empty
// file is rejected (would attempt unauthenticated calls otherwise).
// Mode-check is deferred to the upstream writer (lthn-mlx writes 0600);
// re-checking here only adds friction without security improvement —
// the file is already in the user's home dir under their UID.
func loadTokenFromFile(path string) (string, error) {
	r := core.ReadFile(path)
	if !r.OK {
		return "", core.E("lemma.loadTokenFromFile", "read "+path+": "+r.Error(), nil)
	}
	raw, ok := r.Value.([]byte)
	if !ok {
		return "", core.E("lemma.loadTokenFromFile", "unexpected ReadFile result type", nil)
	}
	tok := core.Trim(string(raw))
	if tok == "" {
		return "", core.E("lemma.loadTokenFromFile", "token file empty: "+path, nil)
	}
	return tok, nil
}
