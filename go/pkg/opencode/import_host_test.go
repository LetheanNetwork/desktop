// SPDX-Licence-Identifier: EUPL-1.2

// import_host_test.go — coverage for import_host.go's non-exec
// surface: importFetchJSON (plain HTTP, httptest-backed),
// readHostAuthJSON (real file read under a temp $HOME),
// persistProjects / persistProviders (orm-backed, Memium), and the
// stringFrom / unixMillis / projectNameFrom pure helpers, plus
// ListImports / ListImportedProviders.
//
// ImportFromHost itself is a DELIBERATE LEAVE-OUT: it hardcodes
// process.RunOptions{Command: "opencode", ...} — a bare command name
// resolved via the OS's normal exec.LookPath / $PATH search, not
// through the package's Options.Runtime seam that Start/Stop/
// Reconcile/Upgrade use. This dev machine has a REAL `opencode`
// binary on $PATH (/opt/homebrew/bin/opencode) — spawning
// ImportFromHost's success path would invoke it for real, which the
// house rules explicitly forbid ("never invoke a real opencode/
// external tool"). We only exercise ImportFromHost's up-front guard
// clauses (nil service / no process backing), which return before any
// exec happens.

package opencode

import (
	"net/http"
	"net/http/httptest"
	"testing"

	core "dappco.re/go"
	"dappco.re/go/orm"
)

// --- importFetchJSON --------------------------------------------------

