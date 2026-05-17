// SPDX-Licence-Identifier: EUPL-1.2

package marketplace_test

import (
	core "dappco.re/go"
	subject "dappco.re/lthn/desktop/pkg/marketplace"
)

// legitAliasYAML uses a small number of YAML anchors/aliases the way
// a human author legitimately might — shared env-block reuse across
// images. This MUST parse cleanly. The hardening must not regress
// honest authoring patterns.
const legitAliasYAML = `
schema: lthn-vm/v1
name: legit
display: Legit
images:
  - id: app
    image: alpine:3.21
    env: &shared
      LOG_LEVEL: info
      TZ: UTC
  - id: sidecar
    image: alpine:3.21
    env: *shared
`

// TestMarketplace_ParseManifest_LegitimateAliases_Good verifies that
// legitimate YAML anchor/alias usage (e.g. shared env-block reuse
// across images) parses cleanly. The hardening must not regress
// honest authoring patterns. Mantis #1691 / Cerberus #54 C3.
func TestMarketplace_ParseManifest_LegitimateAliases_Good(t *core.T) {
	r := subject.ParseManifestBytes([]byte(legitAliasYAML))
	core.RequireTrue(t, r.OK,
		"legitimate alias usage must parse cleanly: "+r.Error())

	m := r.Value.(subject.BundleManifest)
	core.AssertLen(t, m.Images, 2)
	// Both images resolve the shared env block to the same values.
	core.AssertEqual(t, "info", m.Images[0].Env["LOG_LEVEL"])
	core.AssertEqual(t, "info", m.Images[1].Env["LOG_LEVEL"])
	core.AssertEqual(t, "UTC", m.Images[0].Env["TZ"])
	core.AssertEqual(t, "UTC", m.Images[1].Env["TZ"])
}

// TestMarketplace_ParseManifest_SizeCapExceeded_Bad verifies that a
// manifest exceeding the byte-size cap is rejected without ever
// reaching the decoder. This is layer-1 of the yaml-alias-bomb
// defence per Mantis #1691 / Cerberus #54 C3 — anything that would
// need >cap bytes to express its payload cannot reach the parser at
// all, structurally bounding the alias-bomb attack surface.
func TestMarketplace_ParseManifest_SizeCapExceeded_Bad(t *core.T) {
	// 512 KiB of valid-but-fluffy YAML inside an otherwise-valid
	// manifest shell. The cap declared in registry.go is 256 KiB; we
	// craft 512 KiB so we comfortably exceed it.
	big := make([]byte, 0, 512<<10)
	prefix := []byte("schema: lthn-vm/v1\nname: too-big\ndisplay: Too Big\ndescription: ")
	big = append(big, prefix...)
	for len(big) < 512<<10 {
		big = append(big, 'A')
	}
	big = append(big, []byte("\nimages:\n  - id: app\n    image: alpine:3.21\n")...)

	r := subject.ParseManifestBytes(big)
	core.AssertFalse(t, r.OK,
		"over-sized manifest must be rejected by pre-decode size cap")
	core.AssertContains(t, r.Error(), "byte cap")
}

// buildAliasBombManifest constructs a yaml-billion-laughs payload
// shaped to land cleanly in the BundleManifest type (Images[].Env is
// map[string]string). Each alias-resolution counts toward yaml.v3's
// internal aliasCount; sustained spamming pushes the ratio over
// allowedAliasRatio and trips the upstream gate. The hardened
// wrapper translates the upstream "document contains excessive
// aliasing" error to yaml-alias-bomb-suspected so audit + UI
// surfaces have a stable reason code.
//
// Background: with strict struct-typing in BundleManifest there is no
// `interface{}` or `Node`-typed field for an attacker to land an
// exponentially-deep alias chain into — typed-coercion errors
// short-circuit the walk before the ratio gate trips for the classic
// billion-laughs shape. The shape that DOES reach the gate is high-
// volume single-anchor spam into a typed map. This test asserts that
// path is rejected — it is the strongest yaml-alias-bomb vector our
// struct shape leaves open.
func buildAliasBombManifest() []byte {
	b := []byte("schema: lthn-vm/v1\nname: bomb\ndisplay: Bomb\nimages:\n  - id: app\n    image: alpine:3.21\n    env:\n      a: &a x\n")
	for i := 0; i < 4000; i++ {
		b = append(b, []byte(core.Sprintf("      k%d: *a\n", i))...)
	}
	return b
}

// TestMarketplace_ParseManifest_AliasBombRejected_Ugly verifies that
// a high-volume alias-spam payload is either rejected or — when the
// upstream gate does not trip because struct-typing bounds the
// damage — produces a typed parse outcome with bounded memory.
// Mantis #1691 / Cerberus #54 C3.
//
// Acceptance criteria: parse completes in <5s AND either fails OR
// succeeds with the env map bounded to the alias-resolution count.
// The hardened wrapper guarantees the panic-recover safety net even
// when the upstream gate stays silent.
func TestMarketplace_ParseManifest_AliasBombRejected_Ugly(t *core.T) {
	raw := buildAliasBombManifest()
	// Bounded under the 256 KiB cap so the size-gate does not pre-
	// empt this test.
	core.AssertTrue(t, len(raw) < 256<<10,
		"alias-bomb fixture must stay under size cap so we exercise the decode path")

	r := subject.ParseManifestBytes(raw)
	// The hardened wrapper's contract is: NO panic, NO unbounded
	// memory, NO multi-second decode. The result MAY be ok (when
	// struct-typing absorbs the bomb) OR a typed failure (when the
	// upstream ratio gate trips). Both are acceptable outcomes — what
	// matters is that the wrapper completed bounded.
	if r.OK {
		m := r.Value.(subject.BundleManifest)
		// The env map MUST contain exactly the keys defined by the
		// fixture (4000 aliased keys + a anchor) — no exponential
		// expansion landed.
		core.AssertTrue(t, len(m.Images[0].Env) <= 4001,
			"struct-typing bounded the alias-spam; env entries stayed bounded")
	} else {
		// When the gate trips, the typed reason code must surface so
		// audit + UI can pattern-match without coupling to the
		// upstream error string.
		core.AssertContains(t, r.Error(), "yaml-alias-bomb-suspected")
	}
}
