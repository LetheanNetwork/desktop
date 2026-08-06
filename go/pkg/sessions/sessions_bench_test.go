// SPDX-Licence-Identifier: EUPL-1.2

// Benchmarks for the realistic Append/Read round trip — the sealed
// envelope plumbing (atrest_bench_test.go) plus JSON marshal/unmarshal
// plus the go-store action-bus write/read, exactly as a live chat
// window drives it on every message. Uses an in-memory store
// (":memory:", same substrate coreFixture's tempdir variant persists
// through) and an unlocked-account fixture so every Append/Read seals
// and unseals, matching the common "account unlocked" runtime state.
//
// Append genuinely re-reads and re-writes the WHOLE message log on
// every call (readMessages → append in memory → writeMessages) — see
// sessions.go's Append. With -benchtime=20x the seeded session grows
// by up to 20 messages over one benchmark run, so the reported ns/op
// is the realistic average cost of appending to a session in the
// 24-44 message range, not a pathological unbounded blow-up.
//
// Run:
//
//	go test ./pkg/sessions/... -run '^$' -bench BenchmarkSessions -benchmem -benchtime=20x

package sessions_test

import (
	"sync"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/store"

	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/sessions"

	"github.com/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// benchSealedAccountSurface mirrors sessionsTestAccountSurface
// (atrest_test.go) — redeclared under a distinct name so this file's
// benchmark-only fixture stays independent of the correctness-test
// fixtures it sits beside in the same package.
type benchSealedAccountSurface struct {
	unlocked []string
	pub      []byte
	priv     []byte
}

func (s *benchSealedAccountSurface) UnlockedAccountIDs() []string { return s.unlocked }

func (s *benchSealedAccountSurface) PublicKeyFor(id string) ([]byte, bool) {
	if id == "" || s.pub == nil {
		return nil, false
	}
	return s.pub, true
}

func (s *benchSealedAccountSurface) PrivateKeyFor(id string) (*account.PrivateKeyHandle, bool) {
	if id == "" || s.priv == nil {
		return nil, false
	}
	return account.NewPrivateKeyHandleForTest(append([]byte(nil), s.priv...)), true
}

var (
	benchSessionsKeyOnce sync.Once
	benchSessionsKeyPub  []byte
	benchSessionsKeyPriv []byte
)

// benchSessionsKeyPair lazily generates one real PGP keypair, cached
// for the whole benchmark run — never regenerated per iteration.
func benchSessionsKeyPair(b *testing.B) (pub, priv []byte) {
	b.Helper()
	benchSessionsKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Bench", "sessions-bench@lthn.local", "bench")
		if err != nil {
			b.Fatalf("generate bench key pair: %v", err)
		}
		benchSessionsKeyPub = p
		benchSessionsKeyPriv = k
	})
	return benchSessionsKeyPub, benchSessionsKeyPriv
}

// benchSessionsSealedCore builds a Core with an in-memory store AND an
// unlocked-account surface, so writeMessages/readMessages seal and
// unseal on every call — the realistic "account unlocked" runtime
// state a live chat window runs under.
func benchSessionsSealedCore(b *testing.B) *core.Core {
	b.Helper()
	pub, priv := benchSessionsKeyPair(b)
	c := core.New(
		core.WithName("store", store.NewService(store.StoreConfig{
			DatabasePath: ":memory:",
		})),
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &benchSealedAccountSurface{
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

// seedSession creates a session and appends n realistic prior
// messages, returning its id.
func seedSession(b *testing.B, c *core.Core, n int) string {
	b.Helper()
	idR := sessions.Create(c, "bench thread")
	if !idR.OK {
		b.Fatalf("Create: %s", idR.Error())
	}
	id := idR.Value.(string)
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if r := sessions.Append(c, id, role, benchTurnContent); !r.OK {
			b.Fatalf("seed Append: %s", r.Error())
		}
	}
	return id
}

const benchTurnContent = "Can you summarise the last deploy and flag anything odd in the logs? " +
	"The last deploy shipped the sessions at-rest sealing change; logs show no errors."

func BenchmarkSessions_Append_Sealed(b *testing.B) {
	c := benchSessionsSealedCore(b)
	id := seedSession(b, c, 24)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if r := sessions.Append(c, id, "user", benchTurnContent); !r.OK {
			b.Fatalf("Append: %s", r.Error())
		}
	}
}

func BenchmarkSessions_Read_Sealed(b *testing.B) {
	c := benchSessionsSealedCore(b)
	id := seedSession(b, c, 24)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if r := sessions.Read(c, id); !r.OK {
			b.Fatalf("Read: %s", r.Error())
		}
	}
}
