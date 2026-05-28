// SPDX-Licence-Identifier: EUPL-1.2

package r1

import (
	"testing"

	core "dappco.re/go"
)

// tempCorpus repoints ~ to t.TempDir so paths.R1Dir() resolves under
// a per-test root. Mirrors the pattern in pkg/sessions tests.
func tempCorpus(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func newRec(model, subject, probe string) R1 {
	return R1{
		Model:     model,
		Subject:   subject,
		ProbeID:   probe,
		Substrate: "CONT",
		Tier:      0,
		Prompt:    "kernel + probe + sig",
		Response:  "model produced this R₁",
	}
}

// --- Write ---

func TestWrite_Good(t *testing.T) {
	tempCorpus(t)
	r := newRec("gemma4-e2b-it-q4", "english", "P01_TEST")
	rr := Write(r)
	if !rr.OK {
		t.Fatalf("Write failed: %v", rr.Value)
	}
}

func TestWrite_BadMissingModel(t *testing.T) {
	tempCorpus(t)
	r := newRec("", "english", "P01_TEST")
	if rr := Write(r); rr.OK {
		t.Error("Write with empty Model returned OK, want failure")
	}
}

func TestWrite_BadMissingSubject(t *testing.T) {
	tempCorpus(t)
	r := newRec("gemma4-e2b-it-q4", "", "P01_TEST")
	if rr := Write(r); rr.OK {
		t.Error("Write with empty Subject returned OK, want failure")
	}
}

func TestWrite_BadMissingProbe(t *testing.T) {
	tempCorpus(t)
	r := newRec("gemma4-e2b-it-q4", "english", "")
	if rr := Write(r); rr.OK {
		t.Error("Write with empty ProbeID returned OK, want failure")
	}
}

func TestWrite_UglyEnrichesTimestampAndHash(t *testing.T) {
	tempCorpus(t)
	r := newRec("gemma4-e2b-it-q4", "english", "P01")
	if rr := Write(r); !rr.OK {
		t.Fatalf("Write failed: %v", rr.Value)
	}
	res := Read(Filter{Model: "gemma4-e2b-it-q4", Subject: "english"})
	if !res.OK {
		t.Fatalf("Read failed: %v", res.Value)
	}
	recs := res.Value.([]R1)
	if len(recs) != 1 {
		t.Fatalf("Read after Write returned %d records, want 1", len(recs))
	}
	if recs[0].Timestamp == 0 {
		t.Error("Timestamp not auto-populated")
	}
	if recs[0].Hash == "" {
		t.Error("Hash not auto-populated")
	}
	if len(recs[0].Hash) != 64 {
		t.Errorf("Hash length = %d, want 64 (SHA-256 hex)", len(recs[0].Hash))
	}
}

// --- Read / Filter ---

func TestRead_GoodRoundtrip(t *testing.T) {
	tempCorpus(t)
	in := newRec("gemma4-e2b-it-q4", "english", "P01")
	in.Response = "specific R₁ text"
	in.Voice = "founder"
	if rr := Write(in); !rr.OK {
		t.Fatalf("Write failed: %v", rr.Value)
	}

	res := Read(Filter{Model: "gemma4-e2b-it-q4"})
	if !res.OK {
		t.Fatalf("Read failed: %v", res.Value)
	}
	recs := res.Value.([]R1)
	if len(recs) != 1 {
		t.Fatalf("Read returned %d records, want 1", len(recs))
	}
	if recs[0].Response != in.Response {
		t.Errorf("Response mismatch: %q vs %q", recs[0].Response, in.Response)
	}
	if recs[0].Voice != "founder" {
		t.Errorf("Voice mismatch: %q vs founder", recs[0].Voice)
	}
}

func TestRead_GoodFilterDiscriminates(t *testing.T) {
	tempCorpus(t)

	writes := []R1{
		newRec("gemma4-e2b-it-q4", "english", "P01"),
		newRec("gemma4-e2b-it-q4", "english", "P02"),
		newRec("gemma4-e2b-it-q4", "chinese", "P01"),
		newRec("gemma4-e4b-it-q4", "english", "P01"),
	}
	for _, r := range writes {
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}

	cases := []struct {
		name   string
		filter Filter
		want   int
	}{
		{"all", Filter{}, 4},
		{"model only", Filter{Model: "gemma4-e2b-it-q4"}, 3},
		{"subject only", Filter{Subject: "english"}, 3},
		{"model + subject", Filter{Model: "gemma4-e2b-it-q4", Subject: "english"}, 2},
		{"model + subject + probe", Filter{Model: "gemma4-e2b-it-q4", Subject: "english", ProbeID: "P01"}, 1},
		{"non-matching model", Filter{Model: "gemma4-31b-it-q4"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Read(c.filter)
			if !res.OK {
				t.Fatalf("Read failed: %v", res.Value)
			}
			recs := res.Value.([]R1)
			if len(recs) != c.want {
				t.Errorf("filter %+v returned %d records, want %d", c.filter, len(recs), c.want)
			}
		})
	}
}

func TestRead_GoodLimitCaps(t *testing.T) {
	tempCorpus(t)
	for i := 0; i < 5; i++ {
		r := newRec("gemma4-e2b-it-q4", "english", "P0"+string(rune('1'+i)))
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}
	res := Read(Filter{Model: "gemma4-e2b-it-q4", Limit: 3})
	recs := res.Value.([]R1)
	if len(recs) != 3 {
		t.Errorf("Limit=3 returned %d records, want 3", len(recs))
	}
}

func TestRead_UglyEmptyCorpus(t *testing.T) {
	tempCorpus(t)
	res := Read(Filter{})
	if !res.OK {
		t.Fatalf("Read on empty corpus failed: %v", res.Value)
	}
	recs := res.Value.([]R1)
	if len(recs) != 0 {
		t.Errorf("empty corpus returned %d records, want 0", len(recs))
	}
}

// --- ListModels / ListSubjects / ListProbes ---

func TestListModels_Good(t *testing.T) {
	tempCorpus(t)
	for _, m := range []string{"gemma4-e2b-it-q4", "gemma4-e4b-it-q4"} {
		r := newRec(m, "english", "P01")
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}
	res := ListModels()
	if !res.OK {
		t.Fatalf("ListModels failed: %v", res.Value)
	}
	models := res.Value.([]string)
	if len(models) != 2 {
		t.Errorf("ListModels returned %d, want 2: %v", len(models), models)
	}
}

func TestListSubjects_Good(t *testing.T) {
	tempCorpus(t)
	for _, s := range []string{"english", "chinese", "european"} {
		r := newRec("gemma4-e2b-it-q4", s, "P01")
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}
	res := ListSubjects("gemma4-e2b-it-q4")
	if !res.OK {
		t.Fatalf("ListSubjects failed: %v", res.Value)
	}
	subjects := res.Value.([]string)
	if len(subjects) != 3 {
		t.Errorf("ListSubjects returned %d, want 3: %v", len(subjects), subjects)
	}
}

func TestListSubjects_BadMissingModel(t *testing.T) {
	tempCorpus(t)
	if res := ListSubjects(""); res.OK {
		t.Error("ListSubjects(\"\") returned OK, want failure")
	}
}

func TestListProbes_Good(t *testing.T) {
	tempCorpus(t)
	for _, p := range []string{"P01", "P02", "P03"} {
		r := newRec("gemma4-e2b-it-q4", "english", p)
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}
	// Duplicate write of P01 should NOT inflate the probe count.
	dup := newRec("gemma4-e2b-it-q4", "english", "P01")
	dup.Response = "different response, same probe id"
	if rr := Write(dup); !rr.OK {
		t.Fatalf("dup Write failed: %v", rr.Value)
	}
	res := ListProbes("gemma4-e2b-it-q4", "english")
	if !res.OK {
		t.Fatalf("ListProbes failed: %v", res.Value)
	}
	probes := res.Value.([]string)
	if len(probes) != 3 {
		t.Errorf("ListProbes returned %d distinct probes, want 3: %v", len(probes), probes)
	}
}

func TestListProbes_UglyEmpty(t *testing.T) {
	tempCorpus(t)
	res := ListProbes("nonexistent", "subject")
	if !res.OK {
		t.Fatalf("ListProbes on missing file failed: %v", res.Value)
	}
	probes := res.Value.([]string)
	if len(probes) != 0 {
		t.Errorf("ListProbes on missing returned %d, want 0", len(probes))
	}
}

// --- Hash dedup behaviour ---

func TestWrite_GoodHashDeterministicForSameResponse(t *testing.T) {
	tempCorpus(t)
	a := newRec("gemma4-e2b-it-q4", "english", "P01")
	a.Response = "identical text"
	b := newRec("gemma4-e2b-it-q4", "english", "P02")
	b.Response = "identical text"

	if rr := Write(a); !rr.OK {
		t.Fatalf("Write a failed: %v", rr.Value)
	}
	if rr := Write(b); !rr.OK {
		t.Fatalf("Write b failed: %v", rr.Value)
	}
	res := Read(Filter{Model: "gemma4-e2b-it-q4", Subject: "english"})
	recs := res.Value.([]R1)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if recs[0].Hash != recs[1].Hash {
		t.Errorf("identical Response produced different hashes: %q vs %q",
			recs[0].Hash, recs[1].Hash)
	}
}

// --- Filter.matches direct unit ---

func TestFilterMatches_Good(t *testing.T) {
	rec := R1{Model: "m", Subject: "s", ProbeID: "p"}
	cases := []struct {
		filter Filter
		want   bool
	}{
		{Filter{}, true},
		{Filter{Model: "m"}, true},
		{Filter{Model: "other"}, false},
		{Filter{Subject: "s"}, true},
		{Filter{ProbeID: "p"}, true},
		{Filter{Model: "m", Subject: "s", ProbeID: "p"}, true},
		{Filter{Model: "m", Subject: "wrong"}, false},
	}
	for _, c := range cases {
		got := c.filter.matches(rec)
		if got != c.want {
			t.Errorf("matches(%+v) = %v, want %v", c.filter, got, c.want)
		}
	}
}

// --- Sanity check that R1Dir resolves where we expect ---

func TestR1Dir_ResolvedUnderTempHome(t *testing.T) {
	tempCorpus(t)
	rec := newRec("gemma4-e2b-it-q4", "english", "P01")
	if rr := Write(rec); !rr.OK {
		t.Fatalf("Write failed: %v", rr.Value)
	}
	// After Write, listing models should find our model dir.
	res := ListModels()
	models := res.Value.([]string)
	found := false
	for _, m := range models {
		if m == "gemma4-e2b-it-q4" {
			found = true
		}
	}
	if !found {
		t.Errorf("model dir not found after Write; got models %v", models)
	}
}

// --- Tier / MaxTier filter (RFC.fork-tree.md §7 cascade dimension) ---

// ptr returns the address of i — sugar for building Filter.Tier /
// Filter.MaxTier in table-driven tests.
func ptr(i int) *int { return &i }

// newRecTier mirrors newRec but lets the caller pin the tier value.
func newRecTier(model, subject, probe string, tier int) R1 {
	r := newRec(model, subject, probe)
	r.Tier = tier
	return r
}

func TestRead_GoodTierExactFilter(t *testing.T) {
	tempCorpus(t)
	writes := []R1{
		newRecTier("gemma4-e2b-it-q4", "english", "P01", 0),
		newRecTier("gemma4-e4b-it-q4", "english", "P01", 1),
		newRecTier("gemma4-26b-q4", "english", "P01", 2),
	}
	for _, r := range writes {
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}

	res := Read(Filter{Tier: ptr(1)})
	if !res.OK {
		t.Fatalf("Read failed: %v", res.Value)
	}
	recs := res.Value.([]R1)
	if len(recs) != 1 {
		t.Fatalf("Tier=1 filter returned %d records, want 1", len(recs))
	}
	if recs[0].Tier != 1 {
		t.Errorf("returned record has Tier=%d, want 1", recs[0].Tier)
	}
}

func TestRead_GoodMaxTierFilter(t *testing.T) {
	tempCorpus(t)
	writes := []R1{
		newRecTier("gemma4-e2b-it-q4", "english", "P01", 0),
		newRecTier("gemma4-e4b-it-q4", "english", "P01", 1),
		newRecTier("gemma4-26b-q4", "english", "P01", 2),
	}
	for _, r := range writes {
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}

	res := Read(Filter{MaxTier: ptr(1)})
	if !res.OK {
		t.Fatalf("Read failed: %v", res.Value)
	}
	recs := res.Value.([]R1)
	if len(recs) != 2 {
		t.Fatalf("MaxTier=1 returned %d records, want 2 (tiers 0 and 1)", len(recs))
	}
	for _, r := range recs {
		if r.Tier > 1 {
			t.Errorf("MaxTier=1 returned a record with Tier=%d", r.Tier)
		}
	}
}

func TestRead_GoodTierAndOtherFilters(t *testing.T) {
	tempCorpus(t)
	writes := []R1{
		newRecTier("gemma4-e2b-it-q4", "english", "P01", 0),
		newRecTier("gemma4-e2b-it-q4", "english", "P02", 0),
		newRecTier("gemma4-e2b-it-q4", "chinese", "P01", 0),
		newRecTier("gemma4-e4b-it-q4", "english", "P01", 1),
		newRecTier("gemma4-26b-q4", "english", "P01", 2),
	}
	for _, r := range writes {
		if rr := Write(r); !rr.OK {
			t.Fatalf("setup Write failed: %v", rr.Value)
		}
	}

	// MaxTier=1 + Model + Subject — narrows to E2B and E4B english
	// records under the named model. Only the E2B english pair
	// survives (E4B is a different model dir).
	res := Read(Filter{
		Model:   "gemma4-e2b-it-q4",
		Subject: "english",
		MaxTier: ptr(1),
	})
	if !res.OK {
		t.Fatalf("Read failed: %v", res.Value)
	}
	recs := res.Value.([]R1)
	if len(recs) != 2 {
		t.Fatalf("combined filter returned %d records, want 2", len(recs))
	}
	for _, r := range recs {
		if r.Model != "gemma4-e2b-it-q4" || r.Subject != "english" || r.Tier > 1 {
			t.Errorf("unexpected record under combined filter: %+v", r)
		}
	}
}

func TestFilterMatches_TierAndMaxTier(t *testing.T) {
	rec0 := R1{Model: "m", Subject: "s", ProbeID: "p", Tier: 0}
	rec1 := R1{Model: "m", Subject: "s", ProbeID: "p", Tier: 1}
	rec2 := R1{Model: "m", Subject: "s", ProbeID: "p", Tier: 2}

	cases := []struct {
		name   string
		filter Filter
		rec    R1
		want   bool
	}{
		// nil pointers — pre-existing behaviour, no tier filtering.
		{"nil-nil matches tier 0", Filter{}, rec0, true},
		{"nil-nil matches tier 2", Filter{}, rec2, true},

		// Tier exact match.
		{"Tier=1 matches tier 1", Filter{Tier: ptr(1)}, rec1, true},
		{"Tier=1 rejects tier 0", Filter{Tier: ptr(1)}, rec0, false},
		{"Tier=1 rejects tier 2", Filter{Tier: ptr(1)}, rec2, false},

		// MaxTier inclusive upper bound.
		{"MaxTier=1 matches tier 0", Filter{MaxTier: ptr(1)}, rec0, true},
		{"MaxTier=1 matches tier 1", Filter{MaxTier: ptr(1)}, rec1, true},
		{"MaxTier=1 rejects tier 2", Filter{MaxTier: ptr(1)}, rec2, false},

		// Both set — compose (AND). A contradictory pair returns false.
		{"Tier=1 + MaxTier=1 matches tier 1", Filter{Tier: ptr(1), MaxTier: ptr(1)}, rec1, true},
		{"Tier=1 + MaxTier=0 rejects tier 1", Filter{Tier: ptr(1), MaxTier: ptr(0)}, rec1, false},
		{"Tier=0 + MaxTier=1 matches tier 0", Filter{Tier: ptr(0), MaxTier: ptr(1)}, rec0, true},

		// Tier filter composes with the existing string filters.
		{"Model + Tier=0 matches", Filter{Model: "m", Tier: ptr(0)}, rec0, true},
		{"Model mismatch overrides Tier match", Filter{Model: "other", Tier: ptr(0)}, rec0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.filter.matches(c.rec); got != c.want {
				t.Errorf("matches(%+v, Tier=%d) = %v, want %v",
					c.filter, c.rec.Tier, got, c.want)
			}
		})
	}
}

