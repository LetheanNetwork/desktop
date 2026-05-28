// SPDX-Licence-Identifier: EUPL-1.2

// Package analytics provides a DuckDB-backed read-only analytics view
// over the R₁ JSONL corpus written by pkg/r1.
//
// The JSONL files at ~/Lethean/data/r1/<model>/<subject>.jsonl remain
// the canonical store. This package layers a DuckDB
// `read_json_auto`-backed view on top so operators + the training
// dashboard can run aggregate queries without parsing every line in
// Go. No sync layer, no copy — the view reads JSONL live on every
// query.
//
// Opt-in DuckDB dependency: pkg/r1 itself stays lean (Go-only
// JSONL append/read); this subpackage is the heavier analytics
// surface that pulls in the duckdb driver.
//
// Usage example:
//
//	v := analytics.OpenView(analytics.Options{})
//	if !v.OK { return v }
//	view := v.Value.(*analytics.View)
//	defer view.Close()
//
//	r := analytics.RecordsPerSubject(analytics.Options{}, "gemma4-e2b-it-q4")
//	if r.OK {
//	    rows := r.Value.([]analytics.SubjectCount)
//	    for _, row := range rows { _ = row }
//	}
package analytics

import (
	"database/sql"

	core "dappco.re/go"
	"dappco.re/go/orm"

	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/r1"
)

// Options configures the analytics view. All fields optional.
//
// Usage example:
//
//	v := analytics.OpenView(analytics.Options{})            // production
//	v := analytics.OpenView(analytics.Options{Root: "/tmp"}) // test override
type Options struct {
	// Root overrides paths.R1Dir() for tests. Empty = production
	// default (paths.R1Dir(), which honours $HOME for the standard
	// t.Setenv("HOME", t.TempDir()) test repoint).
	Root string
}

// View is the open analytics handle. Wraps an ephemeral in-memory
// orm.DuckDBMedium with the canonical `r1` view defined against the
// on-disk JSONL corpus via read_json_auto. Caller closes via Close()
// — defer it immediately after a successful OpenView.
//
// The view is in-memory by design: read_json_auto reads the JSONL
// files live on every query, so the underlying DB carries no
// persistent state worth keeping. Sharing the app's persistent
// orm.DuckDBMedium (~/Lethean/data/tasks.duckdb) would force
// analytics queries to contend with operational reads/writes for one
// connection pool, with no upside for an inherently read-only view.
//
// Usage example:
//
//	r := analytics.OpenView(analytics.Options{})
//	if !r.OK { return r }
//	view := r.Value.(*analytics.View)
//	defer view.Close()
type View struct {
	medium *orm.DuckDBMedium
}

// Close releases the underlying DuckDB connection. Safe on a nil
// receiver.
//
// Usage example:
//
//	defer view.Close()
func (v *View) Close() core.Result {
	if v == nil || v.medium == nil {
		return core.Ok(nil)
	}
	return v.medium.Close()
}

// DB returns the underlying *sql.DB so test code can run ad-hoc
// queries against the `r1` view. Production callers should prefer
// the typed helpers (RecordsPerSubject etc) and not reach for the
// raw handle.
//
// Usage example:
//
//	rows, err := view.DB().Query("SELECT count(*) FROM r1")
func (v *View) DB() *sql.DB {
	if v == nil {
		return nil
	}
	return v.medium.Raw()
}

// SubjectCount is one row of the RecordsPerSubject result — a count
// of R₁ records grouped by (Model, Subject). Sorted by Count
// descending.
type SubjectCount struct {
	Model   string `json:"model"`
	Subject string `json:"subject"`
	Count   int64  `json:"count"`
}

// TierSubjectCount is one row of the CrossTierCounts result — a
// count of R₁ records pivoted across (Tier, Subject). Lets the
// operator see how many R₁s each cascade tier has captured for
// each rotation subject.
type TierSubjectCount struct {
	Tier    int    `json:"tier"`
	Subject string `json:"subject"`
	Count   int64  `json:"count"`
}

// FingerprintPoint is one point of a fingerprint trajectory — a
// (Timestamp, ProbeID, Value) triple plottable as a time-series.
// Returned by FingerprintTrajectory.
type FingerprintPoint struct {
	Timestamp int64   `json:"ts"`
	ProbeID   string  `json:"probe_id"`
	Value     float64 `json:"value"`
}

