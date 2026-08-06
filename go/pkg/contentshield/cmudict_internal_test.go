// SPDX-Licence-Identifier: EUPL-1.2

// Internal (white-box) fault-injection tests for initCMUDict's
// line-parsing fallbacks — malformed / adversarial CMU-dict-format
// lines that the shipped starter corpus never actually contains, so
// black-box Lookup calls can't reach these branches. cmudictStarterData
// and cmudictEntries are mutated directly (initCMUDict is called
// without going through cmudictOnce, which none of these tests ever
// touch), so every test here restores both to the real starter
// dictionary via t.Cleanup before returning — no other test in this
// package (none use t.Parallel, confirmed) can observe the synthetic
// corpus.

package contentshield

import "testing"

// withSyntheticCMUDict swaps cmudictStarterData for synthetic, parses
// it directly via initCMUDict (bypassing cmudictOnce), hands the
// resulting map to fn, then restores the real starter data and
// rebuilds cmudictEntries from it so every later test — regardless of
// run order — sees the genuine dictionary.
func withSyntheticCMUDict(t *testing.T, synthetic string, fn func(entries map[string][]string)) {
	t.Helper()
	savedData := cmudictStarterData
	t.Cleanup(func() {
		cmudictStarterData = savedData
		initCMUDict() // rebuild the real dictionary before the next test runs
	})

	cmudictStarterData = synthetic
	initCMUDict()
	fn(cmudictEntries)
}

// TestInitCMUDict_Good_DoubleSpaceFormat pins the primary parse path:
// a well-formed WORD<2 spaces>PHONEMES line.
func TestInitCMUDict_Good_DoubleSpaceFormat(t *testing.T) {
	withSyntheticCMUDict(t, "GOODWORD  PH1 PH2\n", func(entries map[string][]string) {
		got, ok := entries["GOODWORD"]
		if !ok {
			t.Fatal("GOODWORD missing from parsed dictionary")
		}
		if len(got) != 2 || got[0] != "PH1" || got[1] != "PH2" {
			t.Errorf("GOODWORD phonemes = %v, want [PH1 PH2]", got)
		}
	})
}

// TestInitCMUDict_Bad_SingleSpaceFallback covers the fallback branch:
// when the 2-space split yields fewer than 2 parts, initCMUDict falls
// back to a single-space SplitN. Some upstream CMU-format files use
// exactly one space, per the fallback's own doc comment.
func TestInitCMUDict_Bad_SingleSpaceFallback(t *testing.T) {
	withSyntheticCMUDict(t, "SINGLESPACED PH1 PH2\n", func(entries map[string][]string) {
		got, ok := entries["SINGLESPACED"]
		if !ok {
			t.Fatal("SINGLESPACED missing — single-space fallback split must still parse the entry")
		}
		if len(got) != 2 || got[0] != "PH1" || got[1] != "PH2" {
			t.Errorf("SINGLESPACED phonemes = %v, want [PH1 PH2]", got)
		}
	})
}

// TestInitCMUDict_Ugly_NoSeparatorAtAll covers the case where even the
// single-space fallback finds fewer than 2 parts — a line with no
// whitespace anywhere. Must be skipped (continue), not panic or
// produce a bogus zero-phoneme entry.
func TestInitCMUDict_Ugly_NoSeparatorAtAll(t *testing.T) {
	withSyntheticCMUDict(t, "LONELYWORDWITHNOPHONEMES\nGOODWORD  PH1\n", func(entries map[string][]string) {
		if _, ok := entries["LONELYWORDWITHNOPHONEMES"]; ok {
			t.Error("a line with no separator at all must be skipped, not stored with nil/empty phonemes")
		}
		if _, ok := entries["GOODWORD"]; !ok {
			t.Error("a well-formed line after a malformed one must still parse — one bad line must not abort the loader")
		}
	})
}

// TestInitCMUDict_Ugly_EmptyPhonemeFieldSkipped covers the
// word=="" || phonemeStr=="" guard: a line whose 2-space split lands
// on an empty phoneme field (4 consecutive spaces between word and the
// next token produces an empty middle element from core.Split) must be
// skipped rather than stored with a zero-length phoneme list.
func TestInitCMUDict_Ugly_EmptyPhonemeFieldSkipped(t *testing.T) {
	// "WORD" + 4 spaces + "X": core.Split(line, "  ") walks the 4-space
	// run as two consecutive 2-space matches, producing
	// ["WORD", "", "X"] — parts[1] is the empty string.
	withSyntheticCMUDict(t, "EMPTYFIELD    X\nGOODWORD  PH1\n", func(entries map[string][]string) {
		if got, ok := entries["EMPTYFIELD"]; ok {
			t.Errorf("a line whose phoneme field trims to empty must be skipped, got %v", got)
		}
		if _, ok := entries["GOODWORD"]; !ok {
			t.Error("a well-formed line after the empty-field line must still parse")
		}
	})
}

// TestInitCMUDict_Bad_CommentsAndBlankLinesSkipped pins the existing
// ";;"-comment and blank-line skip alongside the fallback paths above,
// so this file documents the loader's full skip-branch behaviour in
// one place.
func TestInitCMUDict_Bad_CommentsAndBlankLinesSkipped(t *testing.T) {
	withSyntheticCMUDict(t, ";; a comment\n\nGOODWORD  PH1\n", func(entries map[string][]string) {
		if len(entries) != 1 {
			t.Errorf("expected exactly 1 entry (comment + blank line skipped), got %d: %v", len(entries), entries)
		}
		if _, ok := entries["GOODWORD"]; !ok {
			t.Error("GOODWORD must still parse alongside a comment and a blank line")
		}
	})
}
