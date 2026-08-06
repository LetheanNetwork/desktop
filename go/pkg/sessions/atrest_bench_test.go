// SPDX-Licence-Identifier: EUPL-1.2

// Benchmarks for the at-rest seal/unseal envelope plumbing (atrest.go).
// sealPayload/unsealPayload run on every writeMessages/writeManifest
// (Append, Rename, SetTags, ...) and readMessages/readManifest (Read,
// List, ...) call while an account is unlocked — JSON marshal + base64
// + HMAC-SHA256 canonicalisation wrapped around one PGP public/private
// key operation.
//
// BOUNDED per the house rule: the PGP keypair is generated exactly
// once (sync.Once, same pattern as atrest_test.go's atrestKeyPair) and
// reused across every b.N iteration — the benchmark measures the
// steady-state per-call plumbing an unlocked account pays on every
// save/read, never key generation.
//
// Run:
//
//	go test ./pkg/sessions/... -run '^$' -bench BenchmarkAtRest -benchmem -benchtime=20x

package sessions

import (
	"sync"
	"testing"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/account"

	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// benchAccountSurface is a minimal atrestAccountSurface double — the
// same narrow shape atrest_test.go's sessionsTestAccountSurface
// satisfies, redeclared here because that type lives in the external
// sessions_test package and this file needs in-package access to call
// sealPayload/unsealPayload directly.
type benchAccountSurface struct {
	unlocked []string
	pub      []byte
	priv     []byte
}

func (s *benchAccountSurface) UnlockedAccountIDs() []string { return s.unlocked }

func (s *benchAccountSurface) PublicKeyFor(id string) ([]byte, bool) {
	if id == "" || s.pub == nil {
		return nil, false
	}
	return s.pub, true
}

func (s *benchAccountSurface) PrivateKeyFor(id string) (*account.PrivateKeyHandle, bool) {
	if id == "" || s.priv == nil {
		return nil, false
	}
	return account.NewPrivateKeyHandleForTest(append([]byte(nil), s.priv...)), true
}

var (
	benchAtRestKeyOnce sync.Once
	benchAtRestKeyPub  []byte
	benchAtRestKeyPriv []byte
)

// benchAtRestKeyPair lazily generates one real PGP keypair, cached for
// the whole benchmark run — the "pre-derived key" the house rule
// requires so per-iteration cost never includes key generation.
func benchAtRestKeyPair(b *testing.B) (pub, priv []byte) {
	b.Helper()
	benchAtRestKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Bench", "sessions-bench@lthn.local", "bench")
		if err != nil {
			b.Fatalf("generate bench key pair: %v", err)
		}
		benchAtRestKeyPub = p
		benchAtRestKeyPriv = k
	})
	return benchAtRestKeyPub, benchAtRestKeyPriv
}

func benchSealedCore(b *testing.B) *core.Core {
	b.Helper()
	pub, priv := benchAtRestKeyPair(b)
	c := core.New(
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &benchAccountSurface{
				unlocked: []string{"acct-bench"},
				pub:      pub,
				priv:     priv,
			})
		}),
	)
	r := c.ServiceStartup(core.Background(), nil)
	if !r.OK {
		b.Fatalf("ServiceStartup: %s", r.Error())
	}
	return c
}

// benchMessagePayload is representative of one Append's JSON-encoded
// message-log bytes — a couple of realistic chat turns, not an empty
// stub, since JSON size feeds directly into the PGP encrypt/decrypt
// cost being measured.
var benchMessagePayload = []byte(
	`[{"role":"user","content":"Can you summarise the last deploy and flag anything odd in the logs?"},` +
		`{"role":"assistant","content":"The last deploy shipped the sessions at-rest sealing change; ` +
		`logs show no errors, one slow query on the manifest index worth a follow-up."}]`,
)

func BenchmarkAtRest_SealPayload(b *testing.B) {
	c := benchSealedCore(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := sealPayload(c, benchMessagePayload); !ok {
			b.Fatal("sealPayload: expected ok=true with an unlocked account")
		}
	}
}

func BenchmarkAtRest_UnsealPayload(b *testing.B) {
	c := benchSealedCore(b)
	sealed, ok := sealPayload(c, benchMessagePayload)
	if !ok {
		b.Fatal("fixture seal: expected ok=true with an unlocked account")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plaintext, sealedFlag := unsealPayload(c, sealed)
		if !sealedFlag || plaintext == nil {
			b.Fatal("unsealPayload: expected a decrypted sealed envelope")
		}
	}
}
