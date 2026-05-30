// SPDX-License-Identifier: EUPL-1.2

// Host client for the lthn-ai LEM Runtime host (:9100) — the orchestration
// half that supervises the model driver (lthn-mlx / lthn-cuda / lthn-amd) and
// owns serve persistence + hot-swap. This is the counterpart to admin.go: the
// Admin client talks to the *driver* directly (:11434, Bearer-gated); this Host
// client talks to the *host* that supervises that driver.
//
// Why route a model swap through the host instead of the driver's admin/reload:
// lthn-ai's POST /v1/driver/serve hot-swaps the model in place AND persists the
// (model, profile) to ~/Lethean/data/lthn-ai-serve.json so the next boot
// restores the operator's last choice. The driver's own /v1/admin/serve/reload
// swaps but does not persist — a host restart would lose the pick. So the
// model-browser's "Use this model" routes here.
//
//	host := lemma.NewHost(lemma.HostConfig{})        // default :9100
//	r := host.Serve(ctx, lemma.HostServeRequest{     // hot-swap + persist
//	    Runtime: "mlx", Model: "/path/to/weights", Profile: "/path/to/profile.json",
//	})
//
// The host serve surface has no adapter field — an adapter overlay still rides
// the driver's admin/reload path (see WailsService.Reload). Per the binary-is-
// the-model rule this stays an in-process HTTP loopback client; no subprocess.

package lemma

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	core "dappco.re/go"
)

const (
	// DefaultHostBaseURL — host:port for the lthn-ai orchestration API (no /v1).
	// Driver-serve verbs are at /v1/driver/* relative to this base. lthn-ai owns
	// :9100; the driver's own :11434 sits behind it (reached via the Admin client).
	DefaultHostBaseURL = "http://127.0.0.1:9100"

	// DefaultHostTimeout — a cold-start serve waits on the driver answering
	// /v1/health (the host gates "live" on readiness, up to ~30s on its side),
	// so the client timeout is generous enough to outlast that without the HTTP
	// layer giving up before the host replies.
	DefaultHostTimeout = 90 * time.Second
)

// HostConfig configures the Host client. Zero-value targets DefaultHostBaseURL
// with DefaultHostTimeout — no token (the host's driver-serve surface is
// loopback-only, not Bearer-gated like the driver's admin surface).
type HostConfig struct {
	BaseURL string
	Client  *http.Client
	Timeout time.Duration
}

// Host is the typed handle on lthn-ai's /v1/driver/* orchestration surface.
// Goroutine-safe; one per process is the usual shape.
type Host struct {
	baseURL string
	client  *http.Client
}

// NewHost resolves config and returns the handle. Unlike NewAdmin it never
// fails on construction — there is no token to load; the host surface is
// loopback-reachable whenever lthn-ai is up, and an unreachable host surfaces
// as a transport error at call time (which the UI renders into reloadErr).
//
//	host := lemma.NewHost(lemma.HostConfig{})
func NewHost(cfg HostConfig) *Host {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultHostBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultHostTimeout
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: cfg.Timeout}
	}
	return &Host{baseURL: cfg.BaseURL, client: cfg.Client}
}

// HostServeRequest is the body for POST /v1/driver/serve. Mirrors the field
// names lthn-ai's driver.ServeRequest binds (runtime/model/profile/context).
// confirm_machine and adapter_path are intentionally absent: the host ignores
// the machine gate, and its serve path carries no adapter overlay (an adapter
// rides the driver admin/reload path instead).
type HostServeRequest struct {
	Runtime string `json:"runtime"`
	Model   string `json:"model"`
	Profile string `json:"profile,omitempty"`
	Context int    `json:"context,omitempty"`
}

// hostServeResponse decodes lthn-ai's core.Result envelope. The driver provider
// renders {OK, Value} on success and {OK:false, error} on failure, so the
// client branches on OK exactly like an in-process core.Result caller.
type hostServeResponse struct {
	OK    bool   `json:"OK"`
	Error string `json:"error,omitempty"`
}

// Serve hot-swaps the served (model, profile) on the host's driver and persists
// the choice. Cold (no driver up) → the host cold-starts one; warm same-model →
// no-op; warm different-model → the host drains the old + cold-starts the new.
// The host owns all of that; this is a one-shot POST.
//
//	if err := host.Serve(ctx, lemma.HostServeRequest{
//	    Runtime: "mlx", Model: "/Lethean/data/models/lemer-lite",
//	}); err != nil { return err }
func (h *Host) Serve(ctx context.Context, req HostServeRequest) error {
	if core.Trim(req.Runtime) == "" {
		req.Runtime = "mlx"
	}
	if core.Trim(req.Model) == "" {
		return core.E("lemma.Host.Serve", "model required", nil)
	}
	r := core.JSONMarshal(req)
	if !r.OK {
		return core.E("lemma.Host.Serve", "marshal request body", r.Value.(error))
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/v1/driver/serve", bytes.NewReader(r.Value.([]byte)))
	if err != nil {
		return core.E("lemma.Host.Serve", "build request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return core.E("lemma.Host.Serve", "transport", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		// The provider renders {OK:false,error} for a serve failure; surface the
		// host's reason verbatim so the UI shows why the swap was refused.
		var env hostServeResponse
		if jr := core.JSONUnmarshal(body, &env); jr.OK && env.Error != "" {
			return core.E("lemma.Host.Serve", env.Error, nil)
		}
		return core.E("lemma.Host.Serve",
			"status "+core.Itoa(resp.StatusCode)+": "+string(body), nil)
	}
	// 2xx with OK:false shouldn't happen (the provider 500s on !OK), but guard it
	// so a future provider tweak can't make a failed serve read as success.
	var env hostServeResponse
	if jr := core.JSONUnmarshal(body, &env); jr.OK && !env.OK && env.Error != "" {
		return core.E("lemma.Host.Serve", env.Error, nil)
	}
	return nil
}
