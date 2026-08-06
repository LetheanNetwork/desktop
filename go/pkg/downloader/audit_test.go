// SPDX-Licence-Identifier: EUPL-1.2

// Tests for the audit-cluster pattern adoption in pkg/downloader —
// FetchVerified now emits typed audit.EventDownloader* rows via
// audit.Default() so the auditor sees model-supply-chain decisions
// instead of forensically silent fetch ops. Cerberus shape-observation
// follow-on to gateway H#211 / marketplace H#208 / process H#193 /
// sandbox H#188 / runner H#181 — 6th audit-cluster adoption.
//
// Internal package so the resolver seam lookupHostFn is reachable for
// the private-IP-rejected smuggle-surface test (mirrors dns_test.go).
// The fetch-pipeline tests share the homeFixture+httptest pattern from
// downloader_test.go.

package downloader

import (
	"net/http/httptest"
	"sync"

	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/audit"
)

// downloaderScopeLiteral mirrors the package-internal const stamped on
// every typed audit row from this package — duplicated here as a string
// literal so a future rename of downloaderScope still fires the test if
// the constant value drifts away from the closed-set spec.
const downloaderScopeLiteral = "downloader"

// recordingRecorder captures every audit.Event the downloader emits
// during a test. Mirror of the same-named fixture in
// pkg/marketplace/audit_test.go + pkg/process/audit_test.go.
type recordingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingRecorder) Record(ev audit.Event) core.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return core.Ok(nil)
}

func (r *recordingRecorder) snapshot() []audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Event, len(r.events))
	copy(out, r.events)
	return out
}

func (r *recordingRecorder) filterByName(name string) []audit.Event {
	all := r.snapshot()
	out := make([]audit.Event, 0, len(all))
	for _, ev := range all {
		if ev.Event == name {
			out = append(out, ev)
		}
	}
	return out
}

// installAuditRecorder wires a recordingRecorder as audit.Default and
// restores the prior default at test end.
func installAuditRecorder(t *core.T) *recordingRecorder {
	t.Helper()
	rec := &recordingRecorder{}
	audit.SetDefault(rec)
	t.Cleanup(func() { audit.SetDefault(nil) })
	return rec
}

