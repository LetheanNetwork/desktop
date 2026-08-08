// SPDX-Licence-Identifier: EUPL-1.2

//go:build !ios && !android

// opencode_test.go — hermetic coverage for the `lthn opencode` thin
// HTTP client. The package's base-URL vars are repointed at a local
// httptest server that plays the serve daemon's control surface, so
// every verb exercises its full request/response arms without a live
// daemon and without ever reaching a real sandbox. The error arms use
// a loopback port nothing listens on (instant connection refused).

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
)

// withOpencodeServer starts the fake control surface and repoints
// every opencode base URL at it, restoring the production values on
// cleanup.
func withOpencodeServer(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewServer(handler)
	saved := []struct {
		v   *string
		old string
	}{
		{&opencodeBase, opencodeBase},
		{&opencodeProfBase, opencodeProfBase},
		{&opencodeHostConfigBase, opencodeHostConfigBase},
		{&opencodeEnableBase, opencodeEnableBase},
		{&opencodeDisableBase, opencodeDisableBase},
		{&opencodeEnabledBase, opencodeEnabledBase},
		{&opencodeImportBase, opencodeImportBase},
		{&opencodeImportsBase, opencodeImportsBase},
		{&opencodeUpgradeBase, opencodeUpgradeBase},
	}
	t.Cleanup(func() {
		srv.Close()
		for _, s := range saved {
			*s.v = s.old
		}
	})
	opencodeBase = srv.URL + "/v1/api/opencode/sandbox"
	opencodeProfBase = srv.URL + "/v1/api/opencode/profile"
	opencodeHostConfigBase = srv.URL + "/v1/api/opencode/host-config"
	opencodeEnableBase = srv.URL + "/v1/api/opencode/enable"
	opencodeDisableBase = srv.URL + "/v1/api/opencode/disable"
	opencodeEnabledBase = srv.URL + "/v1/api/opencode/enabled"
	opencodeImportBase = srv.URL + "/v1/api/opencode/import"
	opencodeImportsBase = srv.URL + "/v1/api/opencode/imports"
	opencodeUpgradeBase = srv.URL + "/v1/api/opencode/upgrade"
}

// fakeControlSurface answers every control route the CLI knows with
// a minimal 200 body; unknown sandbox ids 404 so the non-200 arms
// run; /web returns the JSON envelope the pretty-printer unwraps.
func fakeControlSurface() http.Handler {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
	mux.HandleFunc("/v1/api/opencode/sandbox", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			ok(w, `{"id":"oc-test"}`)
			return
		}
		ok(w, `[]`)
	})
	mux.HandleFunc("/v1/api/opencode/sandbox/oc-test", func(w http.ResponseWriter, r *http.Request) {
		ok(w, `{"id":"oc-test","status":"running"}`)
	})
	mux.HandleFunc("/v1/api/opencode/sandbox/oc-test/providers", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"providers":[]}`)
	})
	mux.HandleFunc("/v1/api/opencode/sandbox/oc-test/web", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"url":"http://opencode:hex@127.0.0.1:61830/"}`)
	})
	mux.HandleFunc("/v1/api/opencode/sandbox/oc-raw/web", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `plain-text-not-json`)
	})
	mux.HandleFunc("/v1/api/opencode/sandbox/oc-test/tui", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"opened":true}`)
	})
	mux.HandleFunc("/v1/api/opencode/profile", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"profiles":["default"]}`)
	})
	mux.HandleFunc("/v1/api/opencode/profile/default", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"name":"default"}`)
	})
	mux.HandleFunc("/v1/api/opencode/host-config", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"merged":true}`)
	})
	mux.HandleFunc("/v1/api/opencode/enable", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"enabled":true}`)
	})
	mux.HandleFunc("/v1/api/opencode/disable", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"enabled":false}`)
	})
	mux.HandleFunc("/v1/api/opencode/enabled", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"enabled":false}`)
	})
	mux.HandleFunc("/v1/api/opencode/import", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"projects":0,"providers":0}`)
	})
	mux.HandleFunc("/v1/api/opencode/imports", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `[]`)
	})
	mux.HandleFunc("/v1/api/opencode/imports/providers", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `[]`)
	})
	mux.HandleFunc("/v1/api/opencode/upgrade", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, `{"updated":false}`)
	})
	// Everything else (unknown sandbox ids, unknown profiles) is the
	// mux's default 404 — the non-200 arms.
	return mux
}

// TestOpencode_CmdOpenCode_Bad — missing verb, unknown verb, and
// every pre-request usage guard; no server involved.
func TestOpencode_CmdOpenCode_Bad(t *testing.T) {
	core.AssertEqual(t, 2, cmdOpenCode(nil))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"nonsense"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"stop"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"inspect"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"providers"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"web"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"tui"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"profile"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"profile", "nonsense"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"profile", "show"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"profile", "save"}))
	core.AssertEqual(t, 2, cmdOpenCode([]string{"profile", "delete"}))
}

// TestOpencode_CmdOpenCode_Good — every verb against the fake
// control surface completes its 200 arm.
func TestOpencode_CmdOpenCode_Good(t *testing.T) {
	withOpencodeServer(t, fakeControlSurface())

	core.AssertEqual(t, 0, cmdOpenCode([]string{"start", "--profile", "default"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"status"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"inspect", "oc-test"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"stop", "oc-test"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"providers", "oc-test"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"web", "oc-test"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"web", "oc-raw"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"tui", "oc-test"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"profile", "list"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"profile", "show", "default"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"profile", "delete", "default"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"merge-host-config", "--profile", "default", "--force"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"enable"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"disable"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"enabled"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"import"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"imports"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"imports", "providers"}))
	core.AssertEqual(t, 0, cmdOpenCode([]string{"upgrade"}))

	profilePath := core.PathJoin(t.TempDir(), "profile.json")
	core.RequireTrue(t, core.WriteFile(profilePath, []byte(`{"name":"default"}`), 0o644).OK)
	core.AssertEqual(t, 0, cmdOpenCode([]string{"profile", "save", profilePath}))
}

// TestOpencode_CmdOpenCode_Ugly — unknown ids take the non-200 arm;
// a dead loopback port takes the transport-error arm with the serve
// hint; a missing profile file fails before any request.
func TestOpencode_CmdOpenCode_Ugly(t *testing.T) {
	withOpencodeServer(t, fakeControlSurface())

	core.AssertEqual(t, 1, cmdOpenCode([]string{"inspect", "no-such-id"}))
	core.AssertEqual(t, 1, cmdOpenCode([]string{"stop", "no-such-id"}))
	core.AssertEqual(t, 1, cmdOpenCode([]string{"profile", "show", "no-such-profile"}))
	core.AssertEqual(t, 1, cmdOpenCode([]string{"profile", "save", "/no/such/file.json"}))

	opencodeBase = "http://127.0.0.1:1/v1/api/opencode/sandbox"
	core.AssertEqual(t, 1, cmdOpenCode([]string{"start"}))
	core.AssertEqual(t, 1, cmdOpenCode([]string{"status"}))
}
