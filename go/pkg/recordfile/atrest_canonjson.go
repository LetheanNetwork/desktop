// SPDX-Licence-Identifier: EUPL-1.2

// atrest_canonjson.go — RFC 8785 JSON Canonicalisation Scheme (JCS)
// encoder used to produce the byte-stable input fed into the
// at-rest header MAC (§2.6 of RFC.stage-e-encrypt-at-rest v2).
//
// MAC stability depends on canonical encoding: any map ordering /
// whitespace / number-formatting drift between encode and decode
// would cause every previously written header to fail MAC
// verification on the next read. RFC 8785 fixes the shape.
//
// Scope: this implementation covers the subset of JSON values the
// at-rest header carries (string, integer, bool, null, map[string]any,
// []any). Floating-point numbers and exponent forms are not used by
// the §2.4 header schema and are rejected with a typed error so an
// accidental float entering the header surfaces at encode rather
// than producing a digest that drifts cross-platform.
//
// Usage example:
//
//	r := recordfile.CanonicalJSON(map[string]any{"b": 1, "a": 2})
//	if r.OK { bytes := r.Value.([]byte) } // → {"a":2,"b":1}

package recordfile

import (
	"bytes"
	"sort"
	"unicode/utf16"

	core "dappco.re/go"
)

// CanonicalJSON serialises v into RFC 8785 canonical JSON.
//
// Result.OK=false carries a *core.Err with Code
// "recordfile.atrest.canonjson.unsupported_type" when v contains a
// value type outside the at-rest header subset (notably float64 /
// float32 — see file doc-comment for the rationale).
//
// Sort keys are compared by their UTF-16 code-unit sequences per
// RFC 8785 §3.2.3, not by raw byte order — Go's default
// sort.Strings would diverge for non-BMP code points.
//
// Usage example:
//
//	header := map[string]any{"schema": "lthn.atrest.v1", "id": "abc"}
//	r := recordfile.CanonicalJSON(header)
//	if !r.OK { return r }
//	mac := core.HMAC("sha256", pubKey, r.Value.([]byte))
func CanonicalJSON(v any) core.Result {
	buf := &bytes.Buffer{}
	if err := canonEncode(buf, v); err != nil {
		return core.Fail(err)
	}
	return core.Ok(buf.Bytes())
}

// canonEncode dispatches v to the correct canonical encoder.
// String / int / bool / nil / map / slice are supported; everything
// else returns the typed "unsupported_type" error.
func canonEncode(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case nil:
		buf.WriteString("null")
		return nil
	case bool:
		if x {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return nil
	case string:
		return canonEncodeString(buf, x)
	case int:
		return canonEncodeInt(buf, int64(x))
	case int32:
		return canonEncodeInt(buf, int64(x))
	case int64:
		return canonEncodeInt(buf, x)
	case uint:
		return canonEncodeUint(buf, uint64(x))
	case uint32:
		return canonEncodeUint(buf, uint64(x))
	case uint64:
		return canonEncodeUint(buf, x)
	case float64:
		// JSON unmarshal lands integer header values as float64. The
		// header schema (§2.4) carries integers only — version,
		// created_at, updated_at, audience sizes — so float64 with
		// no fractional part is the JSON-unmarshal artefact of an
		// integer and re-encodes as integer for MAC stability.
		// Floats with fractional parts ARE rejected because the
		// header schema never produces one and silent acceptance
		// would risk a cross-platform digest drift.
		if x != float64(int64(x)) {
			return core.NewCode("recordfile.atrest.canonjson.unsupported_type",
				core.Sprintf("canonjson: non-integral float64 %v cannot serialise stably", x))
		}
		return canonEncodeInt(buf, int64(x))
	case float32:
		if x != float32(int32(x)) {
			return core.NewCode("recordfile.atrest.canonjson.unsupported_type",
				core.Sprintf("canonjson: non-integral float32 %v cannot serialise stably", x))
		}
		return canonEncodeInt(buf, int64(x))
	case map[string]any:
		return canonEncodeMap(buf, x)
	case []any:
		return canonEncodeSlice(buf, x)
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return canonEncodeSlice(buf, out)
	}
	return core.NewCode("recordfile.atrest.canonjson.unsupported_type",
		core.Sprintf("canonjson: unsupported value type %T", v))
}

// canonEncodeInt writes a JSON integer with no decimal / exponent.
// RFC 8785 §3.2.2 mandates I-JSON number form; integers within the
// safe range (-2^53, 2^53) serialise without any spread.
func canonEncodeInt(buf *bytes.Buffer, n int64) error {
	buf.WriteString(core.Sprintf("%d", n))
	return nil
}

// canonEncodeUint writes a JSON unsigned integer.
func canonEncodeUint(buf *bytes.Buffer, n uint64) error {
	buf.WriteString(core.Sprintf("%d", n))
	return nil
}

// canonEncodeString writes a JSON string per RFC 8785 §3.2.2.2 — only
// the mandatory escapes (control chars, '"', '\\') are emitted; every
// other UTF-8 sequence passes through verbatim (no \uXXXX expansion).
func canonEncodeString(buf *bytes.Buffer, s string) error {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				buf.WriteString(core.Sprintf(`\u%04x`, r))
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
	return nil
}

// canonEncodeMap writes a JSON object with keys sorted by their
// UTF-16 code-unit sequence (RFC 8785 §3.2.3). Within JSON header
// schemas this collapses to lexicographic byte order, but the UTF-16
// projection is used for spec-conformance so canonical bytes never
// drift if a non-BMP key (extremely unlikely in headers) is added
// later.
func canonEncodeMap(buf *bytes.Buffer, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return lessUTF16(keys[i], keys[j])
	})
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := canonEncodeString(buf, k); err != nil {
			return err
		}
		buf.WriteByte(':')
		if err := canonEncode(buf, m[k]); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

// canonEncodeSlice writes a JSON array preserving input order
// (RFC 8785 §3.2.4 — arrays are ordered).
func canonEncodeSlice(buf *bytes.Buffer, xs []any) error {
	buf.WriteByte('[')
	for i, x := range xs {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := canonEncode(buf, x); err != nil {
			return err
		}
	}
	buf.WriteByte(']')
	return nil
}

// lessUTF16 compares two strings by their UTF-16 code-unit sequences,
// matching RFC 8785 §3.2.3 sort order. For pure-ASCII keys this is
// identical to byte comparison; the implementation matters only when
// supplementary-plane code points (rare in header schemas) appear.
func lessUTF16(a, b string) bool {
	au := utf16.Encode([]rune(a))
	bu := utf16.Encode([]rune(b))
	n := len(au)
	if len(bu) < n {
		n = len(bu)
	}
	for i := 0; i < n; i++ {
		if au[i] != bu[i] {
			return au[i] < bu[i]
		}
	}
	return len(au) < len(bu)
}
