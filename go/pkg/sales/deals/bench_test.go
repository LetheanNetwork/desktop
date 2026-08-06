// SPDX-Licence-Identifier: EUPL-1.2

// bench_test.go — read-lane instrument for pkg/sales/deals (perf
// campaign, lane/perf-c). Targets loadAll — the scan every deals pane
// re-issues on open (List's sole data source) — at a realistic
// 100-record collection size, hermetic via t.TempDir + b.Setenv
// ("HOME"), -benchmem, -benchtime=20x, steady-state confirmed.
//
// Two fixture shapes are benched, both real production code paths:
//
//   - Lthn100: 100 encrypted .lthn records. loadAll's header-only
//     decode (DecodeHeader) does MAC verification against a PUBLIC
//     key only — no decrypt, no KDF. The keypair is generated ONCE
//     via sync.Once (mirrors deals_test.go's genTestKeyPair) and
//     reused for every fixture write AND every benchmark iteration —
//     the benchmark measures the decode/scan plumbing at production
//     cost, never a passphrase-derivation cost that doesn't belong in
//     this read path at all.
//
//   - MD100: 100 legacy plaintext .md records. loadAll's fallback
//     path fully parses each file's YAML frontmatter AND builds the
//     Notes (body) string via parseRecord, then throws Notes away
//     (rec.Notes = "") because List never shows it.
//
// This file lives in package deals (white-box) because loadAll is
// unexported.
package deals

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/account"
	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// benchGate satisfies SessionGate + the accountKeyProvider runtime
// assertion (mirrors deals_test.go's stubSessionGate, duplicated here
// because that type lives in the external deals_test package and
// loadAll is only reachable from this white-box package).
type benchGate struct {
	ids  []string
	pub  []byte
	priv []byte
}

func (g *benchGate) UnlockedAccountIDs() []string { return g.ids }

func (g *benchGate) PublicKeyFor(_ string) ([]byte, bool) {
	if len(g.pub) == 0 {
		return nil, false
	}
	cp := make([]byte, len(g.pub))
	copy(cp, g.pub)
	return cp, true
}

func (g *benchGate) PrivateKeyFor(_ string) (*account.PrivateKeyHandle, bool) {
	if len(g.priv) == 0 {
		return nil, false
	}
	cp := make([]byte, len(g.priv))
	copy(cp, g.priv)
	return account.NewPrivateKeyHandleForTest(cp), true
}

// benchMinimalGate satisfies ONLY SessionGate (no key methods) so
// atrestWriterFor's runtime type-assertion fails and writeRecord
// falls through to the legacy plaintext .md writer — mirrors
// legacy_write_test.go's minimalSessionGate.
type benchMinimalGate struct{ ids []string }

func (g *benchMinimalGate) UnlockedAccountIDs() []string { return g.ids }

// benchKeyPair lazily generates a real PGP keypair ONCE per process
// (sync.Once) and caches it — the "pre-derived/cheapest-legitimate
// keys" fixture the campaign brief requires: real key material, but
// the costly generation happens once, outside every benchmark's timed
// loop, never repeated at production KDF cost.
var (
	benchKeyOnce sync.Once
	benchKeyPub  []byte
	benchKeyPriv []byte
)

func benchKeyPair(b *testing.B) (pub, priv []byte) {
	b.Helper()
	benchKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Bench", "bench@lthn.local", "bench")
		if err != nil {
			b.Fatalf("generate bench key pair: %v", err)
		}
		benchKeyPub = p
		benchKeyPriv = k
	})
	return benchKeyPub, benchKeyPriv
}

// seedLthnRecords wires a Service with a real (cached) keypair, writes
// n encrypted .lthn deal records via the production Create path, and
// returns the wired Service. Setup runs before the benchmark's timer
// starts — 100 real PGP encryptions is real one-time cost, not a
// per-iteration one.
func seedLthnRecords(b *testing.B, n int) *Service {
	b.Helper()
	home := b.TempDir()
	b.Setenv("HOME", home)

	pub, priv := benchKeyPair(b)
	svc := NewService(nil)
	svc.SetSessionGate(&benchGate{ids: []string{"acct-bench"}, pub: pub, priv: priv})

	for i := 0; i < n; i++ {
		r := svc.Create(CreateInput{
			Customer:    core.Sprintf("Bench Customer %03d", i),
			AmountPence: 24000 + i,
			Stage:       "engage",
			Owner:       "Bench",
		})
		if !r.OK {
			b.Fatalf("seedLthnRecords: Create[%d]: %s", i, r.Error())
		}
	}
	return svc
}

// seedMdRecords wires a Service with the minimal (no-key) gate, writes
// n legacy plaintext .md deal records via the production Create path
// (writeRecord's legacy fallback engages because the gate doesn't
// satisfy accountKeyProvider), and returns the wired Service.
func seedMdRecords(b *testing.B, n int) *Service {
	b.Helper()
	home := b.TempDir()
	b.Setenv("HOME", home)

	svc := NewService(nil)
	svc.SetSessionGate(&benchMinimalGate{ids: []string{"acct-legacy"}})

	for i := 0; i < n; i++ {
		r := svc.Create(CreateInput{
			Customer:    core.Sprintf("Bench Customer %03d", i),
			AmountPence: 24000 + i,
			Stage:       "engage",
			Owner:       "Bench",
		})
		if !r.OK {
			b.Fatalf("seedMdRecords: Create[%d]: %s", i, r.Error())
		}
	}
	return svc
}

// BenchmarkLoadAll_Lthn100 — 100 encrypted records, header-only decode
// per record (MAC verify against a public key; no decrypt, no KDF).
func BenchmarkLoadAll_Lthn100(b *testing.B) {
	svc := seedLthnRecords(b, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		records, err := svc.loadAll()
		if err != nil {
			b.Fatalf("loadAll: %v", err)
		}
		if len(records) != 100 {
			b.Fatalf("loadAll: got %d records, want 100", len(records))
		}
	}
}

// BenchmarkLoadAll_MD100 — 100 legacy plaintext records, full YAML
// frontmatter + body parse per record via parseRecord.
func BenchmarkLoadAll_MD100(b *testing.B) {
	svc := seedMdRecords(b, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		records, err := svc.loadAll()
		if err != nil {
			b.Fatalf("loadAll: %v", err)
		}
		if len(records) != 100 {
			b.Fatalf("loadAll: got %d records, want 100", len(records))
		}
	}
}

// BenchmarkList_Lthn100 — the full wails-facing List() call over the
// same 100 encrypted records (loadAll + toDeal projection + filter).
func BenchmarkList_Lthn100(b *testing.B) {
	svc := seedLthnRecords(b, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := svc.List(ListInput{Limit: 100})
		if !r.OK {
			b.Fatalf("List: %s", r.Error())
		}
		out := r.Value.(ListOutput)
		if out.Total != 100 {
			b.Fatalf("List: got Total %d, want 100", out.Total)
		}
	}
}
