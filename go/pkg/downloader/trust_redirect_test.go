// SPDX-Licence-Identifier: EUPL-1.2

// Cerberus #1424 — redirect-following must re-check AllowedSource on
// every hop. core.HTTPGet (= http.Get) used the default redirect
// policy which only validated the original URL; any 302 from an
// allowlisted mirror to evil.example.com pulled bytes from the latter.
//
// The httpClient in downloader.go installs a CheckRedirect that
// re-validates each hop. These tests verify both directions of the
// gate.

package downloader_test

import (
	"net/http/httptest"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/downloader"
)

// TestDownloader_Redirect_Bad_ToDisallowedHost — a 302 from an
// allowlisted host (httptest = 127.0.0.1) to an explicit non-
// allowlisted host must fail in CheckRedirect before any bytes are
// pulled from the disallowed source. The fetch returns Fail.
func TestDownloader_Redirect_Bad_ToDisallowedHost(t *core.T) {
	homeFixture(t)
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.Header().Set("Location", "https://attacker.example/payload.gguf")
		w.WriteHeader(302) // core.StatusFound not exported yet — gap-logged
	}))
	defer srv.Close()

	r := downloader.Fetch(srv.URL, modelGGUF)
	core.AssertFalse(t, r.OK,
		"redirect to disallowed host must fail via CheckRedirect")
}

// TestDownloader_Redirect_Good_BetweenAllowedHosts — two httptest
// servers (both 127.0.0.1, both allowlisted) with the first issuing
// 302 to the second. CheckRedirect accepts; bytes pulled from the
// second server land in the model dir.
func TestDownloader_Redirect_Good_BetweenAllowedHosts(t *core.T) {
	homeFixture(t)
	payload := []byte("HOP-2-PAYLOAD")
	hop2 := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payload)
	}))
	defer hop2.Close()

	hop1 := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.Header().Set("Location", hop2.URL+"/m.gguf")
		w.WriteHeader(302) // core.StatusFound not exported yet — gap-logged
	}))
	defer hop1.Close()

	r := downloader.Fetch(hop1.URL, "redirected.gguf")
	core.AssertTrue(t, r.OK,
		"redirect between two allowlisted hosts must succeed")
	got := core.ReadFile(r.Value.(string)).Value.([]byte)
	core.AssertEqual(t, string(payload), string(got))
}

// TestDownloader_Redirect_Bad_TooMany — a server that loops back to
// itself eleven times must hit the maxRedirects cap. Cerberus's
// remit was the trust gate, but the explicit cap (matching the Go
// default of 10) keeps this auditable in downloader.go rather than
// inherited from net/http.
func TestDownloader_Redirect_Bad_TooMany(t *core.T) {
	homeFixture(t)
	var srv *httptest.Server
	srv = httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, r *core.Request) {
		w.Header().Set("Location", srv.URL+r.URL.Path+"/r")
		w.WriteHeader(302) // core.StatusFound not exported yet — gap-logged
	}))
	defer srv.Close()

	r := downloader.Fetch(srv.URL, "loop.gguf")
	core.AssertFalse(t, r.OK, "infinite redirect loop must hit the cap")
}
