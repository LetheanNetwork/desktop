// SPDX-Licence-Identifier: EUPL-1.2

package benchmark

import (
	core "dappco.re/go"
	"dappco.re/go/orm"
)

// runRow is the storage-tier mirror of Run. The split exists because
// Run.Extra is map[string]any (ergonomic for in-Go consumers + the
// Wails binding generator) but DuckDB needs a flat column type — so
// the storage layer keeps Extra as a JSON-encoded string and converts
// at the read/write boundary in toRow / fromRow.
//
// Not exported — only Schemas() and the package-level helpers
// (insertRun / listRuns / getRun) touch this type. Callers see Run.
type runRow struct {
	ID        string    `json:"id"`
	Timestamp core.Time `json:"timestamp"`
	Bencher   string    `json:"bencher"`
	Model     string    `json:"model"`
	Ctx       int       `json:"ctx"`
	PpTokSec  float64   `json:"pp_tok_sec"`
	TgTokSec  float64   `json:"tg_tok_sec"`
	PromptLen int       `json:"prompt_len"`
	OutputLen int       `json:"output_len"`
	PeakWatts float64   `json:"peak_watts"`
	PeakMemMB float64   `json:"peak_mem_mb"`
	Endpoint  string    `json:"endpoint"`
	Extra     string    `json:"extra"` // JSON-encoded map[string]any, empty when no extra
}

// Schema declares the DuckDB shape for runRow. Registered at Service
// boot per the orm.RegisterSchema + Medium.RegisterTable convention
// pkg/tasks established.
//
// Usage example:
//
//	for _, schema := range benchmark.Schemas() {
//	    orm.RegisterSchema(c, schema)
//	    mediumDefault.RegisterTable(schema.Name, schema)
//	}
func (runRow) Schema() orm.Schema {
	return orm.Define(func(b *orm.Builder) {
		b.Name("benchmark_runs")
		b.PK("id")
		b.String("id").NotNull()
		b.Time("timestamp").NotNull()
		b.String("bencher").NotNull()
		b.String("model").NotNull()
		b.Int("ctx").NotNull()
		b.Float64("pp_tok_sec").NotNull()
		b.Float64("tg_tok_sec").NotNull()
		b.Int("prompt_len").NotNull()
		b.Int("output_len").NotNull()
		b.Float64("peak_watts")
		b.Float64("peak_mem_mb")
		b.String("endpoint")
		b.String("extra")
		b.Index("bencher")
		b.Index("model")
		b.Index("timestamp")
	})
}

// Schemas returns the orm Schemas the benchmark subsystem owns. The
// desktop boot path (pkg/desktop/desktop.go) walks this list to
// register the tables against the live Core orm Service.
//
// Usage example:
//
//	for _, schema := range benchmark.Schemas() {
//	    orm.RegisterSchema(c, schema)
//	}
func Schemas() []orm.Schema {
	return []orm.Schema{
		runRow{}.Schema(),
	}
}

// toRow converts the public Run shape into the storage-tier runRow,
// JSON-encoding Extra so it round-trips through DuckDB's TEXT column.
// Empty Extra produces empty string (NOT "null") so consumers can
// distinguish "no extra" from "extra is JSON null".
//
// Usage example:
//
//	row := toRow(run)
//	if r := orm.Insert(c, &row); !r.OK { return r }
func toRow(run Run) (runRow, core.Result) {
	row := runRow{
		ID:        run.ID,
		Timestamp: run.Timestamp,
		Bencher:   run.Bencher,
		Model:     run.Model,
		Ctx:       run.Ctx,
		PpTokSec:  run.PpTokSec,
		TgTokSec:  run.TgTokSec,
		PromptLen: run.PromptLen,
		OutputLen: run.OutputLen,
		PeakWatts: run.PeakWatts,
		PeakMemMB: run.PeakMemMB,
		Endpoint:  run.Endpoint,
	}
	if len(run.Extra) > 0 {
		r := core.JSONMarshal(run.Extra)
		if !r.OK {
			return runRow{}, r
		}
		row.Extra = string(r.Value.([]byte))
	}
	return row, core.Ok(nil)
}

