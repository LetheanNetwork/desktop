// SPDX-Licence-Identifier: EUPL-1.2

// export_test.go — internal-to-test bridges for the marketplace
// package. The `_test.go` suffix scopes these symbols to the test
// binary only; production builds never see them.

package marketplace

import (
	"net/http"
	"sync"

	core "dappco.re/go"

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

// PersistManifestForTest exposes the same persistManifest helper that
// Install uses internally so external tests (the *_test package) can
// drive the manifest.yml write step without spinning up a real sandbox
// runtime (docker / apple container). Mantis #1689 HIGH (Cerberus #54
// C1).
//
// Usage example (test code):
//
//	dir := /* tmp dir */
//	r := subject.PersistManifestForTest(svc, dir, m)
//	core.AssertTrue(t, r.OK)
func PersistManifestForTest(s *Service, configPath string, m BundleManifest) interface{ Error() string } {
	r := s.persistManifest(configPath, m)
	if r.OK {
		return nil
	}
	if err, ok := r.Value.(error); ok {
		return err
	}
	return errString(r.Error())
}

// BundleConfigPathForTest exposes Service.bundleConfigPath so external
// tests can resolve the install-time config dir without re-encoding
// the ~/Lethean/conf/marketplace/<id>/ convention. Mantis #1689.
func BundleConfigPathForTest(s *Service, bundleID string) string {
	return s.bundleConfigPath(bundleID)
}

// ResolvePluginCodeForTest exposes Service.resolvePluginCode so
// external tests can verify Uninstall's plugin-code resolution walks
// the on-disk manifest (Cerberus #54 C1 — when manifest.yml is absent
// this silently falls back to bundle id, leaking ViewRegistry state on
// uninstall whenever plugin.code != bundle.Name).
func ResolvePluginCodeForTest(s *Service, bundleID string) string {
	return s.resolvePluginCode(bundleID)
}

// AllowedManifestHostSuffixesForTest returns a copy of the compile-time
// host-allowlist (Cerberus #54 C2 / Mantis #1690). External tests use
// this to assert the sorted-and-unique invariant without exporting the
// list itself into the production API surface.
func AllowedManifestHostSuffixesForTest() []string {
	out := make([]string, len(allowedManifestHostSuffixes))
	copy(out, allowedManifestHostSuffixes)
	return out
}

// FetchManifestWithSigForTest exposes the .sig + body ETag-pinned
// fetch primitive so external tests can drive the DREAD v2 N2 / Mantis
// #1650 rotation-race gate against an httptest.NewTLSServer().Client()
// without ripping the production httpsOnlyClient discipline out of
// the in-package call path.
//
// Usage example (test code):
//
//	srv := httptest.NewTLSServer(handler)
//	r := subject.FetchManifestWithSigForTest(srv.URL+"/x.yml", srv.Client())
//	if !r.OK { ... }
//	res, _ := r.Value.(subject.FetchManifestSignedResult)
func FetchManifestWithSigForTest(url string, client *http.Client) core.Result {
	return fetchManifestWithSigClient(fetchManifestOp, url, client)
}