func TestImportFetchJSON_Good(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Basic abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"p1"}]`))
	}))
	t.Cleanup(srv.Close)

	got, err := importFetchJSON(srv.URL, "Basic abc")
	if err != nil {
		t.Fatalf("importFetchJSON failed: %v", err)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("importFetchJSON value = %#v; want a 1-element array", got)
	}
}

func TestImportFetchJSON_UpstreamError_Bad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(srv.Close)

	_, err := importFetchJSON(srv.URL, "")
	if err == nil {
		t.Fatalf("importFetchJSON against a 500 upstream returned nil error")
	}
}

func TestImportFetchJSON_MalformedJSON_Bad(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(srv.Close)

	_, err := importFetchJSON(srv.URL, "")
	if err == nil {
		t.Fatalf("importFetchJSON against a malformed body returned nil error")
	}
}

func TestImportFetchJSON_UnreachableHost_Bad(t *testing.T) {
	_, err := importFetchJSON("http://127.0.0.1:1/nope", "")
	if err == nil {
		t.Fatalf("importFetchJSON against an unreachable host returned nil error")
	}
}

// --- readHostAuthJSON --------------------------------------------------

func TestReadHostAuthJSON_MissingFile_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := readHostAuthJSON()
	if len(got) != 0 {
		t.Errorf("readHostAuthJSON() with no file = %+v; want empty map", got)
	}
}

func TestReadHostAuthJSON_Present_Good(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := core.PathJoin(home, ".local/share/opencode/auth.json")
	if r := core.MkdirAll(core.PathDir(path), 0o755); !r.OK {
		t.Fatalf("MkdirAll failed: %s", r.Error())
	}
	body := `{"anthropic":{"type":"apikey","key":"sk-ant-secret"}}`
	if r := core.WriteFile(path, []byte(body), 0o600); !r.OK {
		t.Fatalf("WriteFile failed: %s", r.Error())
	}

	got := readHostAuthJSON()
	entry, ok := got["anthropic"]
	if !ok {
		t.Fatalf("readHostAuthJSON() = %+v; want an 'anthropic' entry", got)
	}
	if entry["key"] != "sk-ant-secret" {
		t.Errorf("entry[key] = %v; want sk-ant-secret", entry["key"])
	}
}

func TestReadHostAuthJSON_MalformedFile_Bad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := core.PathJoin(home, ".local/share/opencode/auth.json")
	if r := core.MkdirAll(core.PathDir(path), 0o755); !r.OK {
		t.Fatalf("MkdirAll failed: %s", r.Error())
	}
	if r := core.WriteFile(path, []byte("not-json"), 0o600); !r.OK {
		t.Fatalf("WriteFile failed: %s", r.Error())
	}
	got := readHostAuthJSON()
	if len(got) != 0 {
		t.Errorf("readHostAuthJSON() with malformed file = %+v; want empty map (fail-open)", got)
	}
}

// --- stringFrom / unixMillis / projectNameFrom -------------------------

func TestStringFrom_Good(t *testing.T) {
	m := map[string]any{"id": "p1", "count": 3}
	if got := stringFrom(m, "id"); got != "p1" {
		t.Errorf("stringFrom(id) = %q; want p1", got)
	}
	if got := stringFrom(m, "count"); got != "" {
		t.Errorf("stringFrom(count, non-string) = %q; want empty", got)
	}
	if got := stringFrom(m, "missing"); got != "" {
		t.Errorf("stringFrom(missing) = %q; want empty", got)
	}
}

func TestUnixMillis_Good(t *testing.T) {
	if got := unixMillis(float64(1700000000000)); got.IsZero() {
		t.Errorf("unixMillis(float64) returned zero time")
	}
	if got := unixMillis(int64(1700000000000)); got.IsZero() {
		t.Errorf("unixMillis(int64) returned zero time")
	}
	if got := unixMillis(float64(0)); !got.IsZero() {
		t.Errorf("unixMillis(0) = %v; want zero time", got)
	}
	if got := unixMillis(float64(-5)); !got.IsZero() {
		t.Errorf("unixMillis(negative) = %v; want zero time", got)
	}
	if got := unixMillis("not-a-number"); !got.IsZero() {
		t.Errorf("unixMillis(string) = %v; want zero time", got)
	}
	if got := unixMillis(nil); !got.IsZero() {
		t.Errorf("unixMillis(nil) = %v; want zero time", got)
	}
}

func TestProjectNameFrom_Good(t *testing.T) {
	if got := projectNameFrom("/home/user/code/myproj", "fallback"); got != "myproj" {
		t.Errorf("projectNameFrom(worktree) = %q; want myproj", got)
	}
	if got := projectNameFrom("", "fallback-id"); got != "fallback-id" {
		t.Errorf("projectNameFrom(empty) = %q; want fallback-id", got)
	}
	if got := projectNameFrom("/", "fallback-id"); got != "fallback-id" {
		t.Errorf("projectNameFrom(/) = %q; want fallback-id", got)
	}
	if got := projectNameFrom("  ", "fallback-id"); got != "fallback-id" {
		t.Errorf("projectNameFrom(whitespace) = %q; want fallback-id", got)
	}
}

// --- persistProjects / persistProviders --------------------------------

func TestPersistProjects_Good(t *testing.T) {
	c := newTestCore(t)
	now := core.Now()
	raw := []any{
		map[string]any{
			"id":       "src-1",
			"worktree": "/home/user/code/proj1",
			"vcs":      "git",
			"icon":     map[string]any{"color": "purple"},
			"time":     map[string]any{"created": float64(1700000000000), "updated": float64(1700000001000)},
		},
		// Missing "id" — must be skipped, not error.
		map[string]any{"worktree": "/tmp/no-id"},
		// Non-map entry — must be skipped.
		"not-a-map",
	}
	count := persistProjects(c, raw, now)
	if count != 1 {
		t.Fatalf("persistProjects count = %d; want 1", count)
	}

	rows := orm.Of[ImportedProject](c).Get()
	if !rows.OK {
		t.Fatalf("Get failed: %s", rows.Error())
	}
	list, _ := rows.Value.([]ImportedProject)
	if len(list) != 1 || list[0].SourceID != "src-1" {
		t.Fatalf("persisted rows = %+v; want 1 row with SourceID=src-1", list)
	}
	if list[0].Name != "proj1" {
		t.Errorf("Name = %q; want proj1 (derived from worktree basename)", list[0].Name)
	}
	if list[0].IconColor != "purple" {
		t.Errorf("IconColor = %q; want purple", list[0].IconColor)
	}
}

func TestPersistProjects_SaveFails_Bad(t *testing.T) {
	c := newTestCoreNoORM(t)
	now := core.Now()
	raw := []any{map[string]any{"id": "src-1", "worktree": "/x"}}
	count := persistProjects(c, raw, now)
	if count != 0 {
		t.Errorf("persistProjects with no orm Medium count = %d; want 0", count)
	}
}

func TestPersistProviders_Good(t *testing.T) {
	c := newTestCore(t)
	now := core.Now()
	raw := []any{
		map[string]any{"id": "anthropic", "name": "Anthropic", "npm": "@ai-sdk/anthropic", "options": map[string]any{"baseURL": "https://api.anthropic.com"}},
		map[string]any{"id": "openai"}, // no auth entry
		map[string]any{"missing": "id"},
		42, // non-map
	}
	authMap := map[string]map[string]any{
		"anthropic": {"type": "apikey", "key": "sk-ant-abc"},
	}
	count, withAuth := persistProviders(c, raw, authMap, now)
	if count != 2 {
		t.Fatalf("persistProviders count = %d; want 2", count)
	}
	if withAuth != 1 {
		t.Fatalf("persistProviders withAuth = %d; want 1", withAuth)
	}

	rows := orm.Of[ImportedProvider](c).Where("provider_id", "=", "anthropic").Get()
	if !rows.OK {
		t.Fatalf("Get failed: %s", rows.Error())
	}
	list, _ := rows.Value.([]ImportedProvider)
	if len(list) != 1 || !list[0].HasAuth || list[0].AuthKey != "sk-ant-abc" {
		t.Fatalf("anthropic row = %+v; want HasAuth=true AuthKey=sk-ant-abc", list)
	}
}

func TestPersistProviders_SaveFails_Bad(t *testing.T) {
	c := newTestCoreNoORM(t)
	now := core.Now()
	raw := []any{map[string]any{"id": "anthropic"}}
	count, withAuth := persistProviders(c, raw, nil, now)
	if count != 0 || withAuth != 0 {
		t.Errorf("persistProviders with no orm Medium = (%d, %d); want (0, 0)", count, withAuth)
	}
}

// --- ListImports / ListImportedProviders --------------------------------

func TestListImports_NilService_Bad(t *testing.T) {
	var svc *Service
	r := svc.ListImports()
	if r.OK {
		t.Fatalf("ListImports on a nil Service returned OK; want Fail")
	}
}

func TestListImportedProviders_NilService_Bad(t *testing.T) {
	var svc *Service
	r := svc.ListImportedProviders()
	if r.OK {
		t.Fatalf("ListImportedProviders on a nil Service returned OK; want Fail")
	}
}

func TestListImports_OrderedMostRecentFirst_Good(t *testing.T) {
	svc := newTestService(t, Options{})
	c := svc.Core()
	older := ImportedProject{ID: "a:1", Source: "a", SourceID: "1", Name: "old", ImportedAt: core.UnixMilli(1000)}
	newer := ImportedProject{ID: "a:2", Source: "a", SourceID: "2", Name: "new", ImportedAt: core.UnixMilli(2000)}
	if r := orm.Of[ImportedProject](c).Save(&older); !r.OK {
		t.Fatalf("seed failed: %s", r.Error())
	}
	if r := orm.Of[ImportedProject](c).Save(&newer); !r.OK {
		t.Fatalf("seed failed: %s", r.Error())
	}
	r := svc.ListImports()
	if !r.OK {
		t.Fatalf("ListImports failed: %s", r.Error())
	}
	rows, _ := r.Value.([]ImportedProject)
	if len(rows) != 2 || rows[0].Name != "new" {
		t.Fatalf("ListImports = %+v; want [new, old]", rows)
	}
}

// --- ImportFromHost guard clauses only (see file header) ---------------

func TestImportFromHost_NilService_Bad(t *testing.T) {
	var svc *Service
	r := svc.ImportFromHost()
	if r.OK {
		t.Fatalf("ImportFromHost on a nil Service returned OK; want Fail")
	}
}

func TestImportFromHost_ProcessUnavailable_Bad(t *testing.T) {
	svc := &Service{}
	r := svc.ImportFromHost()
	if r.OK {
		t.Fatalf("ImportFromHost on a bare Service returned OK; want Fail")
	}
	if !core.Contains(r.Error(), "process service unavailable") {
		t.Errorf("error = %q; want 'process service unavailable'", r.Error())
	}
}
