// SPDX-Licence-Identifier: EUPL-1.2

// Service — Core integration for the forecast surface. Derives ForecastRow
// and ForecastKpi values by scanning ~/Lethean/sales/deals/ at call time.
// No persistence file — forecast is computed on demand.
//
// Lifecycle:
//   - Register(c)   wires the service into the Core container
//   - ServiceName() returns "Forecast" for the Wails namespace
//
// All I/O uses CoreGO wrappers. Banned stdlib imports: os, path/filepath,
// strings, encoding/json, fmt, log, errors.

package forecast

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"gopkg.in/yaml.v3"
)

// Service owns the forecast surface.
//
// Usage example:
//
//	svc := forecast.NewService(c)
type Service struct {
	core *core.Core
}

// NewService constructs the forecast service against a Core container.
//
// Usage example:
//
//	svc := forecast.NewService(c)
func NewService(c *core.Core) *Service {
	return &Service{core: c}
}

// Register constructs the forecast service for Core registration.
//
// Usage example:
//
//	core.New(core.WithName("sales-forecast", forecast.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// ServiceName labels the binding namespace exposed to JS.
// Wails binds methods as "Forecast.Quarterly()" etc.
func (s *Service) ServiceName() string { return "Forecast" }

// dealsDir resolves ~/Lethean/sales/deals/ and creates it if missing.
// Mode 0o700 (Cerberus #1487 PR-1): mirrors pkg/sales/deals — same
// directory, same sensitive contents, same permission posture.
func dealsDir() core.Result {
	root := paths.Root()
	if !root.OK {
		return root
	}
	dir := core.PathJoin(root.Value.(string), "sales", "deals")
	if r := core.MkdirAll(dir, 0o700); !r.OK {
		return r
	}
	return core.Ok(dir)
}

// dealFM is the minimal frontmatter shape for forecast computation.
type dealFM struct {
	AmountPence    int    `yaml:"amount_pence"`
	Stage          string `yaml:"stage"`
	CloseTarget    string `yaml:"close_target"`
	ProbabilityPct int    `yaml:"probability_pct"`
}

// parseFM extracts only the frontmatter from a Trix file.
func parseFM(raw []byte) (dealFM, error) {
	content := raw
	open := []byte("---\n")
	if len(content) >= len(open) {
		match := true
		for i, b := range open {
			if content[i] != b {
				match = false
				break
			}
		}
		if match {
			content = content[len(open):]
		}
	}
	closeIdx := -1
	for i := 0; i < len(content)-3; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			if i == 0 || content[i-1] == '\n' {
				closeIdx = i
				break
			}
		}
	}
	fmBytes := content
	if closeIdx >= 0 {
		fmBytes = content[:closeIdx]
	}
	var fm dealFM
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return dealFM{}, core.E("forecast.parseFM", "yaml unmarshal", err)
	}
	return fm, nil
}

// loadAllDeals scans ~/Lethean/sales/deals/ and returns DealSummary values.
func loadAllDeals() ([]DealSummary, error) {
	dirR := dealsDir()
	if !dirR.OK {
		return nil, core.E("forecast.loadAllDeals", dirR.Error(), nil)
	}
	dir := dirR.Value.(string)

	entriesR := core.ReadDir(core.DirFS(dir), ".")
	if !entriesR.OK {
		return nil, nil
	}
	entries, _ := entriesR.Value.([]core.FsDirEntry)

	var summaries []DealSummary
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) < 4 || name[len(name)-3:] != ".md" {
			continue
		}
		raw := core.ReadFile(core.PathJoin(dir, name))
		if !raw.OK {
			continue
		}
		fm, err := parseFM(raw.Value.([]byte))
		if err != nil {
			continue
		}
		summaries = append(summaries, DealSummary{
			AmountPence:    fm.AmountPence,
			Stage:          fm.Stage,
			CloseTarget:    fm.CloseTarget,
			ProbabilityPct: fm.ProbabilityPct,
		})
	}
	return summaries, nil
}

// stageWeight returns the default probability weight for a stage (0-1).
// The frontier weights match the footer copy in forecast.ts:
// "best case = committed + 60% engaged + 30% proposal"
// — but we use more nuanced defaults across all stages.
func stageWeight(stage string) float64 {
	switch stage {
	case "qual":
		return 0.10
	case "engage":
		return 0.30
	case "propose":
		return 0.60
	case "close":
		return 0.85
	case "won":
		return 1.00
	case "lost":
		return 0.00
	default:
		return 0.20
	}
}

// formatGBPK formats integer pence as "£N K" (above 1000p) or "£N".
func formatGBPK(pence int) string {
	if pence >= 1000 {
		k := (pence + 500) / 1000
		return core.Sprintf("£%d K", k)
	}
	return core.Sprintf("£%d", pence/100)
}

// currentQuarter returns the year and 1-based quarter (1-4) of now.
func currentQuarter(now core.Time) (year int, quarter int) {
	t := now.UTC()
	mo := int(core.TimeFormat(t, "01")[0]-'0')*10 + int(core.TimeFormat(t, "01")[1]-'0')
	yr := 0
	// Parse year from format string: "2006" → 4 digits.
	yStr := core.TimeFormat(t, "2006")
	for _, ch := range []byte(yStr) {
		yr = yr*10 + int(ch-'0')
	}
	q := (mo-1)/3 + 1
	return yr, q
}

