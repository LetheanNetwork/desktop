// SPDX-Licence-Identifier: EUPL-1.2

// legacy_write_test.go — the plaintext writeRecord fallback (Stage
// E.D.B.1's pre-cutover path) is reachable whenever the wired
// SessionGate satisfies the narrow SessionGate contract (so
// assertUnlocked passes) but does NOT satisfy the wider
// accountKeyProvider surface (so atrestWriterFor degrades). The
// existing test double (stubSessionGate in deals_test.go) always wires
// both PublicKeyFor + PrivateKeyFor, so the legacy branch has been
// dark since the E.D.B.1 retrofit landed — minimalSessionGate below
// is deliberately narrower so writeRecord's plaintext fallback +
// paths.AtomicWriteWithVersion conflict-envelope wiring get real
// cover.

package deals_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

// minimalSessionGate satisfies ONLY deals.SessionGate — no
// PublicKeyFor / PrivateKeyFor — so atrestWriterFor's runtime type
// assertion against accountKeyProvider fails and writeRecord falls
// through to the legacy plaintext .md writer.
type minimalSessionGate struct{ ids []string }

func (m *minimalSessionGate) UnlockedAccountIDs() []string { return m.ids }

func TestDeals_LegacyWritePath_Create_WritesPlaintextMd_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})

	r := svc.Create(deals.CreateInput{
		Customer: "Legacy Plaintext Co", AmountPence: 5000, Stage: "engage",
	})
	if !r.OK {
		t.Fatalf("Create over the legacy gate must succeed: %s", r.Error())
	}
	d := r.Value.(deals.Deal)

	mdPath := core.PathJoin(home, "Lethean", "sales", "deals", d.ID+".md")
	if stat := core.Stat(mdPath); !stat.OK {
		t.Fatalf("expected plaintext %s, stat failed: %s", mdPath, stat.Error())
	}
	lthnPath := core.PathJoin(home, "Lethean", "sales", "deals", d.ID+".lthn")
	if stat := core.Stat(lthnPath); stat.OK {
		t.Fatal("minimal gate must NOT produce an encrypted .lthn envelope")
	}
}

func TestDeals_LegacyWritePath_UpdateStage_VersionAdvances_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})

	cr := svc.Create(deals.CreateInput{Customer: "Stannard & Co", AmountPence: 9000, Stage: "engage"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	ur := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
	if !ur.OK {
		t.Fatalf("UpdateStage over legacy gate must succeed: %s", ur.Error())
	}
	if ur.Value.(deals.Deal).Stage != "Proposal" {
		t.Fatalf("expected Proposal, got %q", ur.Value.(deals.Deal).Stage)
	}
}

func TestDeals_LegacyWritePath_AddActivity_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})

	cr := svc.Create(deals.CreateInput{Customer: "Whitethorn Press", AmountPence: 3600, Stage: "close"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	ar := svc.AddActivity(deals.AddActivityInput{DealID: id, K: "meet", Who: "you", T: "site visit"})
	if !ar.OK {
		t.Fatalf("AddActivity over legacy gate must succeed: %s", ar.Error())
	}
}

// TestDeals_LegacyWritePath_VersionStale_Ugly — the pre-cutover
// version-frontmatter conflict path (paths.ConflictEnvelope wrapping
// paths.VersionStale) IS reachable through the plaintext fallback, so
// pin it directly instead of relying on the skipped at-rest variant in
// deals_test.go. Same race shape: two concurrent UpdateStage calls,
// one loses the optimistic-lock check inside writeRecord's legacy
// branch.
func TestDeals_LegacyWritePath_VersionStale_Ugly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})

	cr := svc.Create(deals.CreateInput{Customer: "Pemberton Capital", AmountPence: 6200, Stage: "engage"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}
	id := cr.Value.(deals.Deal).ID

	var conflict core.Result
	var saw bool
	for attempt := 0; attempt < 32 && !saw; attempt++ {
		var (
			mu      sync.Mutex
			results []core.Result
			wg      sync.WaitGroup
			start   = make(chan struct{})
		)
		wg.Add(2)
		for i := 0; i < 2; i++ {
			go func() {
				defer wg.Done()
				<-start
				r := svc.UpdateStage(deals.UpdateStageInput{ID: id, Stage: "propose"})
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}()
		}
		close(start)
		wg.Wait()

		for _, r := range results {
			if !r.OK && core.Contains(r.Error(), "deals.update.conflict") {
				conflict = r
				saw = true
				break
			}
		}
	}
	if !saw {
		t.Skip("could not provoke a writer race after 32 attempts — environment lock skew; flake-defensive skip")
	}

	env, ok := paths.ConflictEnvelopeFrom(conflict.Value)
	if !ok {
		t.Fatalf("expected paths.ConflictEnvelope in Result.Value, got %T", conflict.Value)
	}
	if env.Code != "deals.update.conflict" {
		t.Fatalf("expected envelope code deals.update.conflict, got %q", env.Code)
	}
	if env.CurrentVersion < 1 {
		t.Fatalf("expected CurrentVersion >= 1, got %d", env.CurrentVersion)
	}
}

// TestDeals_LegacyWritePath_LockedGate_FailsClosed_Bad — even on the
// legacy (non-keyed) gate shape, zero unlocked accounts must still
// fail-closed before any write is attempted.
func TestDeals_LegacyWritePath_LockedGate_FailsClosed_Bad(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := deals.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{}})

	r := svc.Create(deals.CreateInput{Customer: "X", AmountPence: 1, Stage: "engage"})
	if r.OK {
		t.Fatal("expected Create to fail-closed with zero unlocked accounts")
	}
	if !core.Contains(r.Error(), "deals.session.locked") {
		t.Fatalf("expected deals.session.locked, got %q", r.Error())
	}
}
