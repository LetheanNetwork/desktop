// SPDX-Licence-Identifier: EUPL-1.2

// Internal tests for pr_fetch.go — doFetch, isSafePRURL, parseTime,
// and newPRActivityID are unexported. fetchClient is swapped to a
// hermetic httptest.NewTLSServer client for the duration of each
// test (mirrors pkg/downloader/tlspin_internal_test.go's precedent);
// no production code changes — fetchClient is already a mutable
// package var.

package vi

import (
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
	"dappco.re/go/orm"
)

// newFetchInternalCore builds a Core with orm + memium + the vi
// schema registered — enough for Fetch/doFetch's persistence path,
// no config/queue needed since callers pass PRRepo values directly.
// Named distinctly from vi_test's newFetchCore (pr_fetch_test.go) —
// same shape, different package scope, no collision, but a shared
// name across the internal/external split would read as accidental.
func newFetchInternalCore(t *core.T) *core.Core {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, orm.Register(c).OK)
	mem := orm.NewMemium()
	core.RequireTrue(t, orm.Mount(c, "default", mem).OK)
	for _, schema := range Schemas() {
		core.RequireTrue(t, orm.RegisterSchema(c, schema).OK)
		mem.RegisterTable(schema.Name, schema)
	}
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() { _ = c.ServiceShutdown(core.Background()) })
	return c
}

// withFetchClient swaps fetchClient for the duration of the test.
func withFetchClient(t *core.T, client *http.Client) {
	t.Helper()
	orig := fetchClient
	fetchClient = client
	t.Cleanup(func() { fetchClient = orig })
}

// TestPRFetch_DoFetch_Good — a real TLS round-trip against a
// hermetic server returns a decoded PR slice + 200 + no error.
func TestPRFetch_DoFetch_Good(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		core.AssertEqual(t, "application/json", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"number":1,"title":"t","state":"open",` +
			`"html_url":"https://forge.lthn.sh/o/n/pulls/1","user":{"login":"a"}}]`))
	}))
	defer ts.Close()
	withFetchClient(t, ts.Client())

	prs, status, errMsg := doFetch(ts.URL, ProviderForge, "")
	core.AssertEqual(t, "", errMsg)
	core.AssertEqual(t, 200, status)
	core.AssertEqual(t, 1, len(prs))
	core.AssertEqual(t, 1, prs[0].Number)
}

// TestPRFetch_DoFetch_Good_GitHubHeaders — the GitHub provider path
// stamps the versioned Accept + API-Version headers and accepts a
// bearer-shaped "token " auth header.
func TestPRFetch_DoFetch_Good_GitHubHeaders(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		core.AssertEqual(t, "application/vnd.github+json", r.Header.Get("Accept"))
		core.AssertEqual(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))
		core.AssertEqual(t, "token sekret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer ts.Close()
	withFetchClient(t, ts.Client())

	prs, status, errMsg := doFetch(ts.URL, ProviderGitHub, "sekret")
	core.AssertEqual(t, "", errMsg)
	core.AssertEqual(t, 200, status)
	core.AssertEqual(t, 0, len(prs))
}

// TestPRFetch_DoFetch_Bad_NonHTTPS — a plain-http URL is rejected
// before any request is built.
func TestPRFetch_DoFetch_Bad_NonHTTPS(t *core.T) {
	_, status, errMsg := doFetch("http://insecure.example", ProviderForge, "")
	core.AssertEqual(t, 0, status)
	core.AssertContains(t, errMsg, "https://")
}

// TestPRFetch_DoFetch_Bad_TransportError — an unreachable https
// endpoint (loopback, closed port) surfaces a transport error rather
// than panicking. No fetchClient swap — the default client dialling
// a closed local port fails fast.
func TestPRFetch_DoFetch_Bad_TransportError(t *core.T) {
	_, status, errMsg := doFetch("https://127.0.0.1:1/", ProviderForge, "")
	core.AssertEqual(t, 0, status)
	core.AssertNotEmpty(t, errMsg)
}

// TestPRFetch_DoFetch_Bad_HTTPErrorStatus — a 404 response drains
// the body + reports the status without attempting a JSON parse.
func TestPRFetch_DoFetch_Bad_HTTPErrorStatus(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()
	withFetchClient(t, ts.Client())

	prs, status, errMsg := doFetch(ts.URL, ProviderForge, "")
	core.AssertEqual(t, 404, status)
	core.AssertEqual(t, "", errMsg)
	core.AssertNil(t, prs)
}

// TestPRFetch_DoFetch_Bad_ContentLengthExceeded — an honest
// Content-Length above MaxPRBodyBytes rejects before any body read.
func TestPRFetch_DoFetch_Bad_ContentLengthExceeded(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "99999999")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withFetchClient(t, ts.Client())

	_, status, errMsg := doFetch(ts.URL, ProviderForge, "")
	core.AssertEqual(t, 200, status)
	core.AssertContains(t, errMsg, "exceeds cap")
}

// TestPRFetch_DoFetch_Ugly_MalformedJSON — a 200 response whose body
// isn't valid JSON surfaces a parse error rather than panicking.
func TestPRFetch_DoFetch_Ugly_MalformedJSON(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}))
	defer ts.Close()
	withFetchClient(t, ts.Client())

	prs, status, errMsg := doFetch(ts.URL, ProviderForge, "")
	core.AssertEqual(t, 200, status)
	core.AssertContains(t, errMsg, "JSON parse failed")
	core.AssertNil(t, prs)
}

// TestPRFetch_Fetch_Good — end-to-end: a hermetic server returns one
// safe-URL open PR; Fetch persists it and reports inserted=1.
func TestPRFetch_Fetch_Good(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"number":42,"title":"widen the gate","state":"open",` +
			`"html_url":"https://forge.lthn.sh/lthn/desktop/pulls/42",` +
			`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T03:04:05Z",` +
			`"user":{"login":"snider"}}]`))
	}))
	defer ts.Close()
	withFetchClient(t, ts.Client())

	c := newFetchInternalCore(t)
	repo := PRRepo{Provider: ProviderForge, BaseURL: ts.URL, Owner: "lthn", Name: "desktop"}
	r := Fetch(c, repo)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 1, r.Value.(int))

	rows := orm.Of[PRActivity](c).Get()
	core.RequireTrue(t, rows.OK)
	activity, ok := rows.Value.([]PRActivity)
	core.RequireTrue(t, ok)
	core.RequireTrue(t, len(activity) == 1)
	core.AssertEqual(t, 42, activity[0].PRNumber)
	core.AssertEqual(t, "widen the gate", activity[0].Title)
	core.AssertEqual(t, "snider", activity[0].Author)
	core.AssertFalse(t, activity[0].OpenedAt.IsZero())
}