// OpenView opens a DuckDB connection with the canonical `r1` view
// created. The view name is "r1"; queries read `FROM r1 WHERE ...`
// as usual.
//
// When the corpus is empty (no JSONL files anywhere under the root),
// the view is created with the same column shape but zero rows so
// `SELECT count(*) FROM r1` returns 0 rather than erroring — this is
// the legitimate state during the first hour of training.
//
// Caller is responsible for closing the returned View via
// `defer view.Close()`.
//
// Usage example:
//
//	r := analytics.OpenView(analytics.Options{})
//	if !r.OK { return r }
//	view := r.Value.(*analytics.View)
//	defer view.Close()
func OpenView(opts Options) core.Result {
	rootR := resolveRoot(opts)
	if !rootR.OK {
		return rootR
	}
	root := rootR.Value.(string)

	// Open an in-memory DuckDB through orm.NewDuckDB — empty path
	// opens the duckdb driver against the special ":memory:" handle.
	// Going through orm gives one canonical DuckDB-open path
	// (ping check, error wrapping) and lets us reach the *sql.DB
	// via medium.Raw() for the non-Intent SQL below (CREATE VIEW
	// over read_json_auto). The view points at on-disk JSONL, so
	// the DB itself carries no persistent state — every OpenView
	// gets a fresh handle that re-reads the canonical corpus.
	mr := orm.NewDuckDB("")
	if !mr.OK {
		return core.Fail(core.E("analytics.OpenView", "orm.NewDuckDB: "+mr.Error(), nil))
	}
	medium := mr.Value.(*orm.DuckDBMedium)
	conn := medium.Raw()

	glob := core.PathJoin(root, "*", "*.jsonl")
	hasFiles := len(core.PathGlob(glob)) > 0

	var stmt string
	if hasFiles {
		// format='newline_delimited' + ignore_errors=true mirrors
		// pkg/r1.decodeLines tolerance: a torn record from a crashed
		// writer is skipped silently, not propagated as a panic.
		// union_by_name=true lets per-(model,subject) files with
		// different fingerprint key sets coexist in one view.
		stmt = core.Sprintf(
			"CREATE OR REPLACE VIEW r1 AS SELECT * FROM read_json_auto('%s', format='newline_delimited', ignore_errors=true, union_by_name=true)",
			escapeSQLPath(glob),
		)
	} else {
		// Empty-corpus stub: same column shape as a populated view,
		// zero rows. Queries return empty slices, no error.
		stmt = "CREATE OR REPLACE VIEW r1 AS " +
			"SELECT " +
			"NULL::VARCHAR AS model, " +
			"NULL::VARCHAR AS subject, " +
			"NULL::VARCHAR AS probe_id, " +
			"NULL::VARCHAR AS voice, " +
			"NULL::VARCHAR AS kernel, " +
			"NULL::VARCHAR AS sig, " +
			"NULL::VARCHAR AS prompt, " +
			"NULL::VARCHAR AS response, " +
			"NULL::VARCHAR AS substrate, " +
			"NULL::INTEGER AS tier, " +
			"NULL::BIGINT AS ts, " +
			"NULL::VARCHAR AS hash, " +
			"NULL::JSON AS fingerprint " +
			"WHERE FALSE"
	}

	if _, err := conn.Exec(stmt); err != nil {
		_ = medium.Close()
		return core.Fail(core.E("analytics.OpenView", "create view", err))
	}

	return core.Ok(&View{medium: medium})
}

// RecordsPerSubject returns the count of R1 records grouped by
// subject, optionally filtered to one model. Sorted by count
// descending then subject ascending for stable tie-break.
//
//	model — canonical model ID (e.g. "gemma4-e2b-it-q4"). Empty
//	        aggregates across every model in the corpus.
//
// Usage example:
//
//	r := analytics.RecordsPerSubject(analytics.Options{}, "gemma4-e2b-it-q4")
//	if r.OK {
//	    rows := r.Value.([]analytics.SubjectCount)
//	    for _, row := range rows { _ = row.Count }
//	}
func RecordsPerSubject(opts Options, model string) core.Result {
	viewR := OpenView(opts)
	if !viewR.OK {
		return viewR
	}
	view := viewR.Value.(*View)
	defer func() { _ = view.Close() }()

	var (
		rows *sql.Rows
		err  error
	)
	if model == "" {
		rows, err = view.DB().Query(
			`SELECT model, subject, count(*)::BIGINT AS n
			 FROM r1
			 WHERE model IS NOT NULL
			 GROUP BY model, subject
			 ORDER BY n DESC, subject ASC`,
		)
	} else {
		rows, err = view.DB().Query(
			`SELECT model, subject, count(*)::BIGINT AS n
			 FROM r1
			 WHERE model = ?
			 GROUP BY model, subject
			 ORDER BY n DESC, subject ASC`,
			model,
		)
	}
	if err != nil {
		return core.Fail(core.E("analytics.RecordsPerSubject", "query", err))
	}
	defer func() { _ = rows.Close() }()

	out := []SubjectCount{}
	for rows.Next() {
		var rc SubjectCount
		if err := rows.Scan(&rc.Model, &rc.Subject, &rc.Count); err != nil {
			return core.Fail(core.E("analytics.RecordsPerSubject", "scan", err))
		}
		out = append(out, rc)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("analytics.RecordsPerSubject", "rows", err))
	}
	return core.Ok(out)
}

