// SPDX-Licence-Identifier: EUPL-1.2

package modelruntime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	core "dappco.re/go"
)

func TestHTTPClient_Health_GoodDecodesBoundedLEMResponse(t *core.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		core.AssertEqual(t, http.MethodGet, request.Method)
		core.AssertEqual(t, "/v1/health", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			writer,
			`{"status":"ok","runtime":"go-inference","models":[],"time":1785146400}`,
		)
	}))
	t.Cleanup(server.Close)
	client := newHTTPClient(server.URL, &http.Client{Timeout: time.Second})

	result := client.Health(context.Background())

	core.RequireTrue(t, result.OK, result.Error())
	health := result.Value.(Health)
	core.AssertEqual(t, "ok", health.Status)
	core.AssertEqual(t, "go-inference", health.Runtime)
	core.AssertEqual(t, int64(1785146400), health.Time)
	core.AssertLen(t, health.Models, 0)
}

func TestHTTPClient_Admin_GoodUsesBearerAndKeepsPathsInternal(t *core.T) {
	const token = "lthn-mlx_abcdefghijklmnopqrstuvwx"
	var captured ReloadCommand
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		core.AssertEqual(t, "Bearer "+token, request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/admin/machine":
			_, _ = io.WriteString(
				writer,
				`{"hash":"lem-machine","runtime":"go-inference"}`,
			)
		case "/v1/admin/serve/status":
			_, _ = io.WriteString(
				writer,
				`{"model_path":"/trusted/models/gemma","runtime":"metal","loaded_at_unix":1785146400,"config":{"context_length":4096}}`,
			)
		case "/v1/admin/serve/reload":
			raw, _ := io.ReadAll(request.Body)
			decoded := map[string]any{}
			core.RequireTrue(t, core.JSONUnmarshal(raw, &decoded).OK)
			captured = ReloadCommand{
				ConfirmMachine: decoded["confirm_machine"].(string),
				NativePath:     decoded["model_path"].(string),
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.Error(writer, "missing", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	client := newHTTPClient(server.URL, &http.Client{Timeout: time.Second})

	machineResult := client.Machine(context.Background(), token)
	statusResult := client.Status(context.Background(), token)
	reloadResult := client.Reload(context.Background(), token, ReloadCommand{
		ConfirmMachine: "lem-machine",
		NativePath:     "/trusted/models/gemma",
	})

	core.RequireTrue(t, machineResult.OK, machineResult.Error())
	core.AssertEqual(t, "lem-machine", machineResult.Value.(Machine).Hash)
	core.RequireTrue(t, statusResult.OK, statusResult.Error())
	core.AssertEqual(t, "metal", statusResult.Value.(Status).Runtime)
	core.RequireTrue(t, reloadResult.OK, reloadResult.Error())
	core.AssertEqual(t, ReloadCommand{
		ConfirmMachine: "lem-machine",
		NativePath:     "/trusted/models/gemma",
	}, captured)
	core.AssertNil(t, reloadResult.Value)
}

func TestHTTPClient_Response_BadRejectsNonJSONRedirectAndOversize(t *core.T) {
	tests := []struct {
		name string
		body string
		code int
		kind ClientErrorCode
	}{
		{
			name: "non-json",
			body: "healthy",
			code: http.StatusOK,
			kind: ClientResponseInvalid,
		},
		{
			name: "redirect",
			body: "",
			code: http.StatusTemporaryRedirect,
			kind: ClientResponseInvalid,
		},
		{
			name: "oversize",
			body: strings.Repeat("x", maxResponseBytes+1),
			code: http.StatusOK,
			kind: ClientResponseTooLarge,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *core.T) {
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				if test.name != "non-json" {
					writer.Header().Set("Content-Type", "application/json")
				}
				if test.name == "redirect" {
					writer.Header().Set("Location", "/elsewhere")
				}
				writer.WriteHeader(test.code)
				_, _ = io.WriteString(writer, test.body)
			}))
			t.Cleanup(server.Close)
			client := newHTTPClient(server.URL, boundedHTTPClient(time.Second))

			result := client.Health(context.Background())

			core.AssertFalse(t, result.OK)
			core.AssertEqual(t, test.kind, ClientErrorCodeOf(result))
		})
	}
}

func TestHTTPClient_Response_BadClassifiesUnauthorisedAndTimeout(t *core.T) {
	unauthorisedServer := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		http.Error(writer, `{"error":"denied"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(unauthorisedServer.Close)
	unauthorisedClient := newHTTPClient(
		unauthorisedServer.URL,
		boundedHTTPClient(time.Second),
	)
	unauthorised := unauthorisedClient.Status(context.Background(), "wrong")
	core.AssertFalse(t, unauthorised.OK)
	core.AssertEqual(t, ClientUnauthorised, ClientErrorCodeOf(unauthorised))

	timeoutServer := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		time.Sleep(50 * time.Millisecond)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"status":"ok"}`)
	}))
	t.Cleanup(timeoutServer.Close)
	timeoutClient := newHTTPClient(
		timeoutServer.URL,
		boundedHTTPClient(5*time.Millisecond),
	)
	timedOut := timeoutClient.Health(context.Background())
	core.AssertFalse(t, timedOut.OK)
	core.AssertEqual(t, ClientRequestTimeout, ClientErrorCodeOf(timedOut))
}

func TestNewClient_UglyPinsProductionLoopbackAndRejectsRedirects(t *core.T) {
	client := NewClient()
	core.AssertEqual(t, defaultLEMBaseURL, client.baseURL)
	core.RequireTrue(t, client.client != nil)

	request, _ := http.NewRequest(http.MethodGet, defaultLEMBaseURL+"/elsewhere", nil)
	redirect := client.client.CheckRedirect(request, nil)
	core.AssertEqual(t, http.ErrUseLastResponse, redirect)
}