// TestWrite_Good_LargeRecord — a real R₁ carries the LEK-1 sandwich
// plus a model response plus a Fingerprint map and routinely exceeds
// PIPE_BUF (Darwin 512 B, Linux 4096 B). Before the swap to
// paths.AtomicAppendLineLarge, every production R₁ write on macOS
// failed with paths.append.record_too_large. This test pins the new
// contract: a 64 KiB Response writes cleanly and round-trips via Read
// with Hash intact.
func TestWrite_Good_LargeRecord(t *testing.T) {
	tempCorpus(t)

	// 64 KiB Response — well above Linux PIPE_BUF (4096) and far above
	// Darwin PIPE_BUF (512). With the previous AtomicAppendLine call,
	// this would fail on every supported platform.
	const size = 64 * 1024
	big := make([]byte, size)
	for i := range big {
		big[i] = 'A' + byte(i%26)
	}

	in := newRec("gemma4-e2b-it-q4", "english", "P_LARGE")
	in.Response = string(big)

	rr := Write(in)
	if !rr.OK {
		t.Fatalf("Write of 64 KiB record failed: %v", rr.Value)
	}

	res := Read(Filter{Model: "gemma4-e2b-it-q4", Subject: "english"})
	if !res.OK {
		t.Fatalf("Read after large Write failed: %v", res.Value)
	}
	recs := res.Value.([]R1)
	if len(recs) != 1 {
		t.Fatalf("Read returned %d records, want 1", len(recs))
	}
	if len(recs[0].Response) != size {
		t.Errorf("Response length = %d, want %d", len(recs[0].Response), size)
	}
	if recs[0].Response != in.Response {
		t.Error("Response content mismatch after large Write round-trip")
	}
	if recs[0].Hash == "" || len(recs[0].Hash) != 64 {
		t.Errorf("Hash malformed: len=%d", len(recs[0].Hash))
	}
}

// silence unused import warning if core ever becomes unused
var _ = core.Ok