// CrossTierCounts returns the count of R1 records pivoted by
// (tier × subject), optionally filtered to one model. Lets the
// operator see how many R₁s each cascade tier has captured for
// each rotation subject.
//
//	model — canonical model ID. Empty aggregates across every model.
//
// Usage example:
//
//	r := analytics.CrossTierCounts(analytics.Options{}, "")
//	if r.OK { _ = r.Value.([]analytics.TierSubjectCount) }
func CrossTierCounts(opts Options, model string) core.Result {
	viewR := OpenView(opts)
	if !viewR.OK {
		return viewR
	}
	view := viewR.Value.(*View)
	defer func() { _ = view.Close() }()

	var (
		rows *sql.Rows
		err  error
	)
	if model == "" {
		rows, err = view.DB().Query(
			`SELECT tier, subject, count(*)::BIGINT AS n
			 FROM r1
			 WHERE tier IS NOT NULL
			 GROUP BY tier, subject
			 ORDER BY tier ASC, n DESC, subject ASC`,
		)
	} else {
		rows, err = view.DB().Query(
			`SELECT tier, subject, count(*)::BIGINT AS n
			 FROM r1
			 WHERE model = ?
			 GROUP BY tier, subject
			 ORDER BY tier ASC, n DESC, subject ASC`,
			model,
		)
	}
	if err != nil {
		return core.Fail(core.E("analytics.CrossTierCounts", "query", err))
	}
	defer func() { _ = rows.Close() }()

	out := []TierSubjectCount{}
	for rows.Next() {
		var rc TierSubjectCount
		if err := rows.Scan(&rc.Tier, &rc.Subject, &rc.Count); err != nil {
			return core.Fail(core.E("analytics.CrossTierCounts", "scan", err))
		}
		out = append(out, rc)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("analytics.CrossTierCounts", "rows", err))
	}
	return core.Ok(out)
}

// LatestPerProbe returns the most-recent R1 record per (model,
// subject, probe_id) tuple for one (model, subject) — the canonical
// "what does the model say today" query. Older revisions of the
// same probe are dropped.
//
// Ordered by ts DESC then probe_id ASC.
//
// Usage example:
//
//	r := analytics.LatestPerProbe(analytics.Options{}, "gemma4-e2b-it-q4", "english")
//	if r.OK { _ = r.Value.([]r1.R1) }
func LatestPerProbe(opts Options, model, subject string) core.Result {
	if model == "" || subject == "" {
		return core.Fail(core.E("analytics.LatestPerProbe", "model and subject are required", nil))
	}
	viewR := OpenView(opts)
	if !viewR.OK {
		return viewR
	}
	view := viewR.Value.(*View)
	defer func() { _ = view.Close() }()

	// QUALIFY ROW_NUMBER() OVER picks the latest ts per
	// (model, subject, probe_id) within the model/subject filter.
	// DuckDB supports QUALIFY natively (since 0.7); avoids a sub-select.
	//
	// We project the whole row as JSON via to_json(r1). This is
	// resilient against the inferred schema dropping optional fields
	// (omitempty fields absent from every sampled record) — the
	// row JSON only contains keys that were actually present, and
	// the canonical R1 struct has omitempty/zero-value defaults for
	// every optional field, so the unmarshalled value is well-formed
	// regardless of which optional columns DuckDB inferred.
	rows, err := view.DB().Query(
		`SELECT CAST(to_json(r1) AS VARCHAR) AS row_json
		 FROM r1
		 WHERE model = ? AND subject = ?
		 QUALIFY ROW_NUMBER() OVER (
		   PARTITION BY model, subject, probe_id
		   ORDER BY ts DESC
		 ) = 1
		 ORDER BY ts DESC, probe_id ASC`,
		model, subject,
	)
	if err != nil {
		return core.Fail(core.E("analytics.LatestPerProbe", "query", err))
	}
	defer func() { _ = rows.Close() }()

	out := []r1.R1{}
	for rows.Next() {
		var rowJSON sql.NullString
		if err := rows.Scan(&rowJSON); err != nil {
			return core.Fail(core.E("analytics.LatestPerProbe", "scan", err))
		}
		if !rowJSON.Valid || rowJSON.String == "" {
			continue
		}
		var rec r1.R1
		if r := core.JSONUnmarshalString(rowJSON.String, &rec); !r.OK {
			// Malformed row JSON — skip; mirrors decodeLines tolerance.
			continue
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("analytics.LatestPerProbe", "rows", err))
	}

	return core.Ok(out)
}

