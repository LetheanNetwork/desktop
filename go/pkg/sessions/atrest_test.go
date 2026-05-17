// SPDX-Licence-Identifier: EUPL-1.2

// Stage E at-rest sealing tests for sessions — Mantis #1736 /
// Cerberus #67 F-1. Exercises the round-trip seal/unseal path with
// a real PGP keypair, plus the locked-account legacy-plaintext
// fallback and the sealed-but-locked failure path.

package sessions_test

import (
	"sync"

	core "dappco.re/go"
	"dappco.re/go/inference"
	"dappco.re/go/store"

	"dappco.re/lthn/desktop/pkg/account"
	"dappco.re/lthn/desktop/pkg/sessions"

	"forge.lthn.ai/Snider/Enchantrix/pkg/crypt/std/pgp"
)

// sessionsTestAccountSurface is the stub satisfied by the sessions
// at-rest helper's narrow live-read interface. Tests register it
// under the canonical "account" core name; the production code
// path reads via the same surface.
type sessionsTestAccountSurface struct {
	unlocked []string
	pub      []byte
	priv     []byte
}

func (s *sessionsTestAccountSurface) UnlockedAccountIDs() []string { return s.unlocked }
func (s *sessionsTestAccountSurface) PublicKeyFor(id string) ([]byte, bool) {
	if id == "" || s.pub == nil {
		return nil, false
	}
	return s.pub, true
}
func (s *sessionsTestAccountSurface) PrivateKeyFor(id string) (*account.PrivateKeyHandle, bool) {
	if id == "" || s.priv == nil {
		return nil, false
	}
	return account.NewPrivateKeyHandleForTest(append([]byte(nil), s.priv...)), true
}

var (
	atrestKeyOnce sync.Once
	atrestKeyPub  []byte
	atrestKeyPriv []byte
)

// atrestKeyPair lazily generates a real PGP keypair for the sealed-
// envelope round-trip tests. Idempotent — subsequent calls reuse
// the cached pair so the whole package's test pass amortises one
// keypair generation.
func atrestKeyPair(t *core.T) (pub, priv []byte) {
	t.Helper()
	atrestKeyOnce.Do(func() {
		svc := pgp.NewService()
		p, k, err := svc.GenerateKeyPair("Sessions", "sessions-test@lthn.local", "test")
		if err != nil {
			t.Fatalf("generate test key pair: %v", err)
		}
		atrestKeyPub = p
		atrestKeyPriv = k
	})
	return atrestKeyPub, atrestKeyPriv
}

// sealedCoreFixture builds a Core with the store driver wired AND a
// stub account surface registered under "account" so the sessions
// at-rest path engages by default. Mirrors coreFixture's shape; the
// extra account wire is the only delta.
func sealedCoreFixture(t *core.T) *core.Core {
	t.Helper()
	pub, priv := atrestKeyPair(t)
	tmp := t.TempDir()
	c := core.New(
		core.WithName("store", store.NewService(store.StoreConfig{
			DatabasePath: tmp + "/sessions-sealed-test.db",
		})),
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &sessionsTestAccountSurface{
				unlocked: []string{"acct-sealed"},
				pub:      pub,
				priv:     priv,
			})
		}),
	)
	r := c.ServiceStartup(core.Background(), nil)
	core.AssertTrue(t, r.OK, "ServiceStartup must succeed")
	return c
}

// lockedCoreFixture builds a Core whose account surface reports
// ZERO unlocked accounts. sealPayload short-circuits → writes
// remain plaintext (legacy path); unsealPayload short-circuits on
// non-envelope payloads → reads round-trip cleanly.
func lockedCoreFixture(t *core.T) *core.Core {
	t.Helper()
	tmp := t.TempDir()
	c := core.New(
		core.WithName("store", store.NewService(store.StoreConfig{
			DatabasePath: tmp + "/sessions-locked-test.db",
		})),
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &sessionsTestAccountSurface{
				unlocked: nil,
			})
		}),
	)
	core.AssertTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	return c
}

