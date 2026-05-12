// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the inference-endpoint probe. Spins up an httptest server
// per case to drive the real http path through core.HTTPGet — no
// network mocking, the request/response round-trip is what we're
// actually validating.

package validator_test

import (
	"net/http"
	"net/http/httptest"
	"strings"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/validator"
)

func TestValidator_Endpoint_Good_2xx(t *core.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		core.AssertEqual(t, "/models", req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gemma-4-e2b"}]}`))
	}))
	defer srv.Close()

	r := validator.Endpoint(srv.URL)
	core.AssertTrue(t, r.OK)
	info := r.Value.(validator.EndpointInfo)
	core.AssertTrue(t, info.OK, "2xx should mark info.OK=true")
	core.AssertEqual(t, 200, info.Status)
	core.AssertTrue(t, strings.Contains(info.Body, "gemma-4-e2b"))
	core.AssertTrue(t, strings.HasSuffix(info.URL, "/models"))
}

func TestValidator_Endpoint_Good_TrailingSlashStripped(t *core.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		core.AssertEqual(t, "/models", req.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Trailing slash on the base URL should be trimmed before
	// /models is appended — verifies the TrimRight step.
	r := validator.Endpoint(srv.URL + "/")
	core.AssertTrue(t, r.OK)
	info := r.Value.(validator.EndpointInfo)
	core.AssertTrue(t, info.OK)
}

func TestValidator_Endpoint_Bad_404(t *core.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`not found`))
	}))
	defer srv.Close()

	r := validator.Endpoint(srv.URL)
	// Non-2xx is still core.Ok — the caller wants the diagnostic.
	core.AssertTrue(t, r.OK)
	info := r.Value.(validator.EndpointInfo)
	core.AssertFalse(t, info.OK, "404 should mark info.OK=false")
	core.AssertEqual(t, 404, info.Status)
}

func TestValidator_Endpoint_Bad_EmptyURL(t *core.T) {
	r := validator.Endpoint("")
	core.AssertFalse(t, r.OK, "empty URL must Fail")
}

func TestValidator_Endpoint_Bad_Unreachable(t *core.T) {
	// Reserved-for-documentation block (RFC 5737) — guarantees no
	// listener answers, so the GET fails before we get a Response.
	r := validator.Endpoint("http://198.51.100.1:1/")
	core.AssertTrue(t, r.OK, "unreachable host returns Ok with Status=0")
	info := r.Value.(validator.EndpointInfo)
	core.AssertFalse(t, info.OK)
	core.AssertEqual(t, 0, info.Status)
}
