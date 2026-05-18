// SPDX-Licence-Identifier: EUPL-1.2

package benchmark

import (
	core "dappco.re/go"
)

// FixtureBencher is a deterministic Bencher used for substrate
// self-tests AND for fixture-mode in the frontend (registering a
// fixture bencher so the admin Benchmark window has a Bench source
// available before any real adapters land).
//
// The fixture returns Runs from its Canned slice in order, advancing
// a position on each Bench call. When Canned is empty, Bench returns
// a baseline Run synthesised from the request (pp/tg set to fixed
// representative values) so callers get something realistic-looking
// without needing to pre-populate.
//
// Usage example:
//
//	fx := &benchmark.FixtureBencher{
//	    BencherName: "fixture-local",
//	    BencherKind: benchmark.KindLocal,
//	    Canned: []benchmark.Run{{Model: "gemma-4-e2b", PpTokSec: 4800, TgTokSec: 47.2, Ctx: 2048}},
//	}
//	svc.RegisterBencher(fx)
type FixtureBencher struct {
	BencherName string
	BencherKind Kind
	BencherDesc string
	// ModelList overrides the auto-derived model list returned by
	// Models(). Empty → derive unique models from Canned.
	ModelList []string
	Canned    []Run
	pos       int
}

// Name implements Bencher.
func (f *FixtureBencher) Name() string { return f.BencherName }

// Kind implements Bencher.
func (f *FixtureBencher) Kind() Kind { return f.BencherKind }

// Describe satisfies the optional Describer interface so the
// fixture shows a sensible BencherInfo.Description in ListBenchers.
func (f *FixtureBencher) Describe() string { return f.BencherDesc }

// CanBench returns true for any request — the fixture is a generic
// stand-in. Real adapters narrow this to declared support.
func (f *FixtureBencher) CanBench(_ Bench) bool { return true }

// Models returns ModelList when set; otherwise derives a unique list
// of models from Canned (so a fixture seeded with Run rows
// automatically advertises those models in the picker without the
// caller having to repeat them).
//
// Usage example:
//
//	fx := &benchmark.FixtureBencher{ModelList: []string{"gemma-4-e2b"}}
//	r := fx.Models(core.Background())
//	if r.OK { ids := r.Value.([]string); _ = ids }
func (f *FixtureBencher) Models(_ core.Context) core.Result {
	if len(f.ModelList) > 0 {
		out := make([]string, len(f.ModelList))
		copy(out, f.ModelList)
		return core.Ok(out)
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(f.Canned))
	for _, r := range f.Canned {
		if r.Model == "" {
			continue
		}
		if _, ok := seen[r.Model]; ok {
			continue
		}
		seen[r.Model] = struct{}{}
		out = append(out, r.Model)
	}
	return core.Ok(out)
}

// Bench returns the next canned Run, or a synthesised baseline when
// the canned slice is empty / exhausted. The substrate fills in ID +
// Timestamp + final Bencher attribution after this returns, so the
// fixture only needs to populate the measurement fields.
//
// Usage example:
//
//	r := fx.Bench(c.Context(), benchmark.Bench{Model: "gemma-4-e2b", Ctx: 2048})
//	if r.OK { run := r.Value.(benchmark.Run); _ = run }
func (f *FixtureBencher) Bench(_ core.Context, req Bench) core.Result {
	if f.pos < len(f.Canned) {
		run := f.Canned[f.pos]
		f.pos++
		// Fill in request-derived fields the canned entry left zero,
		// so tests that vary req can still observe what they passed.
		if run.Model == "" {
			run.Model = req.Model
		}
		if run.Ctx == 0 {
			run.Ctx = req.Ctx
		}
		return core.Ok(run)
	}
	return core.Ok(Run{
		Model:     req.Model,
		Ctx:       req.Ctx,
		PpTokSec:  1000.0,
		TgTokSec:  20.0,
		PromptLen: 1024,
		OutputLen: maxOrDefault(req.MaxOutput, 256),
	})
}

// maxOrDefault returns v when v > 0, otherwise fallback.
func maxOrDefault(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