// sandboxHome puts $HOME under t.TempDir so ~/Lethean/conf/models/
// resolves to a disposable tree. Mirror of homeFixture in
// downloader_test.go but uses *core.T because this is an internal-
// package test.
func sandboxHome(t *core.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// TestDownloader_FetchVerified_EmitsAuditTrail_Good — happy-path fetch
// from a httptest server (resolves to 127.0.0.1, which is on the
// loopbackAllowlistEntries skip-list). Asserts the Requested + Succeeded
// pair lands with the expected scope + outcome + meta keys.
func TestDownloader_FetchVerified_EmitsAuditTrail_Good(t *core.T) {
	sandboxHome(t)
	rec := installAuditRecorder(t)

	payload := []byte("MOCK-MODEL-BYTES-FOR-AUDIT-EMIT")
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := FetchVerified(srv.URL, "audit-good.gguf", "", nil)
	core.AssertTrue(t, r.OK, "FetchVerified happy path must succeed")

	requested := rec.filterByName(audit.EventDownloaderFetchRequested)
	succeeded := rec.filterByName(audit.EventDownloaderFetchSucceeded)
	failed := rec.filterByName(audit.EventDownloaderFetchFailed)
	rejected := rec.filterByName(audit.EventDownloaderFetchRejected)

	core.AssertEqual(t, 1, len(requested),
		"exactly one Requested row on happy path")
	core.AssertEqual(t, 1, len(succeeded),
		"exactly one Succeeded row on happy path")
	core.AssertEqual(t, 0, len(failed),
		"no Failed row on happy path")
	core.AssertEqual(t, 0, len(rejected),
		"no Rejected row on happy path")

	// Scope + Outcome shape per RFC §2.
	core.AssertEqual(t, downloaderScopeLiteral, requested[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, requested[0].Outcome)
	core.AssertEqual(t, downloaderScopeLiteral, succeeded[0].Scope)
	core.AssertEqual(t, audit.OutcomeOK, succeeded[0].Outcome)

	// Meta shape — Requested carries source_host + sha256_expected.
	core.AssertEqual(t, "127.0.0.1", requested[0].Meta["source_host"])
	core.AssertEqual(t, "", requested[0].Meta["sha256_expected"])

	// Succeeded carries source_host + bytes.
	core.AssertEqual(t, "127.0.0.1", succeeded[0].Meta["source_host"])
	core.AssertEqual(t, int64(len(payload)), succeeded[0].Meta["bytes"])
}

// TestDownloader_FetchVerified_PrivateIPRejected_EmitsRejected_Bad —
// allowlisted hostname resolves to 127.0.0.1 via the stub resolver
// (DNS rebinding pattern). MUST emit Requested + Rejected with
// reason=private_ip_resolved and OutcomeDenied; no Succeeded/Failed.
func TestDownloader_FetchVerified_PrivateIPRejected_EmitsRejected_Bad(t *core.T) {
	sandboxHome(t)
	rec := installAuditRecorder(t)
	// Pin huggingface.co → 127.0.0.1 so the SSRF gate fires.
	stubResolver(t, map[string][]string{
		"huggingface.co": {"127.0.0.1"},
	})

	r := FetchVerified(
		"https://huggingface.co/foo/resolve/main/model.gguf",
		"rebind.gguf", "", nil)
	core.AssertFalse(t, r.OK, "private-IP gate must reject")
	core.AssertTrue(t,
		core.Is(r.Value.(error), ErrPrivateIPResolved),
		"error must carry ErrPrivateIPResolved")

	requested := rec.filterByName(audit.EventDownloaderFetchRequested)
	rejected := rec.filterByName(audit.EventDownloaderFetchRejected)
	succeeded := rec.filterByName(audit.EventDownloaderFetchSucceeded)
	failed := rec.filterByName(audit.EventDownloaderFetchFailed)

	core.AssertEqual(t, 1, len(requested),
		"Requested row commits BEFORE the SSRF gate fires")
	core.AssertEqual(t, 1, len(rejected),
		"exactly one Rejected row on private-IP gate")
	core.AssertEqual(t, 0, len(succeeded),
		"no Succeeded row when policy gate fires")
	core.AssertEqual(t, 0, len(failed),
		"no Failed row when policy gate fires — Rejected has its own bucket")

	core.AssertEqual(t, downloaderScopeLiteral, rejected[0].Scope)
	core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
	core.AssertEqual(t, "huggingface.co", rejected[0].Meta["source_host"])
	core.AssertEqual(t, "private_ip_resolved", rejected[0].Meta["reason"])
}

// TestDownloader_FetchVerified_HostNotAllowed_EmitsRejected_Bad —
// off-allowlist URL hits the AllowedSource gate. MUST emit
// Requested + Rejected with reason=host_not_allowed; no
// Succeeded/Failed.
func TestDownloader_FetchVerified_HostNotAllowed_EmitsRejected_Bad(t *core.T) {
	sandboxHome(t)
	rec := installAuditRecorder(t)

	r := FetchVerified(
		"https://evil.example.com/model.gguf",
		"offlist.gguf", "", nil)
	core.AssertFalse(t, r.OK, "off-allowlist host must reject")
	core.AssertTrue(t,
		core.Contains(r.Error(), "source not allowed"),
		"error must carry the source-not-allowed scope")

	requested := rec.filterByName(audit.EventDownloaderFetchRequested)
	rejected := rec.filterByName(audit.EventDownloaderFetchRejected)
	succeeded := rec.filterByName(audit.EventDownloaderFetchSucceeded)
	failed := rec.filterByName(audit.EventDownloaderFetchFailed)

	core.AssertEqual(t, 1, len(requested),
		"Requested row commits BEFORE the host-allowlist gate fires")
	core.AssertEqual(t, 1, len(rejected),
		"exactly one Rejected row on host-not-allowed gate")
	core.AssertEqual(t, 0, len(succeeded))
	core.AssertEqual(t, 0, len(failed),
		"host-allowlist rejection is Rejected, not Failed")

	core.AssertEqual(t, downloaderScopeLiteral, rejected[0].Scope)
	core.AssertEqual(t, audit.OutcomeDenied, rejected[0].Outcome)
	core.AssertEqual(t, "evil.example.com", rejected[0].Meta["source_host"])
	core.AssertEqual(t, "host_not_allowed", rejected[0].Meta["reason"])
}

// TestDownloader_AuditMeta_ErrorCodeBoundedKeyspace_Bad — STRIDE-I
// prose-leak canary per RFC.error-code-cascade.md §3 (W4) /
// Mantis #1716 / Cerberus #61 C-4. Forces the HTTP-500 failure path
// (which builds a failErr whose Error() reads "downloader.Fetch: HTTP
// 500 from http://127.0.0.1:NNN/...") and asserts the emitted Failed
// row's Meta["error_code"] is the BOUNDED Operation literal
// "downloader.Fetch", NOT the raw prose containing the URL. Defends
// the audit.ErrorCode(r) substrate at the helper boundary so future
// regressions that revert the helper signature to a raw string fall
// loud rather than silently re-leaking caller-controlled URL bytes.
//
// The pre-W4 shape (emitFetchFailed(url, failErr.Error())) would have
// landed the full "downloader.Fetch: HTTP 500 from <url>" string in
// Meta["error_code"] — this test pins the type-system contract that
// makes that bug class unreachable.
func TestDownloader_AuditMeta_ErrorCodeBoundedKeyspace_Bad(t *core.T) {
	sandboxHome(t)
	rec := installAuditRecorder(t)

	// Server returns HTTP 500 — fires the resp.StatusCode >= 400 branch
	// in FetchVerified where failErr embeds the full URL via
	// "HTTP 500 from <url>".
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	// Smuggle a canary token into the URL query so we can grep for any
	// caller-controlled bytes echoing through into the audit row.
	smuggled := srv.URL + "/?canary=LTHN-ERRORCODE-LEAK-CANARY-NOTAREALTOKEN"
	r := FetchVerified(smuggled, "errorcode-bounded.gguf", "", nil)
	core.AssertFalse(t, r.OK, "HTTP 500 must fail")

	failed := rec.filterByName(audit.EventDownloaderFetchFailed)
	core.AssertEqual(t, 1, len(failed),
		"exactly one Failed row on HTTP 500 path")

	// Bounded-keyspace contract: error_code is the *core.Err.Operation
	// canon per audit.ErrorCode(r) resolution order, NOT the raw
	// failErr.Error() prose. fetchOp == "downloader.Fetch".
	code, ok := failed[0].Meta["error_code"].(string)
	core.AssertTrue(t, ok, "error_code Meta value must be a string")
	core.AssertEqual(t, fetchOp, code,
		"error_code must be the bounded Operation literal (audit.ErrorCode), not raw prose")

	// Belt-and-braces: even if a future shape returns a different
	// bounded code, NONE of the caller-controlled URL bytes (host:port
	// canary query) may appear in the field.
	core.AssertFalse(t, core.Contains(code, "canary"),
		"error_code must not echo caller-controlled URL query: got "+code)
	core.AssertFalse(t, core.Contains(code, "LTHN-ERRORCODE-LEAK"),
		"error_code must not echo canary marker: got "+code)
	core.AssertFalse(t, core.Contains(code, "HTTP 500"),
		"error_code must not echo upstream HTTP status prose: got "+code)
	core.AssertFalse(t, core.Contains(code, "127.0.0.1"),
		"error_code must not echo target host: got "+code)
}

// TestDownloader_AuditMeta_NoRawBearer_Bad — smuggle-surface canary
// per the H#181 SECURITY-NOTE pattern. A URL embedding a `Bearer
// not-a-real-token` query argument fires through the happy path; the
// emitted Meta MUST NOT carry the raw bearer bytes in source_host or
// sha256_expected (or any string in Meta). Defends Cerberus #1465
// closure-only-scope discipline at the audit boundary.
//
// NOTE: this asserts the BOUNDARY guarantee — the emit helpers stamp
// only the closed-set Meta keys (source_host, sha256_expected, bytes,
// error_code, reason) and never include the raw URL. Even if a future
// regression added the raw URL to Meta, the audit.Default sink's
// secret-shape redactor would strip "Bearer ..." prefixes, but the
// test verifies the helper-level discipline so the redactor stays a
// backstop rather than the primary defence.
func TestDownloader_AuditMeta_NoRawBearer_Bad(t *core.T) {
	sandboxHome(t)
	rec := installAuditRecorder(t)

	payload := []byte("PAYLOAD")
	srv := httptest.NewServer(core.HandlerFunc(func(w core.ResponseWriter, _ *core.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	// httptest URL plus a smuggled "?bearer=not-a-real-token" query.
	smuggled := srv.URL + "/?bearer=LTHN-BOOT-1.test.fixture.canary.notarealtoken"
	r := FetchVerified(smuggled, "bearer-canary.gguf", "", nil)
	core.AssertTrue(t, r.OK)

	// Walk every emitted Meta value — none may contain the bearer
	// payload (case-insensitive contains check covers both literal
	// bearer prefixes and the LTHN-BOOT marker).
	for _, ev := range rec.snapshot() {
		for k, v := range ev.Meta {
			s, ok := v.(string)
			if !ok {
				continue
			}
			lower := core.Lower(s)
			core.AssertFalse(t,
				core.Contains(lower, "bearer"),
				"Meta["+k+"] must not contain raw bearer bytes; got: "+s)
			core.AssertFalse(t,
				core.Contains(lower, "lthn-boot-1"),
				"Meta["+k+"] must not contain bootstrap-token canary; got: "+s)
			core.AssertFalse(t,
				core.Contains(s, "?"),
				"Meta["+k+"] must not contain query-string bytes; got: "+s)
		}
	}
}

// TestDomainOnly_Good — a well-formed URL reduces to its lowercase
// hostname with no path/query/fragment/port.
func TestDomainOnly_Good(t *core.T) {
	core.AssertEqual(t, "hf.co", domainOnly("https://HF.co/foo/resolve/main/x.gguf?token=secret"))
}

// TestDomainOnly_Bad — empty and unparseable inputs both degrade to
// "" rather than panicking.
func TestDomainOnly_Bad(t *core.T) {
	core.AssertEqual(t, "", domainOnly(""))
	core.AssertEqual(t, "", domainOnly("   "))
	core.AssertEqual(t, "", domainOnly("://not a url"))
}

// TestDomainOnly_Ugly — a bare relative path (no scheme/host) parses
// without error but carries no hostname, so domainOnly still yields
// "".
func TestDomainOnly_Ugly(t *core.T) {
	core.AssertEqual(t, "", domainOnly("/just/a/path"))
}
