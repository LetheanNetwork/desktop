// SPDX-License-Identifier: EUPL-1.2

package lemma

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeAdminServer answers the /v1/admin/* surface with canned shapes.
// Caller can override per-path responses via the responses map. Every
// handler verifies the Bearer header matches the expected token.
func fakeAdminServer(t *testing.T, token string, responses map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			http.Error(w, "missing/wrong bearer: "+got, http.StatusUnauthorized)
			return
		}
		key := r.Method + " " + r.URL.Path
		body, ok := responses[key]
		if !ok {
			http.Error(w, "no canned response for "+key, http.StatusNotFound)
			return
		}
		// Body can be raw JSON bytes (already-shaped) or any value to
		// marshal. Lets tests pass mismatched-schema bytes when they
		// want to exercise the decode path.
		w.Header().Set("content-type", "application/json")
		switch v := body.(type) {
		case []byte:
			_, _ = w.Write(v)
		case string:
			_, _ = w.Write([]byte(v))
		default:
			_ = json.NewEncoder(w).Encode(v)
		}
	}))
}

// TestNewAdminLoadsTokenFromFile — explicit TokenPath wins over the
// default home-dir path, and the token is trimmed before use.
func TestNewAdminLoadsTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "admin.token")
	tok := "lthn-mlx_abc123def456abc123def456"
	if err := writeFile(t, tokPath, "  "+tok+"\n  "); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	srv := fakeAdminServer(t, tok, map[string]any{
		"GET /v1/admin/machine": MachineInfo{Hash: "abc", Runtime: "metal"},
	})
	defer srv.Close()

	admin, err := NewAdmin(AdminConfig{
		BaseURL:   srv.URL,
		TokenPath: tokPath,
		Timeout:   2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewAdmin: %v", err)
	}
	mi, err := admin.Machine(context.Background())
	if err != nil {
		t.Fatalf("Machine: %v", err)
	}
	if mi.Hash != "abc" || mi.Runtime != "metal" {
		t.Fatalf("Machine = %+v, want hash=abc runtime=metal", mi)
	}
}

// TestNewAdminEmptyTokenFileFails — admin without token is useless,
// loader rejects empty files instead of silently authenticating with
// the empty string.
func TestNewAdminEmptyTokenFileFails(t *testing.T) {
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "admin.token")
	if err := writeFile(t, tokPath, "   \n   "); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	_, err := NewAdmin(AdminConfig{TokenPath: tokPath})
	if err == nil {
		t.Fatalf("expected error for empty token file, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error should mention empty: %v", err)
	}
}

// TestAdminStatusRoundtrip — the full ServeStatus shape survives a
// real HTTP cycle (catches type-tag drift between client + server).
func TestAdminStatusRoundtrip(t *testing.T) {
	const tok = "lthn-mlx_token123"
	want := ServeStatus{
		ModelPath:    "/models/lemer-lite",
		ProfilePath:  "/profiles/laptop.json",
		Runtime:      "metal",
		LoadedAtUnix: 1716700000,
		Config: ServeStatusConfig{
			ContextLength:        4096,
			ParallelSlots:        1,
			PromptCache:          true,
			PromptCacheMinTokens: 32,
			CachePolicy:          "fifo",
			BatchSize:            8,
			AdapterPath:          "/adapters/lek2-rank8",
		},
	}
	srv := fakeAdminServer(t, tok, map[string]any{
		"GET /v1/admin/serve/status": want,
	})
	defer srv.Close()

	admin, err := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: tok})
	if err != nil {
		t.Fatalf("NewAdmin: %v", err)
	}
	got, err := admin.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got != want {
		t.Fatalf("Status mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// TestAdminProfilesRoundtrip — profile list shape survives.
func TestAdminProfilesRoundtrip(t *testing.T) {
	const tok = "lthn-mlx_token123"
	want := ProfilesList{
		Dir: "/Users/x/Lethean/profiles",
		Profiles: []Profile{
			{Name: "laptop.json", Path: "/Users/x/Lethean/profiles/laptop.json", Backend: "metal", Modified: 1716700000},
			{Name: "ultra.json", Path: "/Users/x/Lethean/profiles/ultra.json", Backend: "metal", Modified: 1716700100},
		},
	}
	srv := fakeAdminServer(t, tok, map[string]any{
		"GET /v1/admin/profiles": want,
	})
	defer srv.Close()

	admin, _ := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: tok})
	got, err := admin.Profiles(context.Background())
	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	if got.Dir != want.Dir || len(got.Profiles) != 2 {
		t.Fatalf("Profiles mismatch: %+v", got)
	}
}

// TestAdminReloadRequiresConfirm — server-side gate also blocks this
// client-side. Reload without confirm_machine returns error pre-flight,
// before any HTTP. Catches dropped-field accidents in callers.
func TestAdminReloadRequiresConfirm(t *testing.T) {
	srv := fakeAdminServer(t, "tok", nil)
	defer srv.Close()
	admin, _ := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: "tok"})
	err := admin.Reload(context.Background(), ReloadRequest{
		ModelPath: "/m/path",
	})
	if err == nil {
		t.Fatalf("expected error for missing confirm_machine, got nil")
	}
}

// TestAdminReloadPostsBody — the JSON sent to the server matches the
// caller's ReloadRequest exactly (catches accidental field renames).
func TestAdminReloadPostsBody(t *testing.T) {
	const tok = "tok"
	var captured ReloadRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/admin/serve/reload" {
			http.Error(w, "path", http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &captured)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	admin, _ := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: tok})
	req := ReloadRequest{
		ConfirmMachine: "machine-hash-xyz",
		ModelPath:      "/models/v2",
		ProfilePath:    "/profiles/ultra.json",
		ContextLength:  8192,
	}
	if err := admin.Reload(context.Background(), req); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if captured != req {
		t.Fatalf("server captured wrong body\n got: %+v\nwant: %+v", captured, req)
	}
}

// TestAdminDownloadFlow — Download returns the job id, then DownloadJob
// returns a status snapshot. The server here speaks the driver's real wire
// shape (go-mlx cmd/mlx adminDownloadJob): the kick body carries "repo", the
// response identifies the job by "id" (not "job_id"), and progress is byte
// counters (bytes_done / bytes_total). This guards the field-tag contract that
// previously drifted from the driver and silently broke the kick.
func TestAdminDownloadFlow(t *testing.T) {
	const tok = "tok"
	const jobID = "dl-job-42"

	var capturedRepo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/admin/models/download":
			var body DownloadRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			capturedRepo = body.Repo
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(DownloadJobStatus{
				ID:     jobID,
				Status: "pending",
				Repo:   "mlx-community/gemma-4-e2b-it-mxfp8",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/admin/models/download" && r.URL.Query().Get("job") == jobID:
			_ = json.NewEncoder(w).Encode(DownloadJobStatus{
				ID:         jobID,
				Status:     "done",
				Repo:       "mlx-community/gemma-4-e2b-it-mxfp8",
				DestPath:   "/Lethean/data/models/mlx-community/gemma-4-e2b-it-mxfp8/main",
				BytesTotal: 123_456_789,
				BytesDone:  123_456_789,
				FileCount:  3,
			})
		default:
			http.Error(w, "unrouted", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	admin, _ := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: tok})
	gotJob, err := admin.Download(context.Background(), DownloadRequest{Repo: "mlx-community/gemma-4-e2b-it-mxfp8"})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if gotJob != jobID {
		t.Fatalf("Download id = %q, want %q", gotJob, jobID)
	}
	if capturedRepo != "mlx-community/gemma-4-e2b-it-mxfp8" {
		t.Fatalf("server captured repo = %q, want the request's repo (wire tag must be \"repo\")", capturedRepo)
	}
	js, err := admin.DownloadJob(context.Background(), jobID)
	if err != nil {
		t.Fatalf("DownloadJob: %v", err)
	}
	if js.Status != "done" || js.BytesDone != 123_456_789 {
		t.Fatalf("DownloadJob = %+v, want status=done bytes_done=123456789", js)
	}
}

// TestEnsureRepoAllowed — the catalogue-download permit step. A fresh HOME
// (no allowlist) gets the file created with the repo; a second call for a
// different repo appends rather than replacing; a repeat of an existing repo
// is a no-op. The on-disk schema must match the driver's {"repos": [...]} so
// the driver's gate reads exactly what we wrote.
func TestEnsureRepoAllowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "Lethean", "data", "allowed-models.json")

	if err := ensureRepoAllowed("mlx-community/gemma-4-e2b-it-mxfp8"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	read := func() allowedModelsFile {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read allowlist: %v", err)
		}
		var f allowedModelsFile
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("allowlist not valid JSON in driver schema: %v\n%s", err, raw)
		}
		return f
	}
	if got := read().Repos; len(got) != 1 || got[0] != "mlx-community/gemma-4-e2b-it-mxfp8" {
		t.Fatalf("after first ensure repos = %v, want [the repo]", got)
	}

	// Append a different repo — must keep the first.
	if err := ensureRepoAllowed("google/gemma-4-e2b-it"); err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if got := read().Repos; len(got) != 2 {
		t.Fatalf("after second ensure repos = %v, want 2 entries (append, not replace)", got)
	}

	// Repeat an existing repo — no-op, count unchanged.
	if err := ensureRepoAllowed("google/gemma-4-e2b-it"); err != nil {
		t.Fatalf("idempotent ensure: %v", err)
	}
	if got := read().Repos; len(got) != 2 {
		t.Fatalf("after idempotent ensure repos = %v, want still 2 (no duplicate)", got)
	}
}

// TestAdminBadStatusSurfacesUpstreamBody — when the server returns
// 4xx, the error string should carry the upstream message so the CLI
// or UI can show the user what went wrong.
func TestAdminBadStatusSurfacesUpstreamBody(t *testing.T) {
	const tok = "tok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "repo not in allowlist", http.StatusForbidden)
	}))
	defer srv.Close()

	admin, _ := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: tok})
	_, err := admin.Download(context.Background(), DownloadRequest{Repo: "evil/repo"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("error should carry status + upstream body: %v", err)
	}
}

// TestAdminUnauthorizedIsExplicit — wrong token surfaces as 401 with
// the upstream auth message so the user knows to re-pair / rotate.
func TestAdminUnauthorizedIsExplicit(t *testing.T) {
	srv := fakeAdminServer(t, "correct-token", map[string]any{
		"GET /v1/admin/machine": MachineInfo{Hash: "x", Runtime: "metal"},
	})
	defer srv.Close()
	admin, _ := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: "wrong-token"})
	_, err := admin.Machine(context.Background())
	if err == nil {
		t.Fatalf("expected 401 error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should carry 401: %v", err)
	}
}

// writeFile is a small test helper — keeps the test file free of
// per-test file-IO boilerplate.
func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o600)
}

// TestAdmin_RequestFailedWrapping_Bad drives every remaining "request
// failed" wrap branch (Status/Profiles/Reload/DownloadJob/SFTStart/
// SFTStatus/SFTStop/SFTAdapters — each has its own doJSON call site) through
// one wrong-Bearer 401, so each method's local core.E(...) wrap line runs
// at least once. TestAdminUnauthorizedIsExplicit above already covers
// Machine's; Download's is covered by TestAdminBadStatusSurfacesUpstreamBody.
func TestAdmin_RequestFailedWrapping_Bad(t *testing.T) {
	srv := fakeAdminServer(t, "correct-token", nil) // every path 401s on the wrong token before any canned lookup
	defer srv.Close()
	admin, _ := NewAdmin(AdminConfig{BaseURL: srv.URL, Token: "wrong-token"})
	ctx := context.Background()

	if _, err := admin.Status(ctx); err == nil {
		t.Error("Status: expected wrapped request-failed error")
	}
	if _, err := admin.Profiles(ctx); err == nil {
		t.Error("Profiles: expected wrapped request-failed error")
	}
	if err := admin.Reload(ctx, ReloadRequest{ConfirmMachine: "m"}); err == nil {
		t.Error("Reload: expected wrapped request-failed error")
	}
	if _, err := admin.DownloadJob(ctx, "job-1"); err == nil {
		t.Error("DownloadJob: expected wrapped request-failed error")
	}
	if _, err := admin.SFTStart(ctx, SFTStartRequest{ModelPath: "/m", DatasetPath: "/d"}); err == nil {
		t.Error("SFTStart: expected wrapped request-failed error")
	}
	if _, err := admin.SFTStatus(ctx, "job-1"); err == nil {
		t.Error("SFTStatus: expected wrapped request-failed error")
	}
	if _, err := admin.SFTStop(ctx, "job-1"); err == nil {
		t.Error("SFTStop: expected wrapped request-failed error")
	}
	if _, err := admin.SFTAdapters(ctx); err == nil {
		t.Error("SFTAdapters: expected wrapped request-failed error")
	}
}

// TestAdmin_DownloadJob_Bad_EmptyJobID — the pre-flight validation guard,
// no HTTP call is even attempted.
func TestAdmin_DownloadJob_Bad_EmptyJobID(t *testing.T) {
	admin, _ := NewAdmin(AdminConfig{Token: "t"})
	if _, err := admin.DownloadJob(context.Background(), ""); err == nil {
		t.Fatal("expected an error for an empty job id")
	}
}

// TestAdmin_SFTStart_Bad_MissingDatasetPath — ModelPath alone isn't enough;
// this is a distinct validation line from the ModelPath-missing case that
// TestAdmin_RequestFailedWrapping_Bad's SFTStart call already exercises.
func TestAdmin_SFTStart_Bad_MissingDatasetPath(t *testing.T) {
	admin, _ := NewAdmin(AdminConfig{Token: "t"})
	if _, err := admin.SFTStart(context.Background(), SFTStartRequest{ModelPath: "/m"}); err == nil {
		t.Fatal("expected an error for a missing dataset_path")
	}
}

// TestAdmin_SFTStop_Bad_EmptyJobID — the pre-flight validation guard.
func TestAdmin_SFTStop_Bad_EmptyJobID(t *testing.T) {
	admin, _ := NewAdmin(AdminConfig{Token: "t"})
	if _, err := admin.SFTStop(context.Background(), ""); err == nil {
		t.Fatal("expected an error for an empty job id")
	}
}

// TestAdmin_NewAdmin_Ugly_DefaultTokenPathUnderHome — no explicit Token or
// TokenPath: NewAdmin must fall back to ~/Lethean/data/admin.token.
func TestAdmin_NewAdmin_Ugly_DefaultTokenPathUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tokDir := filepath.Join(home, "Lethean", "data")
	if err := os.MkdirAll(tokDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const tok = "lthn-mlx_default-path-token"
	if err := writeFile(t, filepath.Join(tokDir, "admin.token"), tok); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	srv := fakeAdminServer(t, tok, map[string]any{
		"GET /v1/admin/machine": MachineInfo{Hash: "default-path-ok"},
	})
	defer srv.Close()

	admin, err := NewAdmin(AdminConfig{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewAdmin: %v", err)
	}
	mi, err := admin.Machine(context.Background())
	if err != nil {
		t.Fatalf("Machine: %v", err)
	}
	if mi.Hash != "default-path-ok" {
		t.Fatalf("Machine.Hash = %q, want default-path-ok", mi.Hash)
	}
}

// TestAdmin_LoadTokenFromFile_Bad_PathIsDirectory — a non-ENOENT read
// failure (path is a directory, not a file) must NOT be mistaken for
// ErrNoTokenFile; it has to surface as a real error.
func TestAdmin_LoadTokenFromFile_Bad_PathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "admin.token")
	if err := os.Mkdir(tokPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := loadTokenFromFile(tokPath)
	if err == nil {
		t.Fatal("expected an error when the token path is a directory")
	}
	if errors.Is(err, ErrNoTokenFile) {
		t.Fatalf("a directory-as-path error must not be reported as ErrNoTokenFile: %v", err)
	}
}