// FingerprintTrajectory returns the time-series of one fingerprint
// dimension across all R1 records for one (model, subject). Used to
// plot, e.g., vocab_richness over time for the english subject as
// the model trains.
//
//	dim — a key from R1.Fingerprint (e.g. "vocab_richness",
//	      "tense_entropy"). Missing dim → empty slice (NOT a
//	      failure — fresh corpora may lack any given dimension).
//
// Returned points are ordered by ts ASC then probe_id ASC.
//
// Usage example:
//
//	r := analytics.FingerprintTrajectory(analytics.Options{},
//	    "gemma4-e2b-it-q4", "english", "vocab_richness")
//	if r.OK { _ = r.Value.([]analytics.FingerprintPoint) }
func FingerprintTrajectory(opts Options, model, subject, dim string) core.Result {
	if model == "" || subject == "" {
		return core.Fail(core.E("analytics.FingerprintTrajectory", "model and subject are required", nil))
	}
	if dim == "" {
		return core.Fail(core.E("analytics.FingerprintTrajectory", "dim is required", nil))
	}
	viewR := OpenView(opts)
	if !viewR.OK {
		return viewR
	}
	view := viewR.Value.(*View)
	defer func() { _ = view.Close() }()

	// Project the whole row as JSON, then extract fingerprint.<dim>
	// from it. Going via to_json(r1) avoids a Binder error when
	// the inferred schema lacks a `fingerprint` column entirely
	// (no record in the sampled set populated it). try_cast on the
	// extracted value tolerates missing keys / non-numeric values —
	// returns NULL, which the WHERE filter drops. The path is built
	// as $.fingerprint.<dim> with the dim escaped for the JSON path
	// grammar; the surrounding string is single-quote escaped for SQL.
	path := "$.fingerprint." + escapeJSONIdent(dim)
	stmt := `SELECT ts, probe_id,
	         try_cast(json_extract_string(to_json(r1), '` + escapeSQLLiteral(path) + `') AS DOUBLE) AS val
	         FROM r1
	         WHERE model = ? AND subject = ?
	           AND try_cast(json_extract_string(to_json(r1), '` + escapeSQLLiteral(path) + `') AS DOUBLE) IS NOT NULL
	         ORDER BY ts ASC, probe_id ASC`

	rows, err := view.DB().Query(stmt, model, subject)
	if err != nil {
		return core.Fail(core.E("analytics.FingerprintTrajectory", "query", err))
	}
	defer func() { _ = rows.Close() }()

	out := []FingerprintPoint{}
	for rows.Next() {
		var p FingerprintPoint
		if err := rows.Scan(&p.Timestamp, &p.ProbeID, &p.Value); err != nil {
			return core.Fail(core.E("analytics.FingerprintTrajectory", "scan", err))
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return core.Fail(core.E("analytics.FingerprintTrajectory", "rows", err))
	}
	return core.Ok(out)
}

// resolveRoot returns the corpus root: opts.Root if set, otherwise
// paths.R1Dir(). Centralised so every public function honours the
// same override semantics.
func resolveRoot(opts Options) core.Result {
	if opts.Root != "" {
		return core.Ok(opts.Root)
	}
	return paths.R1Dir()
}

// escapeSQLPath escapes single quotes in a file path for use in a
// DuckDB SQL string literal. Mirrors external/store/go's
// escapeSQLPath helper; duplicated here to keep this package
// dependency-light.
func escapeSQLPath(p string) string {
	return core.Replace(p, "'", "''")
}

// escapeSQLLiteral escapes single quotes in any string interpolated
// into a SQL literal. Same primitive as escapeSQLPath — split by
// callsite intent so each callsite documents what it's escaping.
func escapeSQLLiteral(s string) string {
	return core.Replace(s, "'", "''")
}

// escapeJSONIdent escapes a fingerprint dim name for use inside a
// JSON path expression. Quotes inside the dim become JSON-escaped
// quotes; backslashes become escaped backslashes. Dims that contain
// dots stay literal (we use $.dim form, which selects an exact key).
func escapeJSONIdent(s string) string {
	s = core.Replace(s, "\\", "\\\\")
	s = core.Replace(s, "\"", "\\\"")
	return s
}
