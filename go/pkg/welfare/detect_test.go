// SPDX-Licence-Identifier: EUPL-1.2

package welfare

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/welfare/slurs"
)

func TestDetect_Service_Detect_Good(t *core.T) {
	// Sustained anger: a heated message with no history doesn't trigger, but
	// the same heat on top of prior hostile turns does.
	w := New(Config{})

	r1 := w.Detect("you useless idiot, you absolute moron!!!", nil)
	core.AssertTrue(t, r1.AngerScore > 0.7, "message is strongly hostile")
	core.AssertFalse(t, r1.Triggered, "a single heated message with no history must not trigger")

	priors := []string{"you pathetic moron", "you worthless idiot"}
	r2 := w.Detect("you absolute clueless moron!!!", priors)
	core.AssertTrue(t, r2.SustainedHostility > 0.5, "prior hostile turns build sustained hostility")
	core.AssertTrue(t, r2.Triggered, "sustained + elevated anger triggers mediation")
}

func TestDetect_Service_Detect_Bad(t *core.T) {
	// Civil requests never trigger, however long the conversation.
	w := New(Config{})
	priors := []string{
		"could you help me refactor this",
		"thanks, and how do I test it",
		"great, what about error handling",
	}
	r := w.Detect("could you add a docstring please", priors)
	core.AssertFalse(t, r.Triggered, "civil text never triggers")
	core.AssertEqual(t, false, r.SlurMatch)
	core.AssertEqual(t, 0.0, r.SustainedHostility)
}

func TestDetect_Service_Detect_Ugly(t *core.T) {
	// A slur fires on a single message — bypasses the sustained-anger gate.
	// Default()'s catalogue is Snider-curated (empty stub), so inject a test term.
	w := New(Config{})
	w.matcher = slurs.New([]string{"testterm"})

	r := w.Detect("you testterm", nil)
	core.AssertTrue(t, r.SlurMatch, "slur detected")
	core.AssertEqual(t, "testterm", r.SlurTerm)
	core.AssertTrue(t, r.Triggered, "a slur triggers on a single message")
}
