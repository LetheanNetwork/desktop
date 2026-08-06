// SPDX-Licence-Identifier: EUPL-1.2

// client_internal_test.go — fault injection + direct accessor cover
// for client.go that client_test.go's happy/near-happy httptest
// scenarios don't reach: the ClientFailure error-interface methods
// called directly, the nil-client / zero-timeout constructor
// defaults, every "LEM sent a structurally invalid response" branch
// in Health/Status/Machine/Reload, and the non-timeout transport
// failure path (connection refused).

package modelruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	core "dappco.re/go"
)

func TestClientFailure_Error_Good(t *core.T) {
	failure := &ClientFailure{Message: "boom"}
	core.AssertEqual(t, "boom", failure.Error())
}

func TestClientFailure_Error_NilReceiver_Bad(t *core.T) {
	var failure *ClientFailure
	core.AssertEqual(t, "", failure.Error())
}

func TestClientFailure_Unwrap_NilReceiver_Bad(t *core.T) {
	var failure *ClientFailure
	core.AssertTrue(t, failure.Unwrap() == nil)
}

func TestClientErrorCodeOf_OKResult_Good(t *core.T) {
	core.AssertEqual(t, ClientErrorCode(""), ClientErrorCodeOf(core.Ok(nil)))
}

func TestClientErrorCodeOf_ForeignError_Bad(t *core.T) {
	foreign := core.Fail(core.NewError("unrelated"))
	core.AssertEqual(t, ClientErrorCode(""), ClientErrorCodeOf(foreign))
}

// TestNewHTTPClient_NilClient_DefaultsToBounded_Good — passing a nil
// *http.Client must not panic; newHTTPClient substitutes the default
// bounded client instead of storing nil.
func TestNewHTTPClient_NilClient_DefaultsToBounded_Good(t *core.T) {
	client := newHTTPClient("http://127.0.0.1:1", nil)
	core.RequireTrue(t, client.client != nil)
	core.AssertEqual(t, defaultHTTPTimeout, client.client.Timeout)
}

func TestBoundedHTTPClient_ZeroTimeout_DefaultsToBounded_Good(t *core.T) {
	client := boundedHTTPClient(0)
	core.AssertEqual(t, defaultHTTPTimeout, client.Timeout)
}

func TestBoundedHTTPClient_NegativeTimeout_DefaultsToBounded_Good(t *core.T) {
	client := boundedHTTPClient(-time.Second)
	core.AssertEqual(t, defaultHTTPTimeout, client.Timeout)
}

func jsonServer(t *core.T, body string) *HTTPClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return newHTTPClient(server.URL, &http.Client{Timeout: time.Second})
}

