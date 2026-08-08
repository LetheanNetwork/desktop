// SPDX-Licence-Identifier: EUPL-1.2

// transfer_test.go — fault-arm coverage for the internal-namespace
// initialisation that Register drives per mount. Each arm is injected
// through the shared failingMedium double; the adoption arms use a
// pre-seeded memory medium. Warnings are the contract on every fault
// path — the mount stays usable, only internalReady stays false.

package files

import (
	"io/fs"
	"testing"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// registerWithMedium composes a single Trash-capable mount over the
// supplied medium and runs the real Register path.
func registerWithMedium(t *testing.T, medium coreio.Medium) *Service {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		core.AssertTrue(t, c.ServiceShutdown(core.Background()).OK)
	})
	service := NewService(Options{
		Mounts:  []Mount{memoryMount("docs", medium, ReadWriteCapabilities())},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)
	return service
}

// TestTransfer_InitialiseInternalNamespace_Good — a namespace this
// service owns from an earlier run is adopted, and a fresh medium
// gets the namespace created with the owner marker.
func TestTransfer_InitialiseInternalNamespace_Good(t *testing.T) {
	owned := coreio.NewMemoryMedium()
	core.RequireNoError(t, owned.EnsureDir(internalNamespace))
	core.RequireNoError(t, owned.WriteMode(internalOwnerPath, internalOwnerDocument, 0600))
	core.AssertTrue(t, registerWithMedium(t, owned).internalReady["docs"])

	core.AssertTrue(t, registerWithMedium(t, coreio.NewMemoryMedium()).internalReady["docs"])
}

// TestTransfer_InitialiseInternalNamespace_Bad — a Stat failure that
// is not NotExist, a failed namespace creation, and a failed owner
// marker each leave the namespace unavailable without failing
// registration.
func TestTransfer_InitialiseInternalNamespace_Bad(t *testing.T) {
	statBroken := &failingMedium{Medium: coreio.NewMemoryMedium(), statErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, statBroken).internalReady["docs"])

	dirBroken := &failingMedium{Medium: coreio.NewMemoryMedium(), ensureDirErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, dirBroken).internalReady["docs"])

	markerBroken := &failingMedium{Medium: coreio.NewMemoryMedium(), writeModeErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, markerBroken).internalReady["docs"])
}

// TestTransfer_InitialiseInternalNamespace_Ugly — an existing
// namespace with a foreign (or unreadable) owner marker is never
// adopted: the service must not treat someone else's directory as
// its trash substrate.
func TestTransfer_InitialiseInternalNamespace_Ugly(t *testing.T) {
	foreign := coreio.NewMemoryMedium()
	core.RequireNoError(t, foreign.EnsureDir(internalNamespace))
	core.RequireNoError(t, foreign.WriteMode(internalOwnerPath, `{"owner":"someone-else"}`, 0600))
	core.AssertFalse(t, registerWithMedium(t, foreign).internalReady["docs"])

	unreadable := coreio.NewMemoryMedium()
	core.RequireNoError(t, unreadable.EnsureDir(internalNamespace))
	core.RequireNoError(t, unreadable.WriteMode(internalOwnerPath, internalOwnerDocument, 0600))
	wrapped := &failingMedium{Medium: unreadable, readErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, wrapped).internalReady["docs"])
}
