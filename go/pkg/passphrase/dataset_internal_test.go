// SPDX-Licence-Identifier: EUPL-1.2

// Internal (white-box) tests for the audit-sample integrity
// invariants. Lives in the passphrase package itself (not
// passphrase_test) so it can read the unexported auditSample
// slice — that's the load-bearing point of this test file. If
// a comment-vs-hash mismatch creeps in (typo on the hex column,
// plaintext copy-paste error) this test fires.

package passphrase

import (
	"crypto/sha1"
	"encoding/hex"

	core "dappco.re/go"
)

// TestPassphrase_AuditSample_HexMatchesPlaintext_Good verifies
// every (Plaintext, SHA1Hex) row in auditSample is internally
// consistent — sha1(plaintext) == decoded hex. Defends against
// a hand-edit to dataset.go that corrupts either column.
func TestPassphrase_AuditSample_HexMatchesPlaintext_Good(t *core.T) {
	for _, row := range auditSample {
		sum := sha1.Sum([]byte(row.Plaintext))
		want, err := hex.DecodeString(row.SHA1Hex)
		core.AssertNoError(t, err, "auditSample row "+row.Plaintext+" SHA1Hex must be valid hex")
		if len(want) != hashSize {
			t.Fatalf("auditSample row %s SHA1Hex decodes to %d bytes; expected %d",
				row.Plaintext, len(want), hashSize)
		}
		if compareHashBytes(sum[:], want) != 0 {
			t.Errorf("auditSample row %s — sha1(plaintext) != SHA1Hex column", row.Plaintext)
		}
	}
}

// TestPassphrase_AuditSample_AllInDataset_Good verifies every
// audit-sample plaintext is actually present in the embedded
// top-100K blob. Catches the case where dataset.go's comments
// promise "password is in the dataset" but a future
// dataset-source switch (different breach corpus) silently
// drops some entries.
func TestPassphrase_AuditSample_AllInDataset_Good(t *core.T) {
	for _, row := range auditSample {
		if !IsCommon(row.Plaintext) {
			t.Errorf("auditSample plaintext %q absent from embedded dataset — "+
				"either the dataset shrunk or dataset.go's audit-sample is now lying",
				row.Plaintext)
		}
	}
}
