// SPDX-Licence-Identifier: EUPL-1.2

// atrest_canonjson_test.go — RFC 8785 JCS conformance for the
// CanonicalJSON encoder. Header MAC stability is the load-bearing
// reason this encoder exists; the tests here pin every property
// the MAC depends on.

package recordfile_test

import (
	"testing"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/recordfile"
)

// TestAtRestCanonJSON_DeterministicByteOutput_Good — same map two
// different ways → same bytes.
func TestAtRestCanonJSON_DeterministicByteOutput_Good(t *testing.T) {
	m := map[string]any{"schema": "lthn.atrest.v1", "id": "abc", "version": int64(1)}
	r1 := recordfile.CanonicalJSON(m)
	r2 := recordfile.CanonicalJSON(m)
	if !r1.OK || !r2.OK {
		t.Fatalf("encode failed: r1=%v r2=%v", r1.Error(), r2.Error())
	}
	if string(r1.Value.([]byte)) != string(r2.Value.([]byte)) {
		t.Fatalf("non-deterministic output:\n  r1=%q\n  r2=%q", r1.Value, r2.Value)
	}
}

// TestAtRestCanonJSON_SortedKeys_Good — keys appear in lexicographic
// order regardless of input map iteration order.
func TestAtRestCanonJSON_SortedKeys_Good(t *testing.T) {
	m := map[string]any{"z": "last", "a": "first", "m": "mid"}
	r := recordfile.CanonicalJSON(m)
	if !r.OK {
		t.Fatalf("encode failed: %s", r.Error())
	}
	got := string(r.Value.([]byte))
	want := `{"a":"first","m":"mid","z":"last"}`
	if got != want {
		t.Fatalf("sort order mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestAtRestCanonJSON_NoWhitespace_Good — no insignificant whitespace
// between tokens (the MAC over a header with extra spaces would
// drift).
func TestAtRestCanonJSON_NoWhitespace_Good(t *testing.T) {
	m := map[string]any{"a": 1, "b": []any{"x", "y"}, "c": map[string]any{"nested": true}}
	r := recordfile.CanonicalJSON(m)
	if !r.OK {
		t.Fatalf("encode failed: %s", r.Error())
	}
	got := string(r.Value.([]byte))
	for _, c := range got {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			// Allow whitespace only inside string literals — detect by
			// position-of-quote logic. For this fixture neither key
			// nor value contains whitespace, so any presence is a bug.
			t.Fatalf("found whitespace in canonical output: %q", got)
		}
	}
}

// TestAtRestCanonJSON_StableAcrossOrderings_Good — load-bearing for
// header MAC. Constructing the SAME logical map via two different key
// insertion orders MUST produce byte-identical canonical output.
func TestAtRestCanonJSON_StableAcrossOrderings_Good(t *testing.T) {
	m1 := map[string]any{}
	m1["schema"] = "lthn.atrest.v1"
	m1["id"] = "deal-001"
	m1["version"] = int64(7)
	m1["account"] = map[string]any{"id": "abc", "key.fingerprint": "ff"}
	m1["surface"] = "sales.deals"

	m2 := map[string]any{}
	m2["surface"] = "sales.deals"
	m2["account"] = map[string]any{"key.fingerprint": "ff", "id": "abc"}
	m2["version"] = int64(7)
	m2["id"] = "deal-001"
	m2["schema"] = "lthn.atrest.v1"

	r1 := recordfile.CanonicalJSON(m1)
	r2 := recordfile.CanonicalJSON(m2)
	if !r1.OK || !r2.OK {
		t.Fatalf("encode failed: r1=%v r2=%v", r1.Error(), r2.Error())
	}
	if string(r1.Value.([]byte)) != string(r2.Value.([]byte)) {
		t.Fatalf("canon JSON drifted across orderings:\n  m1: %s\n  m2: %s",
			r1.Value, r2.Value)
	}
}

// TestAtRestCanonJSON_StringEscapes_Good — RFC 8785 mandates exactly
// the 7 mandatory escapes; everything else passes through as UTF-8.
func TestAtRestCanonJSON_StringEscapes_Good(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", `"hello"`},
		{"a\"b", `"a\"b"`},
		{"a\\b", `"a\\b"`},
		{"a\nb", `"a\nb"`},
		{"a\tb", `"a\tb"`},
		{"unicode→arrow", `"unicode→arrow"`}, // UTF-8 passthrough
	}
	for _, tc := range cases {
		r := recordfile.CanonicalJSON(tc.in)
		if !r.OK {
			t.Fatalf("encode %q failed: %s", tc.in, r.Error())
		}
		got := string(r.Value.([]byte))
		if got != tc.want {
			t.Fatalf("escape mismatch for %q:\n  got:  %s\n  want: %s", tc.in, got, tc.want)
		}
	}
}

// TestAtRestCanonJSON_IntegerEncoding_Good — integers MUST serialise
// without exponent / decimal-fraction forms (RFC 8785 §3.2.2.3).
func TestAtRestCanonJSON_IntegerEncoding_Good(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{int64(0), "0"},
		{int64(1), "1"},
		{int64(-1), "-1"},
		{int64(1747440000), "1747440000"},
		{int(42), "42"},
	}
	for _, tc := range cases {
		r := recordfile.CanonicalJSON(tc.in)
		if !r.OK {
			t.Fatalf("encode %v failed: %s", tc.in, r.Error())
		}
		got := string(r.Value.([]byte))
		if got != tc.want {
			t.Fatalf("int encoding mismatch for %v:\n  got:  %s\n  want: %s", tc.in, got, tc.want)
		}
	}
}