func TestHTTPClient_Health_Bad_EmptyStatus(t *core.T) {
	client := jsonServer(t, `{"status":"","runtime":"go-inference","models":[]}`)
	result := client.Health(context.Background())
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Health_Bad_StatusTooLong(t *core.T) {
	client := jsonServer(t, `{"status":"`+strings.Repeat("x", 40)+`","runtime":"go-inference"}`)
	result := client.Health(context.Background())
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Health_Bad_RuntimeTooLong(t *core.T) {
	client := jsonServer(t, `{"status":"ok","runtime":"`+strings.Repeat("x", 200)+`"}`)
	result := client.Health(context.Background())
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Health_Bad_TooManyModels(t *core.T) {
	models := make([]string, 0, 513)
	for index := 0; index < 513; index++ {
		models = append(models, `"m"`)
	}
	body := `{"status":"ok","runtime":"go-inference","models":[` + strings.Join(models, ",") + `]}`
	client := jsonServer(t, body)
	result := client.Health(context.Background())
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Health_Bad_InvalidModelText(t *core.T) {
	client := jsonServer(t, `{"status":"ok","runtime":"go-inference","models":[""]}`)
	result := client.Health(context.Background())
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Status_Bad_InvalidRuntimeText(t *core.T) {
	client := jsonServer(t, `{"model_path":"","runtime":"","loaded_at_unix":0,"config":{}}`)
	result := client.Status(context.Background(), "token")
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Status_Bad_NegativeLoadedAt(t *core.T) {
	client := jsonServer(t, `{"model_path":"","runtime":"metal","loaded_at_unix":-1,"config":{}}`)
	result := client.Status(context.Background(), "token")
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Status_Bad_NegativeContextLength(t *core.T) {
	client := jsonServer(t, `{"model_path":"","runtime":"metal","loaded_at_unix":0,"config":{"context_length":-1}}`)
	result := client.Status(context.Background(), "token")
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Machine_Bad_InvalidHash(t *core.T) {
	client := jsonServer(t, `{"hash":"","runtime":"go-inference"}`)
	result := client.Machine(context.Background(), "token")
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Machine_Bad_InvalidRuntime(t *core.T) {
	client := jsonServer(t, `{"hash":"lem-machine","runtime":""}`)
	result := client.Machine(context.Background(), "token")
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Reload_Bad_InvalidConfirmMachine(t *core.T) {
	client := jsonServer(t, `{}`)
	result := client.Reload(context.Background(), "token", ReloadCommand{
		ConfirmMachine: "",
		NativePath:     "/trusted/models/gemma",
	})
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

func TestHTTPClient_Reload_Bad_InvalidNativePath(t *core.T) {
	client := jsonServer(t, `{}`)
	result := client.Reload(context.Background(), "token", ReloadCommand{
		ConfirmMachine: "lem-machine",
		NativePath:     "",
	})
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

// TestHTTPClient_DoJSON_NilClient_Bad — a zero-value *HTTPClient (no
// underlying http.Client, no baseURL) must fail-closed rather than
// nil-deref.
func TestHTTPClient_DoJSON_NilClient_Bad(t *core.T) {
	var client *HTTPClient
	result := client.doJSON(context.Background(), http.MethodGet, "/v1/health", "", nil, nil)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientUnavailable, ClientErrorCodeOf(result))
}

func TestHTTPClient_DoJSON_EmptyBaseURL_Bad(t *core.T) {
	client := newHTTPClient("", &http.Client{Timeout: time.Second})
	result := client.doJSON(context.Background(), http.MethodGet, "/v1/health", "", nil, nil)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientUnavailable, ClientErrorCodeOf(result))
}

// TestHTTPClient_DoJSON_NilContext_DefaultsToBackground_Good — a nil
// core.Context must not panic; doJSON substitutes core.Background().
func TestHTTPClient_DoJSON_NilContext_DefaultsToBackground_Good(t *core.T) {
	client := jsonServer(t, `{"status":"ok","runtime":"go-inference","models":[]}`)
	var health Health
	result := client.doJSON(nil, http.MethodGet, "/v1/health", "", nil, &health)
	core.RequireTrue(t, result.OK, result.Error())
}

// TestHTTPClient_DoJSON_InvalidMethod_Bad — a method string containing
// a control character makes http.NewRequestWithContext itself fail,
// exercising the "request is invalid" branch that a hardcoded
// http.MethodGet/Post call site never reaches.
func TestHTTPClient_DoJSON_InvalidMethod_Bad(t *core.T) {
	client := newHTTPClient("http://127.0.0.1:1", &http.Client{Timeout: time.Second})
	result := client.doJSON(context.Background(), "BAD METHOD\n", "/v1/health", "", nil, nil)
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientResponseInvalid, ClientErrorCodeOf(result))
}

// TestClientTransportFailure_ConnectionRefused_Bad — dialling a closed
// local port (server started then immediately closed, so the port is
// guaranteed free of any other listener) produces a non-timeout
// transport error, exercising clientTransportFailure's ClientUnavailable
// fallback branch distinct from the DeadlineExceeded/Timeout() path.
func TestClientTransportFailure_ConnectionRefused_Bad(t *core.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := server.URL
	server.Close()

	client := newHTTPClient(closedURL, &http.Client{Timeout: time.Second})
	result := client.Health(context.Background())
	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, ClientUnavailable, ClientErrorCodeOf(result))
}

func TestInvalidProtocolText_AllBranches(t *core.T) {
	core.AssertTrue(t, invalidProtocolText("", 10))
	core.AssertTrue(t, invalidProtocolText(strings.Repeat("a", 11), 10))
	core.AssertTrue(t, invalidProtocolText("bad\x00text", 100))
	core.AssertTrue(t, invalidProtocolText("bad\x7ftext", 100))
	core.AssertFalse(t, invalidProtocolText("fine", 10))
}
