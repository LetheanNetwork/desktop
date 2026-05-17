// SPDX-Licence-Identifier: EUPL-1.2

// Tests for Cerberus #22 HIGH-1 / Mantis #1616 — the HTTP
// listImportedProviders handler must NEVER place a raw provider
// AuthKey on the wire. The Wails surface (WListImportedProviders) has
// always masked via ProviderView; the HTTP surface previously returned
// raw ImportedProvider rows. This file pins the closed gap.
//
// Two layers of cover:
//
//  1. providersToViews — the pure conversion is exercised directly,
//     asserting JSON serialisation never contains the raw AuthKey
//     literal (defence-in-depth: if ProviderView ever gains a leaky
//     field, this test catches it).
//  2. Handler-stub — mirrors the listImportedProviders body verbatim
//     except for the Service call, drives a real gin engine, asserts
//     the response body contains the masked shape AND not the raw key.

package opencode

import (
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"
)

// TestProvidersToViews_AuthKeyAbsent_Good — the JSON bytes produced by
// the conversion helper MUST NOT contain the raw AuthKey literal, and
// MUST include the masked + present fields per ProviderView.
func TestProvidersToViews_AuthKeyAbsent_Good(t *testing.T) {
	const rawKey = "sk-ant-api03-VERY-SECRET-DO-NOT-LEAK-4f2a"

	rows := []ImportedProvider{
		{
			ID:         "host:anthropic",
			Source:     "host",
			ProviderID: "anthropic",
			Name:       "Anthropic",
			AuthType:   "apikey",
			AuthKey:    rawKey,
			HasAuth:    true,
		},
		{
			ID:         "host:openai",
			Source:     "host",
			ProviderID: "openai",
			Name:       "OpenAI",
			AuthType:   "apikey",
			AuthKey:    "",
			HasAuth:    false,
		},
	}

	views := providersToViews(rows)
	if len(views) != 2 {
		t.Fatalf("len(views) = %d; want 2", len(views))
	}

	// Configured-key row → Present true, Masked non-empty, raw absent.
	got := views[0]
	if !got.Present {
		t.Error("views[0].Present = false; want true for configured key")
	}
	if got.Masked == "" {
		t.Error("views[0].Masked empty; want masked rendering of the key")
	}
	if got.Masked == rawKey {
		t.Error("views[0].Masked equals raw AuthKey — masking did not apply")
	}

	// Empty-key row → Present false, Masked empty.
	empty := views[1]
	if empty.Present {
		t.Error("views[1].Present = true; want false for empty key")
	}
	if empty.Masked != "" {
		t.Errorf("views[1].Masked = %q; want empty", empty.Masked)
	}

	// JSON-bytes assertion — the raw key MUST NOT appear anywhere
	// in the serialised payload. core.JSONMarshal is the canonical
	// emitter used by gin's c.JSON path.
	r := core.JSONMarshal(views)
	if !r.OK {
		t.Fatalf("core.JSONMarshal(views) failed: %v", r.Error())
	}
	b, _ := r.Value.([]byte)
	if contains(string(b), rawKey) {
		t.Errorf("providersToViews JSON contains raw AuthKey; payload: %s", string(b))
	}
}

// TestProvidersToViews_NilInput_ReturnsEmptySlice_Good — nil input
// yields a non-nil zero-length slice so the JSON encoder emits []
// rather than null. The frontend ProviderView[] expectation does not
// admit null.
func TestProvidersToViews_NilInput_ReturnsEmptySlice_Good(t *testing.T) {
	views := providersToViews(nil)
	if views == nil {
		t.Fatal("providersToViews(nil) returned nil; want empty []ProviderView")
	}
	if len(views) != 0 {
		t.Errorf("len(providersToViews(nil)) = %d; want 0", len(views))
	}
	r := core.JSONMarshal(views)
	if !r.OK {
		t.Fatalf("core.JSONMarshal(empty views) failed: %v", r.Error())
	}
	b, _ := r.Value.([]byte)
	if string(b) != "[]" {
		t.Errorf("JSONMarshal(empty views) = %q; want []", string(b))
	}
}

// TestListImportedProviders_HTTP_AuthKeyMasked_Bad — end-to-end stub
// of the HTTP handler: a fixture row carrying a raw AuthKey is
// converted via providersToViews and rendered as gin's JSON response.
// The bytes on the wire MUST contain the masked shape AND MUST NOT
// contain the raw key.
//
// "_Bad" classification — the bug shape this test pins is the leak
// where the raw AuthKey reached the wire; the assertion is the
// negative-bytes check that fails loudly on regression.
func TestListImportedProviders_HTTP_AuthKeyMasked_Bad(t *testing.T) {
	const rawKey = "sk-ant-api03-CERBERUS22-HIGH1-MANTIS1616-4f2a"

	// Handler-stub mirrors listImportedProviders verbatim except for
	// the Service call (Service needs ORM + DuckDB — too heavy for a
	// unit test). The conversion + JSON-encode path is the bit under
	// test; that path lives entirely in providersToViews + c.JSON.
	h := func(c *gin.Context) {
		rows := []ImportedProvider{
			{
				ID:         "host:anthropic",
				Source:     "host",
				ProviderID: "anthropic",
				Name:       "Anthropic",
				AuthType:   "apikey",
				AuthKey:    rawKey,
				HasAuth:    true,
			},
		}
		c.JSON(core.StatusOK, gin.H{"providers": providersToViews(rows)})
	}

	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.GET("/imports/providers", h)

	req := httptest.NewRequest(core.MethodGet, "/imports/providers", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if w.Code != core.StatusOK {
		t.Fatalf("status = %d; want 200", w.Code)
	}
	body := w.Body.String()

	// Negative bytes assertion — the raw key MUST NOT appear.
	if contains(body, rawKey) {
		t.Errorf("HTTP response contains raw AuthKey — Cerberus #22 HIGH-1 regression.\nbody: %s", body)
	}
	// Positive shape assertions — masked, present, and the camelCase
	// providerId field (ProviderView json:"providerId") must all be
	// present so the frontend has what it needs.
	if !contains(body, `"present":true`) {
		t.Errorf("HTTP response missing present:true; body: %s", body)
	}
	if !contains(body, `"masked":`) {
		t.Errorf("HTTP response missing masked field; body: %s", body)
	}
	if !contains(body, `"providerId":"anthropic"`) {
		t.Errorf("HTTP response missing providerId; body: %s", body)
	}
	// Defence-in-depth — even an "authKey" or "auth_key" JSON key
	// MUST NOT appear (ProviderView has no such field; this catches
	// future struct drift).
	if contains(body, `"authKey"`) || contains(body, `"auth_key"`) {
		t.Errorf("HTTP response contains an authKey/auth_key field — ProviderView leaked; body: %s", body)
	}
}