// TestAtRestCanonJSON_Ugly_UnsupportedType_Bad — floats / structs /
// channels MUST return the typed unsupported_type error rather than
// silently producing a digest that drifts cross-platform.
func TestAtRestCanonJSON_Ugly_UnsupportedType_Bad(t *testing.T) {
	for _, v := range []any{float64(3.14), float32(1.5), struct{ X int }{}, make(chan int)} {
		r := recordfile.CanonicalJSON(v)
		if r.OK {
			t.Fatalf("expected unsupported_type error for %T, got OK", v)
		}
		if r.Code() != "recordfile.atrest.canonjson.unsupported_type" {
			t.Fatalf("expected code recordfile.atrest.canonjson.unsupported_type for %T, got %q", v, r.Code())
		}
	}
}

// TestAtRestCanonJSON_NestedMap_Good — nested maps are themselves
// sorted; the canonical form recurses.
func TestAtRestCanonJSON_NestedMap_Good(t *testing.T) {
	m := map[string]any{
		"outer": map[string]any{"z": 1, "a": 2},
	}
	r := recordfile.CanonicalJSON(m)
	if !r.OK {
		t.Fatalf("encode failed: %s", r.Error())
	}
	got := string(r.Value.([]byte))
	want := `{"outer":{"a":2,"z":1}}`
	if got != want {
		t.Fatalf("nested sort mismatch:\n  got:  %s\n  want: %s", got, want)
	}
}

// Smoke: confirm core.HMAC over canon-bytes produces stable hex on
// re-encode (the operational property the MAC depends on).
func TestAtRestCanonJSON_MACStability_Good(t *testing.T) {
	hdr := map[string]any{
		"schema":  "lthn.atrest.v1",
		"id":      "rec-001",
		"version": int64(1),
		"account": map[string]any{"id": "abc", "key.fingerprint": "deadbeef"},
		"surface": "sales.deals",
	}
	pubKey := []byte("test-pub-key-bytes")

	canon1 := recordfile.CanonicalJSON(hdr).Value.([]byte)
	canon2 := recordfile.CanonicalJSON(hdr).Value.([]byte)
	mac1 := core.HMAC("sha256", pubKey, canon1)
	mac2 := core.HMAC("sha256", pubKey, canon2)
	if !mac1.OK || !mac2.OK {
		t.Fatalf("HMAC failed: mac1=%v mac2=%v", mac1, mac2)
	}
	if core.HexEncode(mac1.Value.([]byte)) != core.HexEncode(mac2.Value.([]byte)) {
		t.Fatalf("MAC drifted across encodes")
	}
}