// Fetch's surface-guard Bad path (nil core / missing owner+name /
// unsupported provider) is already covered by vi_test's
// TestPRFetch_Fetch_Bad in pr_fetch_test.go — not duplicated here.

// TestPRFetch_Fetch_Ugly — a mixed-safety PR page drops the
// javascript:-URL row (Cerberus pass-6) but persists the safe one;
// a 401 response short-circuits to Ok(0) without erroring the tick.
func TestPRFetch_Fetch_Ugly(t *core.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"number":1,"title":"safe","state":"open","html_url":"https://forge.lthn.sh/o/n/pulls/1","user":{"login":"a"}},
			{"number":2,"title":"unsafe","state":"open","html_url":"javascript:alert(1)","user":{"login":"b"}}
		]`))
	}))
	defer ts.Close()
	withFetchClient(t, ts.Client())

	c := newFetchInternalCore(t)
	repo := PRRepo{Provider: ProviderForge, BaseURL: ts.URL, Owner: "o", Name: "n"}
	r := Fetch(c, repo)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 1, r.Value.(int))

	unauthTS := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthTS.Close()
	withFetchClient(t, unauthTS.Client())
	repo.BaseURL = unauthTS.URL
	r = Fetch(c, repo)
	core.RequireTrue(t, r.OK)
	core.AssertEqual(t, 0, r.Value.(int))
}

// TestPRFetch_IsSafePRURL_Good / _Bad — https:// accepted, anything
// else (including the javascript: RCE vector) rejected.
func TestPRFetch_IsSafePRURL_Good(t *core.T) {
	core.AssertTrue(t, isSafePRURL("https://forge.lthn.sh/o/n/pulls/1"))
}

func TestPRFetch_IsSafePRURL_Bad(t *core.T) {
	core.AssertFalse(t, isSafePRURL("javascript:alert(1)"))
	core.AssertFalse(t, isSafePRURL("http://insecure.example"))
	core.AssertFalse(t, isSafePRURL(""))
}

// TestPRFetch_ParseTime_Good / _Bad — valid RFC3339 parses; empty
// and malformed strings fall back to the zero time rather than
// panicking or erroring the caller.
func TestPRFetch_ParseTime_Good(t *core.T) {
	tm := parseTime("2026-01-02T03:04:05Z")
	core.AssertFalse(t, tm.IsZero())
	core.AssertEqual(t, 2026, tm.Year())
}

func TestPRFetch_ParseTime_Bad(t *core.T) {
	core.AssertTrue(t, parseTime("").IsZero())
	core.AssertTrue(t, parseTime("not-a-time").IsZero())
}

// TestPRFetch_NewPRActivityID_Good — IDs carry the "pra-" grep
// prefix and are non-empty + distinct across calls.
func TestPRFetch_NewPRActivityID_Good(t *core.T) {
	a := newPRActivityID()
	b := newPRActivityID()
	core.AssertTrue(t, core.HasPrefix(a, "pra-"))
	core.AssertNotEqual(t, a, b)
}

// TestPRFetch_BuildListURL_Ugly — the default-provider branches
// (unset BaseURL) compose the documented default hosts.
func TestPRFetch_BuildListURL_Ugly(t *core.T) {
	forge := buildListURL(PRRepo{Provider: ProviderForge, Owner: "lthn", Name: "desktop"})
	core.AssertContains(t, forge, defaultForgeBaseURL)
	core.AssertContains(t, forge, "/api/v1/repos/lthn/desktop/pulls")

	gh := buildListURL(PRRepo{Provider: ProviderGitHub, Owner: "lthn", Name: "desktop"})
	core.AssertContains(t, gh, defaultGitHubBaseURL)
	core.AssertContains(t, gh, "per_page=")

	core.AssertEqual(t, "", buildListURL(PRRepo{Provider: "bitbucket", Owner: "o", Name: "n"}))
}
