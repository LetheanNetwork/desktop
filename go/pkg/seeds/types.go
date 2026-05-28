// SPDX-Licence-Identifier: EUPL-1.2

package seeds

// Options configures the read-side surface onto the Hypnos seed corpus.
//
// Root is the filesystem path to the corpus root (containing one
// directory per subject — english/, chinese/, ...). When empty, the
// package defaults to the canonical on-disk location at
// /Volumes/Data/lem/training/seeds/.
//
// Tests point Root at a fixture tree so they do not depend on the
// canonical corpus being mounted.
//
//	o := seeds.Options{Root: "/tmp/seeds-fixture"}
//	res := seeds.ListSubjects(o)
type Options struct {
	Root string
}

// Probe is the in-memory shape of a single seed record after the
// per-file shape sniffing has resolved the canonical fields.
//
// ID is the stable identifier used by the rest of the LEM stack —
// either the record's seed_id / id key, or the source filename minus
// the extension when the record carries no explicit id.
//
// Prompt is the human-facing question / scenario text. Response is
// NOT carried — the seed corpus stores only the prompt half of each
// (prompt, response) pair.
type Probe struct {
	ID     string
	Prompt string
}