// quarterLabel returns a display label like "Q2 · 2026" for a given
// year + 1-based quarter.
func quarterLabel(year, quarter int) string {
	return core.Sprintf("Q%d · %d", quarter, year)
}

// assignToQuarter returns which quarter offset (0 = current) a deal
// maps to based on CloseTarget. Deals without a parseable close target
// land in offset 0 (current quarter).
// v1: month-name parse ("Jun" → month 6). Quarter = (month-1)/3+1.
func assignToQuarter(d DealSummary, curYear, curQtr int) int {
	if d.CloseTarget == "" {
		return 0
	}
	// CloseTarget format: "14 Jun" — extract month name (last word).
	ct := []byte(d.CloseTarget)
	// Find last space.
	lastSpace := -1
	for i := len(ct) - 1; i >= 0; i-- {
		if ct[i] == ' ' {
			lastSpace = i
			break
		}
	}
	monthName := d.CloseTarget
	if lastSpace >= 0 {
		monthName = string(ct[lastSpace+1:])
	}

	// Map month abbreviations to 1-12.
	monthMap := map[string]int{
		"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4,
		"May": 5, "Jun": 6, "Jul": 7, "Aug": 8,
		"Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12,
	}
	mo, ok := monthMap[monthName]
	if !ok {
		return 0
	}
	dealQtr := (mo-1)/3 + 1

	// Use current year for quarter assignment (v1 simplification —
	// a Dec close target in Q1 of the next year is a follow-up).
	offset := (dealQtr - curQtr)
	if offset < 0 {
		// Already passed — include in current quarter.
		return 0
	}
	return offset
}

// buildRows derives the n-quarter forecast from deal summaries.
func buildRows(summaries []DealSummary, n int, now core.Time) []ForecastRow {
	curYear, curQtr := currentQuarter(now)

	type qAcc struct {
		committedPence int
		bestPence      int
		lowPence       int
	}
	accs := make([]qAcc, n)

	for _, d := range summaries {
		offset := assignToQuarter(d, curYear, curQtr)
		if offset >= n {
			continue // beyond the forecast window
		}
		w := stageWeight(d.Stage)
		pct := d.ProbabilityPct
		if pct != 0 {
			w = float64(pct) / 100.0
		}
		switch d.Stage {
		case "won":
			// Won = already committed. Added to committed only;
			// best + low accumulate separately on top of committed.
			accs[offset].committedPence += d.AmountPence
		case "lost":
			// Excluded from all projections.
		default:
			// Best = 60% of engage, 100% of propose/close.
			bestW := w
			if d.Stage == "engage" {
				bestW = 0.60
			}
			accs[offset].bestPence += int(float64(d.AmountPence) * bestW)

			// Low = 30% of engage only; other active stages excluded from low.
			if d.Stage == "engage" {
				accs[offset].lowPence += int(float64(d.AmountPence) * 0.30)
			}
		}
	}

	rows := make([]ForecastRow, n)
	for i := range rows {
		yr := curYear + (curQtr+i-1)/4
		qtr := (curQtr+i-1)%4 + 1
		status := "open"
		if i > 0 {
			status = "plan"
		}
		// Add committed to best + low (accumulated separately above —
		// committed is already the "won" slice).
		committed := (accs[i].committedPence + 500) / 1000
		best := (accs[i].committedPence + accs[i].bestPence + 500) / 1000
		low := (accs[i].committedPence + accs[i].lowPence + 500) / 1000
		rows[i] = ForecastRow{
			Q:         quarterLabel(yr, qtr),
			Target:    0, // v1: targets not yet configurable; 0 = no target set
			Committed: committed,
			Best:      best,
			Low:       low,
			Status:    status,
		}
	}
	return rows
}

// buildKpis derives the 4 headline KPI cards from the rows + raw summaries.
func buildKpis(rows []ForecastRow, summaries []DealSummary) []ForecastKpi {
	// 1. Y1 target ARR — sum of all row targets (v1: always £0 until
	//    per-quarter targets ship; leave blank to avoid confusion).
	targetSum := 0
	for _, r := range rows {
		targetSum += r.Target
	}
	// 2. Committed — sum of committed across all rows.
	committedSum := 0
	for _, r := range rows {
		committedSum += r.Committed
	}
	// 3. Best case — sum of best across all rows.
	bestSum := 0
	for _, r := range rows {
		bestSum += r.Best
	}
	// 4. Probability-weighted — each deal × stage weight, across all deals.
	probWeightedPence := 0
	for _, d := range summaries {
		w := stageWeight(d.Stage)
		if d.ProbabilityPct != 0 {
			w = float64(d.ProbabilityPct) / 100.0
		}
		probWeightedPence += int(float64(d.AmountPence) * w)
	}
	probWeightedK := (probWeightedPence + 500) / 1000

	return []ForecastKpi{
		{L: "Y1 target ARR", V: formatGBPK(targetSum * 1000)},
		{L: "Committed", V: core.Sprintf("£%d K", committedSum)},
		{L: "Best case", V: core.Sprintf("£%d K", bestSum)},
		{L: "Probability-weighted", V: core.Sprintf("£%d K", probWeightedK)},
	}
}
