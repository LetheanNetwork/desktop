// SPDX-Licence-Identifier: EUPL-1.2

// Internal tests for probe.go — doProbe is unexported. probeClient
// is swapped to a hermetic httptest.NewTLSServer client for the
// duration of each test (same seam as service_internal_test.go and
// pr_fetch_internal_test.go); no production code changes.

package vi

import (
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
)

// TestProbe_DoProbe_Good — a real TLS round-trip against a hermetic
// server returns the response status with no transport error, and
// drains the (bounded) body so the connection can be reused.
func TestProbe_DoProbe_Good(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthy"))
	}))
	defer ts.Close()

	orig := probeClient
	probeClient = ts.Client()
	t.Cleanup(func() { probeClient = orig })

	status, errMsg := doProbe(ts.URL)
	core.AssertEqual(t, 200, status)
	core.AssertEqual(t, "", errMsg)
}

// TestProbe_DoProbe_Bad — an unreachable https endpoint surfaces a
// transport error rather than panicking. No client swap — the
// default client dialling a closed local port fails fast.
func TestProbe_DoProbe_Bad(t *core.T) {
	status, errMsg := doProbe("https://127.0.0.1:1/")
	core.AssertEqual(t, 0, status)
	core.AssertNotEmpty(t, errMsg)
}

// TestProbe_DoProbe_Ugly — a non-2xx response is still a completed
// transport round-trip: doProbe reports the real status code with
// no error, leaving OK-derivation to the caller (Probe).
func TestProbe_DoProbe_Ugly(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	orig := probeClient
	probeClient = ts.Client()
	t.Cleanup(func() { probeClient = orig })

	status, errMsg := doProbe(ts.URL)
	core.AssertEqual(t, 503, status)
	core.AssertEqual(t, "", errMsg)
}

// TestProbe_Probe_Good — a full Probe() round-trip against a
// hermetic server persists an OK=true SiteProbe row and fires the
// Succeeded (not Failed) audit event.
func TestProbe_Probe_Good(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	orig := probeClient
	probeClient = ts.Client()
	t.Cleanup(func() { probeClient = orig })

	c := newFetchInternalCore(t)
	r := Probe(c, SiteCatalogue{URL: ts.URL})
	core.RequireTrue(t, r.OK)
	probe, ok := r.Value.(SiteProbe)
	core.RequireTrue(t, ok)
	core.AssertTrue(t, probe.OK)
	core.AssertEqual(t, 200, probe.StatusCode)
	core.AssertEqual(t, "", probe.LastError)
}
