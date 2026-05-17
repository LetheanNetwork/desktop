// SPDX-Licence-Identifier: EUPL-1.2

package forecast_test

import (
	"dappco.re/lthn/desktop/pkg/sales/deals"
)

// stubSessionGate satisfies deals.SessionGate with a non-empty
// unlocked-accounts slice so dealSvc.Create passes the assertUnlocked
// fail-safe gate added by Stage E.D B.2 (RFC.stage-e-unlockgate v2 §2.2,
// Mantis #1613). The stub omits PublicKeyFor / PrivateKeyFor — the deals
// at-rest writer construction degrades to legacy plaintext mode when no
// key provider is wired, which is exactly what forecast's frontmatter
// scan (loadAllDeals → parseFM) expects on disk.
//
// Mirrors the pattern in pkg/sales/deals/deals_test.go (H#164 newTestSvc)
// scoped down to just the locked/unlocked surface forecast tests need.
//
// Usage example:
//
//	dealSvc := deals.NewService(nil)
//	dealSvc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
//	dealSvc.Create(deals.CreateInput{Customer: "Heritage Law"})
type stubSessionGate struct {
	ids []string
}

func (s *stubSessionGate) UnlockedAccountIDs() []string { return s.ids }

// newUnlockedDealsSvc constructs a deals.Service pre-wired with a
// SessionGate reporting one unlocked account so Create proceeds through
// the legacy plaintext .md path that forecast's loader reads.
//
// Usage example:
//
//	dealSvc := newUnlockedDealsSvc()
//	dealSvc.Create(deals.CreateInput{Customer: "Won Co", Stage: "won", AmountPence: 10000})
func newUnlockedDealsSvc() *deals.Service {
	svc := deals.NewService(nil)
	svc.SetSessionGate(&stubSessionGate{ids: []string{"acct-test"}})
	return svc
}

// Compile-time assertion: stubSessionGate satisfies deals.SessionGate.
var _ deals.SessionGate = (*stubSessionGate)(nil)
