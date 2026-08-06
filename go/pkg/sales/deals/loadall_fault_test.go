// SPDX-Licence-Identifier: EUPL-1.2

// loadall_fault_test.go — fault injection for loadAll's per-entry skip
// branches (RFC.stage-e-encrypt-at-rest v2 §4.1: "do NOT abort whole
// List on one bad file"). Each sub-case corrupts exactly one entry
// while a healthy sibling record proves the scan kept going.

package deals_test

import (
	"os"
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

// TestDeals_LoadAll_CorruptLthn_SkippedNotAborted_Ugly — a .lthn file
// too short/malformed to survive loadHeaderOnly is skipped; a healthy
// .md sibling still surfaces via List.
func TestDeals_LoadAll_CorruptLthn_SkippedNotAborted_Ugly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestSvc(t)

	dealsDir := core.PathJoin(home, "Lethean", "sales", "deals")
	if mk := core.MkdirAll(dealsDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	if w := core.WriteFile(core.PathJoin(dealsDir, "202606-DEAL-900.lthn"), []byte("not-a-real-envelope"), 0o600); !w.OK {
		t.Fatalf("seed corrupt .lthn: %s", w.Error())
	}

	// Healthy sibling via the real Create path.
	cr := svc.Create(deals.CreateInput{Customer: "Healthy Co", AmountPence: 100, Stage: "engage"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate one corrupt .lthn entry: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected exactly the healthy record (corrupt one skipped), got %d", len(out.Deals))
	}
}

// TestDeals_LoadAll_UnreadableMd_SkippedNotAborted_Bad — a .md file
// with permissions denying read hits loadAll's `if !raw.OK { continue
// }` branch. Runs on the legacy (non-keyed) gate so the entry is a
// plaintext .md candidate, not shadowed by a preferred .lthn.
func TestDeals_LoadAll_UnreadableMd_SkippedNotAborted_Bad(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root — permission-denial fault injection does not apply")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})

	dealsDir := core.PathJoin(home, "Lethean", "sales", "deals")
	if mk := core.MkdirAll(dealsDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	unreadable := core.PathJoin(dealsDir, "202606-DEAL-901.md")
	legacy := []byte("---\nid: 202606-DEAL-901\ncustomer: Locked\nstage: qual\n---\n")
	if w := core.WriteFile(unreadable, legacy, 0o600); !w.OK {
		t.Fatalf("seed .md: %s", w.Error())
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })

	cr := svc.Create(deals.CreateInput{Customer: "Healthy Legacy Co", AmountPence: 200, Stage: "engage"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate an unreadable .md entry: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected exactly the healthy record (unreadable one skipped), got %d", len(out.Deals))
	}
}

// TestDeals_LoadAll_MalformedMdYaml_SkippedNotAborted_Bad — a .md file
// whose frontmatter fails yaml.Unmarshal hits loadAll's parseRecord
// error `continue` branch.
func TestDeals_LoadAll_MalformedMdYaml_SkippedNotAborted_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := deals.NewService(nil)
	svc.SetSessionGate(&minimalSessionGate{ids: []string{"acct-legacy"}})

	dealsDir := core.PathJoin(home, "Lethean", "sales", "deals")
	if mk := core.MkdirAll(dealsDir, 0o700); !mk.OK {
		t.Fatalf("MkdirAll: %s", mk.Error())
	}
	bad := core.PathJoin(dealsDir, "202606-DEAL-902.md")
	if w := core.WriteFile(bad, []byte("---\n[not: valid: yaml\n---\nbody"), 0o600); !w.OK {
		t.Fatalf("seed malformed .md: %s", w.Error())
	}

	cr := svc.Create(deals.CreateInput{Customer: "Healthy Legacy Two", AmountPence: 300, Stage: "engage"})
	if !cr.OK {
		t.Fatalf("seed Create: %s", cr.Error())
	}

	r := svc.List(deals.ListInput{})
	if !r.OK {
		t.Fatalf("List must tolerate a malformed .md entry: %s", r.Error())
	}
	out := r.Value.(deals.ListOutput)
	if len(out.Deals) != 1 {
		t.Fatalf("expected exactly the healthy record (malformed YAML skipped), got %d", len(out.Deals))
	}
}
