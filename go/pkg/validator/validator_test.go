// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the inference-endpoint probe. Spins up an httptest server
// per case to drive the real http path through core.HTTPGet — no
// network mocking, the request/response round-trip is what we're
// actually validating.

package validator_test

import (
	"net"
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/validator"
)

const modelsPath = "/models"

func TestValidator_Endpoint_Good_2xx(t *core.T) {
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, req *core.Request) {
		core.AssertEqual(t, modelsPath, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gemma-4-e2b"}]}`))
	}))
	defer srv.Close()

	r := validator.Endpoint(srv.URL)
	core.AssertTrue(t, r.OK)
	info := r.Value.(validator.EndpointInfo)
	core.AssertTrue(t, info.OK, "2xx should mark info.OK=true")
	core.AssertEqual(t, 200, info.Status)
	core.AssertTrue(t, core.Contains(info.Body, "gemma-4-e2b"))
	core.AssertTrue(t, core.HasSuffix(info.URL, modelsPath))
}

func TestValidator_Endpoint_Good_TrailingSlashStripped(t *core.T) {
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, req *core.Request) {
		core.AssertEqual(t, modelsPath, req.URL.Path)
		w.WriteHeader(core.StatusOK)
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
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(core.StatusNotFound)
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
	core.AssertNotEmpty(t, r.Error())
}

// TestValidator_Endpoint_Bad_BodyReadTruncated hijacks the connection
// and claims a Content-Length far larger than the bytes actually
// written, then closes the socket. The client's body read gets
// io.ErrUnexpectedEOF (not io.EOF), driving the readAllSized
// non-EOF-error path in core.ReadAll and Endpoint's "read body
// failed" branch — the one line real requests never hit.
func TestValidator_Endpoint_Bad_BodyReadTruncated(t *core.T) {
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("ResponseWriter does not support hijacking")
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 4096\r\n\r\nshort")
		_ = buf.Flush()
		_ = conn.(*net.TCPConn).CloseWrite()
	}))
	defer srv.Close()

	r := validator.Endpoint(srv.URL)
	core.AssertFalse(t, r.OK, "truncated body must fail ReadAll")
	core.AssertNotEmpty(t, r.Error())
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
