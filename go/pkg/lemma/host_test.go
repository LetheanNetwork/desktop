// SPDX-License-Identifier: EUPL-1.2

package lemma

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- NewHost ---

func TestHost_NewHost_Good(t *testing.T) {
	h := NewHost(HostConfig{BaseURL: "http://127.0.0.1:9100", Timeout: 5 * time.Second})
	if h == nil {
		t.Fatal("NewHost returned nil")
	}
	if h.baseURL != "http://127.0.0.1:9100" {
		t.Fatalf("baseURL = %q, want the configured value", h.baseURL)
	}
}

func TestHost_NewHost_Bad_ZeroValueAppliesDefaults(t *testing.T) {
	h := NewHost(HostConfig{})
	if h.baseURL != DefaultHostBaseURL {
		t.Fatalf("baseURL = %q, want default %q", h.baseURL, DefaultHostBaseURL)
	}
	if h.client == nil {
		t.Fatal("client must default, not be nil")
	}
	if h.client.Timeout != DefaultHostTimeout {
		t.Fatalf("client timeout = %v, want default %v", h.client.Timeout, DefaultHostTimeout)
	}
}

func TestHost_NewHost_Ugly_CustomClientPreserved(t *testing.T) {
	custom := &http.Client{Timeout: 3 * time.Second}
	h := NewHost(HostConfig{Client: custom, Timeout: 3 * time.Second})
	if h.client != custom {
		t.Fatal("a caller-supplied client must be preserved, not replaced")
	}
}

// --- Serve ---

func TestHost_Serve_Good(t *testing.T) {
	var captured HostServeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/driver/serve" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(hostServeResponse{OK: true})
	}))
	defer srv.Close()

	h := NewHost(HostConfig{BaseURL: srv.URL, Timeout: 2 * time.Second})
	err := h.Serve(context.Background(), HostServeRequest{Model: "/models/x", Profile: "/p.json"})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if captured.Runtime != "mlx" {
		t.Fatalf("Runtime not defaulted to mlx: %+v", captured)
	}
	if captured.Model != "/models/x" || captured.Profile != "/p.json" {
		t.Fatalf("request body not threaded through: %+v", captured)
	}
}

func TestHost_Serve_Bad_MissingModel(t *testing.T) {
	h := NewHost(HostConfig{BaseURL: "http://127.0.0.1:1"})
	err := h.Serve(context.Background(), HostServeRequest{})
	if err == nil {
		t.Fatal("expected error for a missing model, got nil")
	}
}

func TestHost_Serve_Bad_TransportError(t *testing.T) {
	// Reserve then release a loopback port so nothing answers there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	h := NewHost(HostConfig{BaseURL: "http://" + addr, Timeout: time.Second})
	err = h.Serve(context.Background(), HostServeRequest{Model: "/m"})
	if err == nil {
		t.Fatal("expected a transport error for an unreachable host, got nil")
	}
}

func TestHost_Serve_Bad_UpstreamErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(hostServeResponse{OK: false, Error: "driver crashed"})
	}))
	defer srv.Close()
	h := NewHost(HostConfig{BaseURL: srv.URL, Timeout: 2 * time.Second})
	err := h.Serve(context.Background(), HostServeRequest{Model: "/m"})
	if err == nil || !strings.Contains(err.Error(), "driver crashed") {
		t.Fatalf("expected the upstream error text surfaced, got %v", err)
	}
}

func TestHost_Serve_Bad_UpstreamPlainTextError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	h := NewHost(HostConfig{BaseURL: srv.URL, Timeout: 2 * time.Second})
	err := h.Serve(context.Background(), HostServeRequest{Model: "/m"})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected a status-carrying error, got %v", err)
	}
}

func TestHost_Serve_Ugly_2xxWithOKFalseStillErrors(t *testing.T) {
	// Guards a future provider regression: a 2xx body carrying OK:false
	// must still surface as an error, never silently read as success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(hostServeResponse{OK: false, Error: "not ready"})
	}))
	defer srv.Close()
	h := NewHost(HostConfig{BaseURL: srv.URL, Timeout: 2 * time.Second})
	err := h.Serve(context.Background(), HostServeRequest{Model: "/m"})
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected the 2xx OK:false envelope error surfaced, got %v", err)
	}
}

func TestHost_Serve_Bad_MalformedBaseURLFailsRequestBuild(t *testing.T) {
	// A raw control character in the URL makes http.NewRequestWithContext
	// itself fail — exercises Serve's "build request" wrap branch without
	// any network I/O.
	h := NewHost(HostConfig{BaseURL: "http://exa\nmple.invalid", Timeout: time.Second})
	err := h.Serve(context.Background(), HostServeRequest{Model: "/m"})
	if err == nil {
		t.Fatal("expected a request-build error for a malformed base URL")
	}
}

func TestHost_Serve_Ugly_BlankRuntimeDefaultsToMlx(t *testing.T) {
	var captured HostServeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	h := NewHost(HostConfig{BaseURL: srv.URL, Timeout: 2 * time.Second})
	err := h.Serve(context.Background(), HostServeRequest{Runtime: "   ", Model: "/m"})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if captured.Runtime != "mlx" {
		t.Fatalf("blank runtime should default to mlx, got %q", captured.Runtime)
	}
}
