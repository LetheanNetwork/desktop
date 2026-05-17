// SPDX-Licence-Identifier: EUPL-1.2

// Per-install identifier for the sandbox host — mirrors the
// `opencode.Service.InstallID` shape (pkg/opencode/auth.go).
//
// Why a sandbox-side InstallID exists:
//   - SpawnLong stamps `lthn.sandbox.install_id` on every long-
//     running container (see pkg/sandbox/sandbox_long.go added
//     by Mantis #1665 / H#187). The label is what a future
//     reconcile pass uses to gate "which surviving containers
//     is it safe for THIS lthn install to adopt" — same threat
//     model as opencode's `lthn.opencode.install_id` (Mantis
//     #1599 Cerberus #22): without the gate, a sibling user on
//     the host could spawn a look-alike `lthn-bundle-*`
//     container and have lthn front it.
//   - The value lives in the sandbox-side KV (one DuckDB file at
//     ~/Lethean/data/sandbox.duckdb) so the marketplace install
//     loop can request it without taking a dependency on the
//     opencode package or the legacy pkg/keys chacha20 substrate
//     that Mantis #1614 is retiring.

package sandbox

import (
	core "dappco.re/go"
	goiostore "dappco.re/go/io/store"
)

const (
	// installIDStoreGroup is the DuckDB group under which the
	// per-install identifier lives. Distinct from the opencode
	// group ("opencode.install") so the two namespaces don't
	// collide if both KV files ever share a store.
	installIDStoreGroup = "sandbox.install"
	// installIDKey is the key inside installIDStoreGroup.
	installIDKey = "install_id"
	// installIDBytes is the random source width — 16 bytes
	// becomes a 32-char hex string. The identifier is a non-
	// secret tag (it shows up in `docker inspect --format
	// '{{.Config.Labels}}'`); 128 bits is plenty to avoid
	// collisions between sibling lthn installs on the same host.
	installIDBytes = 16
	// installIDKVPath is the on-disk location of the sandbox KV
	// store, rooted at the user's home directory. Separate file
	// from opencode.duckdb so a corrupt sandbox KV can't take
	// the opencode profile state down with it.
	installIDKVPath = "Lethean/data/sandbox.duckdb"

	installIDOp = "sandbox.InstallID"
	installIDKv = "sandbox.kv"
)

// kvOnce + kvInst lazily initialise the sandbox-side DuckDB-
// backed KV store on first access. Process-singleton — concurrent
// callers don't race the DuckDB file open thanks to core.Once.
var (
	kvOnce core.Once
	kvErr  error
	kvInst *goiostore.KeyValueStore
)

// kv lazily opens the DuckDB-backed KV store at the
// installIDKVPath under the user's home dir.
//
// Usage example:
//
//	st, r := kv()
//	if !r.OK { return r }
//	_, _ = st.Get(installIDStoreGroup, installIDKey)
func kv() (*goiostore.KeyValueStore, core.Result) {
	kvOnce.Do(func() {
		homeR := core.UserHomeDir()
		if !homeR.OK {
			kvErr = core.E(installIDKv, "home dir resolve failed", nil)
			return
		}
		path := core.PathJoin(homeR.Value.(string), installIDKVPath)
		parent := core.PathDir(path)
		// store.New won't mkdir — ensure parent exists first.
		_ = core.MkdirAll(parent, 0o755)
		store, err := goiostore.New(goiostore.Options{Path: path})
		if err != nil {
			kvErr = err
			return
		}
		kvInst = store
	})
	if kvErr != nil {
		return nil, core.Fail(kvErr)
	}
	if kvInst == nil {
		return nil, core.Fail(core.E(installIDKv, "store not initialised", nil))
	}
	return kvInst, core.Ok(nil)
}

// InstallID returns the persisted per-install identifier,
// generating + storing a new one on first call. Idempotent —
// subsequent calls return the same value.
//
// The identifier is used as the value of the
// `lthn.sandbox.install_id` docker label SpawnLong stamps on
// every long-running container, and (once a reconcile pass is
// added) as the gate that decides which surviving containers
// the current lthn install may safely adopt. See the package-
// level note for the full threat-model rationale.
//
// Usage example:
//
//	r := svc.InstallID()
//	if r.OK { id := r.Value.(string); _ = id }
func (s *Service) InstallID() core.Result {
	st, r := kv()
	if !r.OK {
		return r
	}
	if existing, err := st.Get(installIDStoreGroup, installIDKey); err == nil && existing != "" {
		return core.Ok(existing)
	} else if err != nil && !core.Is(err, goiostore.NotFoundError) {
		return core.Fail(err)
	}
	// First call ever — generate + persist.
	buf := make([]byte, installIDBytes)
	if r := core.RandRead(buf); !r.OK {
		return core.Fail(core.E(installIDOp, "rand read failed", r.Value.(error)))
	}
	id := core.HexEncode(buf)
	if err := st.Set(installIDStoreGroup, installIDKey, id); err != nil {
		return core.Fail(err)
	}
	return core.Ok(id)
}