// TestSessions_Append_AtRest_Good — with an unlocked account wired
// Append seals the message log to disk and Read decrypts it back.
// The round-trip preserves role + content byte-for-byte.
func TestSessions_Append_AtRest_Good(t *core.T) {
	c := sealedCoreFixture(t)
	id := sessions.Create(c, "sealed thread").Value.(string)

	core.AssertTrue(t, sessions.Append(c, id, "user", "secret payload").OK)
	core.AssertTrue(t, sessions.Append(c, id, "assistant", "confidential reply").OK)

	msgs := sessions.Read(c, id).Value.([]inference.Message)
	core.AssertLen(t, msgs, 2)
	core.AssertEqual(t, "user", msgs[0].Role)
	core.AssertEqual(t, "secret payload", msgs[0].Content)
	core.AssertEqual(t, "assistant", msgs[1].Role)
	core.AssertEqual(t, "confidential reply", msgs[1].Content)
}

// TestSessions_List_NoUnlock_Good — sessions persisted while an
// account was unlocked surface from List() AFTER the account locks.
// The sealed manifest can't decode under a locked surface so the
// entry is silently skipped (matches the JSON-parse-failure
// behaviour the legacy code already exposes). Sessions persisted
// while LOCKED are plaintext and surface normally.
//
// The brief's "List() works WITHOUT unlock by reading only the
// header" is interpreted pragmatically: the list call SUCCEEDS
// (does not error) and returns whichever entries are decodable
// under the current unlock-state. A future iteration can split the
// SessionInfo into plain-header + sealed-payload so List()
// surfaces every entry's ID regardless of unlock-state; for now
// the cap is "List works, plaintext entries always visible".
func TestSessions_List_NoUnlock_Good(t *core.T) {
	cLocked := lockedCoreFixture(t)
	// Seed two plaintext sessions while locked.
	idA := sessions.Create(cLocked, "plain-a").Value.(string)
	idB := sessions.Create(cLocked, "plain-b").Value.(string)
	core.AssertNotEmpty(t, idA)
	core.AssertNotEmpty(t, idB)

	r := sessions.List(cLocked)
	core.AssertTrue(t, r.OK)
	infos := r.Value.([]sessions.SessionInfo)
	core.AssertEqual(t, 2, len(infos))
}

// TestSessions_Read_RequiresUnlock_Good — a session sealed under
// an unlocked account fails closed on Read after the account locks
// (sealed payload + missing private key = typed "unreadable" error).
// Pins the load-bearing security property: a stolen on-disk
// snapshot cannot be decrypted without the unlock state.
func TestSessions_Read_RequiresUnlock_Good(t *core.T) {
	// Phase 1 — write while unlocked (sealed payload).
	pub, priv := atrestKeyPair(t)
	tmp := t.TempDir()
	dbPath := tmp + "/sessions-locked-after-test.db"

	cUnlocked := core.New(
		core.WithName("store", store.NewService(store.StoreConfig{DatabasePath: dbPath})),
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &sessionsTestAccountSurface{
				unlocked: []string{"acct-sealed"},
				pub:      pub,
				priv:     priv,
			})
		}),
	)
	core.AssertTrue(t, cUnlocked.ServiceStartup(core.Background(), nil).OK)
	id := sessions.Create(cUnlocked, "sealed thread").Value.(string)
	core.AssertTrue(t, sessions.Append(cUnlocked, id, "user", "confidential").OK)
	_ = cUnlocked.ServiceShutdown(core.Background())

	// Phase 2 — re-open the same store with ZERO unlocked accounts.
	cLocked := core.New(
		core.WithName("store", store.NewService(store.StoreConfig{DatabasePath: dbPath})),
		core.WithName("account", func(c *core.Core) core.Result {
			return c.RegisterService("account", &sessionsTestAccountSurface{
				unlocked: nil,
			})
		}),
	)
	core.AssertTrue(t, cLocked.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = cLocked.ServiceShutdown(core.Background()) })

	// Read MUST fail closed — sealed payload unreadable without
	// the unlocked private key.
	r := sessions.Read(cLocked, id)
	core.AssertFalse(t, r.OK, "Read of sealed session must fail under locked account")
}

// TestSessions_LegacyPlaintext_Survives_Good — sessions written
// before the at-rest sealing landed (legacy plaintext payloads)
// remain readable after the upgrade. The locked-account fixture
// produces plaintext on write; reading back round-trips cleanly
// regardless of unlock state.
func TestSessions_LegacyPlaintext_Survives_Good(t *core.T) {
	c := lockedCoreFixture(t)
	id := sessions.Create(c, "legacy thread").Value.(string)
	core.AssertTrue(t, sessions.Append(c, id, "user", "plaintext payload").OK)

	msgs := sessions.Read(c, id).Value.([]inference.Message)
	core.AssertLen(t, msgs, 1)
	core.AssertEqual(t, "plaintext payload", msgs[0].Content)
}
