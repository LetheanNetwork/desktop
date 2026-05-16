// SPDX-Licence-Identifier: EUPL-1.2

// audit.go — Stage F audit-HMAC-secret derivation per
// RFC.stage-f.md §6.4. The audit recorder hashes account_id values
// at-rest under an HMAC-SHA256 key sourced from this helper. Sharing
// the .seed root with the bootstrap-token path (HKDF-SHA256, distinct
// info string for domain separation) means the audit secret is
// stable across process restarts AND backed by the same machine-
// local entropy the user already trusted with their server-key.
//
// Per RFC §6.4 — backup-readers (Time Machine, Backblaze, iCloud
// sync) read the audit log without same-UID access; hashing
// account_id at-rest defends against enumeration of which accounts
// exist via backup tail. The HMAC secret MUST be unique-per-machine
// (the .seed bytes ensure this) so even an attacker who knows the
// HKDF info string can't pre-compute account_id → hash mappings
// off-host.

package serverkey

import (
	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

// auditHKDFInfo is the HKDF info string that domain-separates the
// audit-HMAC derivation from the bootstrap-token passphrase derivation
// (which uses hkdfInfo = "passphrase" — see types.go). Same .seed
// root, distinct purpose strings, independent output bytes.
var auditHKDFInfo = []byte("audit-account-id-hmac")

// auditHMACKeyLen is the byte-length of the derived HMAC secret. 32
// bytes = 256 bits — matches HMAC-SHA256's natural key size.
const auditHMACKeyLen = 32

// AuditHMACSecret returns the 32-byte HMAC-SHA256 key the audit
// recorder uses to hash account_id values at-rest. Derived via
// HKDF-SHA256 over ~/Lethean/wallets/.seed using the auth-gate's
// existing hkdfSalt + a distinct info string ("audit-account-id-hmac")
// so the bytes are stable across process restarts.
//
// On first-run the .seed file may not exist yet — Bootstrap() creates
// it (idempotently). Callers SHOULD invoke AuditHMACSecret AFTER the
// boot path has called Bootstrap; calling earlier returns an empty
// slice + a warning via core.Print so the audit substrate falls back
// to its process-local random secret (per audit.New degraded path)
// rather than blocking the boot sequence.
//
// Returns nil + warning on any error reading the seed — audit
// failures MUST NEVER block boot.
//
// Usage example (boot wiring):
//
//	if r := serverkeySvc.Bootstrap(); !r.OK { return r }
//	auditSecret := serverkeySvc.AuditHMACSecret()
//	auditSvc := audit.New(c, audit.Options{AuditSecret: auditSecret})
//	audit.SetDefault(auditSvc)
func (s *Service) AuditHMACSecret() []byte {
	walletsR := paths.WalletsDir()
	if !walletsR.OK {
		core.Print(core.Stderr(),
			"serverkey.AuditHMACSecret: wallets dir resolve failed: %s\n",
			walletsR.Error())
		return nil
	}
	walletsDir, _ := walletsR.Value.(string)
	seedPath := core.PathJoin(walletsDir, seedFile)

	readR := core.ReadFile(seedPath)
	if !readR.OK {
		core.Print(core.Stderr(),
			"serverkey.AuditHMACSecret: read seed failed (call Bootstrap first): %s\n",
			readR.Error())
		return nil
	}
	seed, _ := readR.Value.([]byte)
	if len(seed) != seedSize {
		core.Print(core.Stderr(),
			"serverkey.AuditHMACSecret: seed has wrong length (%d != %d)\n",
			len(seed), seedSize)
		return nil
	}
	r := core.HKDF("sha256", seed, hkdfSalt, auditHKDFInfo, auditHMACKeyLen)
	if !r.OK {
		core.Print(core.Stderr(),
			"serverkey.AuditHMACSecret: HKDF derive failed: %s\n", r.Error())
		return nil
	}
	out, _ := r.Value.([]byte)
	return out
}
