// SPDX-Licence-Identifier: EUPL-1.2

package welfare

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/welfare/slurs"
)

func TestDetect_Service_Detect_Good(t *core.T) {
	// Sustained anger: a heated message doesn't trigger alone (no prior
	// pattern), but a second hostile turn builds sustained hostility and fires.
	w := New(Config{})

	r1 := w.Detect("sess-a", "you useless idiot, you absolute moron!!!")
	core.AssertTrue(t, r1.AngerScore > 0.7, "message is strongly hostile")
	core.AssertFalse(t, r1.Triggered, "a single heated message must not trigger")

	r2 := w.Detect("sess-a", "you pathetic moron, you worthless idiot!!!")
	core.AssertTrue(t, r2.SustainedHostility > 0.5, "the prior hostile turn builds sustained hostility")
	core.AssertTrue(t, r2.Triggered, "sustained + elevated anger triggers mediation")
}

func TestDetect_Service_Detect_Bad(t *core.T) {
	// Civil requests never trigger, however many turns.
	w := New(Config{})
	for i := 0; i < 4; i++ {
		r := w.Detect("sess-b", "could you help me refactor this function please")
		core.AssertFalse(t, r.Triggered, "civil text never triggers")
		core.AssertEqual(t, false, r.SlurMatch)
	}
}

func TestDetect_Service_Detect_Ugly(t *core.T) {
	// A slur fires on a single message — bypasses the sustained-anger gate.
	// Default()'s catalogue is Snider-curated (empty stub), so inject a test term.
	w := New(Config{})
	w.matcher = slurs.New([]string{"testterm"})

	r := w.Detect("sess-c", "you testterm")
	core.AssertTrue(t, r.SlurMatch, "slur detected")
	core.AssertEqual(t, "testterm", r.SlurTerm)
	core.AssertTrue(t, r.Triggered, "a slur triggers on a single message")
}
