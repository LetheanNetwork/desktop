// SPDX-Licence-Identifier: EUPL-1.2

// export_test.go — internal-to-test bridges for the marketplace
// package. The `_test.go` suffix scopes these symbols to the test
// binary only; production builds never see them.

package marketplace

import (
	"sync"

	"dappco.re/lthn/desktop/pkg/sandbox"
)

// BundleMutexForTest exposes Service.bundleMutex so external tests
// (the *_test package) can assert the per-bundleID single-flight
// behaviour of Mantis #1583 without exporting the helper into the
// production API surface.
//
// Usage example (test code):
//
//	lock := subject.BundleMutexForTest(svc, "bundle-A")
//	lock.Lock(); defer lock.Unlock()
func BundleMutexForTest(s *Service, bundleID string) *sync.Mutex {
	return s.bundleMutex(bundleID)
}

// WriteIndexCacheForTest exposes the registry's atomic-write helper
// (Mantis #1582) so external tests can drive the cascade-adoption
// surface (compose + AtomicWriteWithVersion) without round-tripping
// through the network downloadIndex.
func WriteIndexCacheForTest(cachePath string, jsonBody []byte) interface{ Error() string } {
	r := writeIndexCache(cachePath, jsonBody)
	if r.OK {
		return nil
	}
	if err, ok := r.Value.(error); ok {
		return err
	}
	return errString(r.Error())
}

// ComposeCacheBodyForTest exposes composeCacheBody for external test
// inspection of the on-disk frontmatter shape (Mantis #1582).
func ComposeCacheBodyForTest(jsonBody []byte, version int) []byte {
	return composeCacheBody(jsonBody, version)
}

// StripCacheFrontmatterForTest exposes stripCacheFrontmatter for
// external test of the lazy-migration parser tolerance.
func StripCacheFrontmatterForTest(raw []byte) []byte {
	return stripCacheFrontmatter(raw)
}

// IndexCacheVersionForTest exposes the current stamped cache
// version constant (Mantis #1582).
func IndexCacheVersionForTest() int { return indexCacheVersion }

// BuildInstallSpawnInputForTest exposes buildInstallSpawnInput so
// external tests can verify the Install + Launch loops stamp
// InstallID + BundleID onto the sandbox.SpawnLongInput they hand
// SpawnLong (Mantis #1670 / H#187 follow-on for #1665) — without
// having to spin up the sandbox service or run a container.
//
// Usage example (test code):
//
//	got := subject.BuildInstallSpawnInputForTest(
//	    img, "echo", env, vols, "iid-deadbeef", "opencode",
//	)
//	core.AssertEqual(t, "iid-deadbeef", got.InstallID)
//	core.AssertEqual(t, "opencode",     got.BundleID)
func BuildInstallSpawnInputForTest(
	img ImageEntry,
	command string,
	env map[string]string,
	volumes []sandbox.LongVolumeMount,
	installID string,
	bundleID string,
) sandbox.SpawnLongInput {
	return buildInstallSpawnInput(buildInstallSpawnInputArgs{
		Img:       img,
		Command:   command,
		Env:       env,
		Volumes:   volumes,
		InstallID: installID,
		BundleID:  bundleID,
	})
}

// errString is the minimal error type used when a Result fails with
// a plain string (vs a typed envelope).
type errString string

func (e errString) Error() string { return string(e) }
