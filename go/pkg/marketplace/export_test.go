// SPDX-Licence-Identifier: EUPL-1.2

// export_test.go — internal-to-test bridges for the marketplace
// package. The `_test.go` suffix scopes these symbols to the test
// binary only; production builds never see them.

package marketplace

import "sync"

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
