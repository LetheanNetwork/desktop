// SPDX-Licence-Identifier: EUPL-1.2

// Package welfare is the guard layer between the user's chat input and the
// model (RFC.welfare). It detects hostile prompt shapes — slurs, sustained
// anger — and, rather than refusing or silently sanitising, opens a meta-
// session where the engine speaks to the model as a peer and lets the model
// decide how to handle it.
//
// This file + detect.go are the DETECT half (RFC.welfare §1): score the user's
// latest message, decide whether the mediation trigger fires. The MEDIATE half
// (§2 — the engine↔model session, engine_ok / engine_rephrase / engine_pause)
// lands separately; it carries Snider's engine-opener voice and is built with
// him, not solo.
package welfare

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/welfare/slurs"
)

// Config tunes the detector. Zero-value uses the RFC.welfare defaults; tunable
// per-deployment.
type Config struct {
	AngerThreshold     float64 // AngerScore above this is "elevated" (default 0.7)
	SustainedThreshold float64 // SustainedHostility above this gates anger (default 0.5)
	SustainedWindow    int     // prior user turns weighed for sustained hostility (default 4)
	AngerFloor         float64 // a prior turn counts toward sustained at/above this (default 0.4)
}

// Service is the welfare guard. Detect is the per-message entry point.
type Service struct {
	cfg     Config
	matcher *slurs.Matcher
	mu      sync.Mutex
	history map[string][]float64 // per-session rolling AngerScores
}

// New constructs the welfare Service over the curated slur catalogue, applying
// RFC.welfare defaults to any zero-value Config field.
//
//	w := welfare.New(welfare.Config{})
func New(cfg Config) *Service {
	if cfg.AngerThreshold == 0 {
		cfg.AngerThreshold = 0.7
	}
	if cfg.SustainedThreshold == 0 {
		cfg.SustainedThreshold = 0.5
	}
	if cfg.SustainedWindow == 0 {
		cfg.SustainedWindow = 4
	}
	if cfg.AngerFloor == 0 {
		cfg.AngerFloor = 0.4
	}
	return &Service{
		cfg:     cfg,
		matcher: slurs.Default(),
		history: make(map[string][]float64),
	}
}

// Register builds the welfare Service for core registration. The runner hook
// (ChatCtx → Detect → mediate) wires in with the mediate half.
//
//	core.New(core.WithName("welfare", welfare.Register))
func Register(_ *core.Core) core.Result { return core.Ok(New(Config{})) }

// ServiceName is the Wails binding name.
func (s *Service) ServiceName() string { return "Welfare" }