// fromRow converts a stored runRow back into the public Run shape,
// JSON-decoding Extra if present. Unparseable Extra is treated as
// non-fatal — the Run is returned with Extra = nil and a warning
// logged so a single bad record does not poison a History query.
//
// Usage example:
//
//	run := fromRow(row)
//	_ = run.Extra["billed_cost"]
func fromRow(row runRow) Run {
	run := Run{
		ID:        row.ID,
		Timestamp: row.Timestamp,
		Bencher:   row.Bencher,
		Model:     row.Model,
		Ctx:       row.Ctx,
		PpTokSec:  row.PpTokSec,
		TgTokSec:  row.TgTokSec,
		PromptLen: row.PromptLen,
		OutputLen: row.OutputLen,
		PeakWatts: row.PeakWatts,
		PeakMemMB: row.PeakMemMB,
		Endpoint:  row.Endpoint,
	}
	if row.Extra != "" {
		var extra map[string]any
		if r := core.JSONUnmarshalString(row.Extra, &extra); r.OK {
			run.Extra = extra
		} else {
			core.Warn("benchmark.fromRow.extra_unmarshal", "id", row.ID, "error", r.Value)
		}
	}
	return run
}

// insertRun stores a Run via orm.Insert against the default medium.
// Auto-fills ID (12-char random) and Timestamp (UTC now) when the
// caller left them empty; this matches the pkg/tasks convention and
// keeps Bencher implementations from having to reach into core for
// the same primitives.
//
// Usage example:
//
//	r := insertRun(c, run)
//	if r.OK { stored := r.Value.(Run); _ = stored }
func insertRun(c *core.Core, run Run) core.Result {
	if run.ID == "" {
		idResult := core.RandomString(idLength)
		if !idResult.OK {
			return idResult
		}
		run.ID = idResult.Value.(string)
	}
	if run.Timestamp.IsZero() {
		run.Timestamp = core.Now().UTC()
	}
	row, conv := toRow(run)
	if !conv.OK {
		return conv
	}
	if r := orm.Insert(c, &row); !r.OK {
		return r
	}
	return core.Ok(run)
}

// getRun returns the Run identified by id. Returns Fail when no row
// matches — callers distinguish "missing" from other errors by the
// Result.Value error code.
//
// Usage example:
//
//	r := getRun(c, "01ABC...")
//	if r.OK { run := r.Value.(Run); _ = run }
func getRun(c *core.Core, id string) core.Result {
	r := orm.Of[runRow](c).Find(id)
	if !r.OK {
		return r
	}
	row, _, ok := orm.Detail[runRow](r)
	if !ok {
		return core.Fail(core.E("benchmark.getRun", "row cast failed", nil))
	}
	return core.Ok(fromRow(row))
}

// listRuns returns Runs matching the filter, newest timestamp first.
// Pagination via Filter.Limit + Filter.Offset; zero Limit means no
// caller-imposed cap but the substrate may apply defaultListLimit
// to protect the Wails surface from accidental full-table returns.
//
// Usage example:
//
//	r := listRuns(c, Filter{Model: "gemma-4-e2b", Limit: 50})
//	if r.OK { runs := r.Value.([]Run); _ = runs }
func listRuns(c *core.Core, f Filter) core.Result {
	bridge := orm.Of[runRow](c)
	if f.Bencher != "" {
		bridge = bridge.Where("bencher", "=", f.Bencher)
	}
	if f.Model != "" {
		bridge = bridge.Where("model", "=", f.Model)
	}
	if f.MinCtx > 0 {
		bridge = bridge.Where("ctx", ">=", f.MinCtx)
	}
	if f.MaxCtx > 0 {
		bridge = bridge.Where("ctx", "<=", f.MaxCtx)
	}
	if !f.Since.IsZero() {
		bridge = bridge.Where("timestamp", ">=", f.Since)
	}
	bridge = bridge.Order("timestamp", "desc")
	limit := f.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	bridge = bridge.Limit(limit)
	if f.Offset > 0 {
		bridge = bridge.Offset(f.Offset)
	}
	r := bridge.Get()
	if !r.OK {
		return r
	}
	rows, ok := orm.Cast[[]runRow](r)
	if !ok {
		return core.Fail(core.E("benchmark.listRuns", "rows cast failed", nil))
	}
	runs := make([]Run, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, fromRow(row))
	}
	return core.Ok(runs)
}

// idLength is the random-string length of generated Run IDs. 12
// chars from core.RandomString gives 62^12 ≈ 3.2e21 keyspace —
// collisions are statistically negligible for a single-machine
// benchmark store. Matches pkg/tasks's idLength for cross-substrate
// consistency.
const idLength = 12

// defaultListLimit caps unconstrained History queries so the Wails
// surface cannot accidentally pull the whole table when a caller
// forgets to set Filter.Limit. Tuned generous — the admin UI shows
// a fixed table window much smaller than this.
const defaultListLimit = 500
