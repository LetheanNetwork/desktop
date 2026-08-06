// SPDX-License-Identifier: EUPL-1.2

package lemma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// wailsAdminFixture seeds a token file on disk and returns a WailsService
// wired to a fakeAdminServer (defined in admin_test.go) scripted with
// responses, plus the server itself for callers that want to inspect it.
func wailsAdminFixture(t *testing.T, tok string, responses map[string]any) *WailsService {
	t.Helper()
	srv := fakeAdminServer(t, tok, responses)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "admin.token")
	if err := writeFile(t, tokPath, tok); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return NewWailsService(AdminConfig{BaseURL: srv.URL, TokenPath: tokPath, Timeout: 2 * time.Second})
}

// --- NewWailsService / ServiceName / ConfigureEndpoint ---

func TestWails_NewWailsService_Good(t *testing.T) {
	svc := NewWailsService(AdminConfig{BaseURL: "http://127.0.0.1:11434"})
	if svc == nil {
		t.Fatal("NewWailsService returned nil")
	}
	if svc.ServiceName() != "Lemma" {
		t.Fatalf("ServiceName = %q, want Lemma", svc.ServiceName())
	}
}

func TestWails_ServiceName_Good(t *testing.T) {
	svc := NewWailsService(AdminConfig{})
	if got := svc.ServiceName(); got != "Lemma" {
		t.Fatalf("ServiceName = %q, want Lemma", got)
	}
}

func TestWails_ConfigureEndpoint_Good(t *testing.T) {
	const tok = "tok-configure"
	svc := wailsAdminFixture(t, tok, map[string]any{
		"GET /v1/admin/machine": MachineInfo{Hash: "h1", Runtime: "metal"},
	})
	mi, err := svc.Machine(context.Background())
	if err != nil {
		t.Fatalf("Machine (pre-reconfigure): %v", err)
	}
	if mi.Hash != "h1" {
		t.Fatalf("Machine.Hash = %q, want h1", mi.Hash)
	}

	// Point at an unreachable endpoint — subsequent calls must now fail,
	// proving ConfigureEndpoint actually mutates the live config.
	svc.ConfigureEndpoint("http://127.0.0.1:1")
	if _, err := svc.Machine(context.Background()); err == nil {
		t.Fatal("expected an error after redirecting to an unreachable endpoint")
	}
}

// --- admin() lazy-build behaviour ---

func TestWails_admin_Bad_NoTokenFileReadsAreNilNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{}) // zero config -> default token path under $HOME
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status with no token file must be (zero, nil), got err=%v", err)
	}
	if st != (ServeStatus{}) {
		t.Fatalf("Status = %+v, want zero value", st)
	}
}

func TestWails_admin_Ugly_TokenFileEmptyIsARealError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "admin.token")
	if err := writeFile(t, tokPath, "   \n  "); err != nil {
		t.Fatalf("seed empty token: %v", err)
	}
	svc := NewWailsService(AdminConfig{TokenPath: tokPath})
	_, err := svc.Status(context.Background())
	if err == nil {
		t.Fatal("expected a real error for an empty (not missing) token file")
	}
	if !strings.Contains(err.Error(), "admin client unavailable") {
		t.Fatalf("error should be wrapped as admin client unavailable, got %v", err)
	}
}

// --- Status / Machine / Profiles (reads) ---

func TestWails_Status_Good(t *testing.T) {
	const tok = "tok-status"
	want := ServeStatus{ModelPath: "/models/x", Runtime: "metal", LoadedAtUnix: 123}
	svc := wailsAdminFixture(t, tok, map[string]any{
		"GET /v1/admin/serve/status": want,
	})
	got, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got != want {
		t.Fatalf("Status = %+v, want %+v", got, want)
	}
}

func TestWails_Machine_Good(t *testing.T) {
	const tok = "tok-machine"
	svc := wailsAdminFixture(t, tok, map[string]any{
		"GET /v1/admin/machine": MachineInfo{Hash: "abc", Runtime: "metal"},
	})
	mi, err := svc.Machine(context.Background())
	if err != nil {
		t.Fatalf("Machine: %v", err)
	}
	if mi.Hash != "abc" {
		t.Fatalf("Machine.Hash = %q, want abc", mi.Hash)
	}
}

func TestWails_Machine_Bad_NoTokenFileIsZeroNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	mi, err := svc.Machine(context.Background())
	if err != nil {
		t.Fatalf("Machine with no token = err %v, want nil", err)
	}
	if mi.Hash != "" || mi.Runtime != "" || mi.Extra != nil {
		t.Fatalf("Machine = %+v, want zero value", mi)
	}
}

func TestWails_Profiles_Bad_NoTokenFileIsZeroNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	pl, err := svc.Profiles(context.Background())
	if err != nil {
		t.Fatalf("Profiles with no token = err %v, want nil", err)
	}
	if pl.Dir != "" || pl.Profiles != nil {
		t.Fatalf("Profiles = %+v, want zero value", pl)
	}
}

func TestWails_Profiles_Good(t *testing.T) {
	const tok = "tok-profiles"
	want := ProfilesList{Dir: "/profiles", Profiles: []Profile{{Name: "a.json"}}}
	svc := wailsAdminFixture(t, tok, map[string]any{
		"GET /v1/admin/profiles": want,
	})
	got, err := svc.Profiles(context.Background())
	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	if got.Dir != want.Dir || len(got.Profiles) != 1 {
		t.Fatalf("Profiles = %+v, want %+v", got, want)
	}
}

// --- Reload ---

func TestWails_Reload_Good_NoAdapterRoutesHostServe(t *testing.T) {
	var captured HostServeRequest
	hostSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(hostServeResponse{OK: true})
	}))
	defer hostSrv.Close()

	svc := NewWailsService(AdminConfig{})
	svc.hostCfg = HostConfig{BaseURL: hostSrv.URL, Timeout: 2 * time.Second}

	err := svc.Reload(context.Background(), ReloadRequest{ModelPath: "/models/y", ProfilePath: "/p.json"})
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if captured.Model != "/models/y" || captured.Profile != "/p.json" {
		t.Fatalf("host serve did not receive the reload request: %+v", captured)
	}
}

func TestWails_Reload_Bad_AdapterRoutesAdminButTokenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	err := svc.Reload(context.Background(), ReloadRequest{ModelPath: "/m", AdapterPath: "/adapters/x"})
	if err != errLemmaNotConfigured {
		t.Fatalf("Reload with adapter + no token = %v, want errLemmaNotConfigured", err)
	}
}

func TestWails_Reload_Ugly_AdapterRoutesAdminSuccess(t *testing.T) {
	const tok = "tok-reload-adapter"
	var captured ReloadRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+tok {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "admin.token")
	if err := writeFile(t, tokPath, tok); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	svc := NewWailsService(AdminConfig{BaseURL: srv.URL, TokenPath: tokPath, Timeout: 2 * time.Second})

	err := svc.Reload(context.Background(), ReloadRequest{
		ConfirmMachine: "machine-hash",
		ModelPath:      "/m",
		AdapterPath:    "/adapters/lek2",
	})
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if captured.AdapterPath != "/adapters/lek2" {
		t.Fatalf("admin reload did not receive the adapter path: %+v", captured)
	}
}

// --- Download / DownloadJob ---

func TestWails_Download_Bad_EmptyRepo(t *testing.T) {
	svc := NewWailsService(AdminConfig{})
	_, err := svc.Download(context.Background(), DownloadRequest{})
	if err == nil {
		t.Fatal("expected an error for an empty repo")
	}
}

func TestWails_Download_Bad_NoTokenFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	_, err := svc.Download(context.Background(), DownloadRequest{Repo: "org/model"})
	if err != errLemmaNotConfigured {
		t.Fatalf("Download with no token = %v, want errLemmaNotConfigured", err)
	}
}

func TestWails_Download_Good_PermitsRepoThenKicksJob(t *testing.T) {
	const tok = "tok-download"
	const jobID = "job-1"
	const repo = "mlx-community/gemma-4-e2b-it-mxfp8"
	srv := fakeAdminServer(t, tok, map[string]any{
		"POST /v1/admin/models/download": DownloadJobStatus{ID: jobID, Status: "pending", Repo: repo},
	})
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	tokPath := filepath.Join(dir, "admin.token")
	if err := writeFile(t, tokPath, tok); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	svc := NewWailsService(AdminConfig{BaseURL: srv.URL, TokenPath: tokPath, Timeout: 2 * time.Second})

	id, err := svc.Download(context.Background(), DownloadRequest{Repo: repo})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if id != jobID {
		t.Fatalf("Download id = %q, want %q", id, jobID)
	}

	allowPath := filepath.Join(home, "Lethean", "data", "allowed-models.json")
	raw, rerr := os.ReadFile(allowPath)
	if rerr != nil {
		t.Fatalf("read allowlist: %v", rerr)
	}
	var got allowedModelsFile
	if jerr := json.Unmarshal(raw, &got); jerr != nil {
		t.Fatalf("allowlist not valid JSON: %v\n%s", jerr, raw)
	}
	if len(got.Repos) != 1 || got.Repos[0] != repo {
		t.Fatalf("allowlist = %v, want [%s]", got.Repos, repo)
	}
}

func TestWails_DownloadJob_Good(t *testing.T) {
	const tok = "tok-downloadjob"
	svc := wailsAdminFixture(t, tok, map[string]any{
		"GET /v1/admin/models/download": DownloadJobStatus{ID: "j1", Status: "done"},
	})
	js, err := svc.DownloadJob(context.Background(), "j1")
	if err != nil {
		t.Fatalf("DownloadJob: %v", err)
	}
	if js.Status != "done" {
		t.Fatalf("DownloadJob.Status = %q, want done", js.Status)
	}
}

func TestWails_DownloadJob_Bad_NoTokenFileIsZeroNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	js, err := svc.DownloadJob(context.Background(), "job-x")
	if err != nil {
		t.Fatalf("DownloadJob with no token = err %v, want nil", err)
	}
	if js != (DownloadJobStatus{}) {
		t.Fatalf("DownloadJob = %+v, want zero value", js)
	}
}

// --- SFT lifecycle (Start/Status/Stop/Adapters) ---

func TestWails_SFTStart_Good(t *testing.T) {
	const tok = "tok-sftstart"
	want := SFTJob{JobID: "sft-1", State: "running"}
	svc := wailsAdminFixture(t, tok, map[string]any{
		"POST /v1/admin/sft/start": want,
	})
	job, err := svc.SFTStart(context.Background(), SFTStartRequest{ModelPath: "/m", DatasetPath: "/d.jsonl"})
	if err != nil {
		t.Fatalf("SFTStart: %v", err)
	}
	if job.JobID != want.JobID {
		t.Fatalf("SFTStart.JobID = %q, want %q", job.JobID, want.JobID)
	}
}

func TestWails_SFTStart_Bad_NoTokenFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	_, err := svc.SFTStart(context.Background(), SFTStartRequest{ModelPath: "/m", DatasetPath: "/d"})
	if err != errLemmaNotConfigured {
		t.Fatalf("SFTStart with no token = %v, want errLemmaNotConfigured", err)
	}
}

func TestWails_SFTStart_Ugly_ValidationErrorSurfacesFromAdmin(t *testing.T) {
	const tok = "tok-sftstart-bad"
	svc := wailsAdminFixture(t, tok, nil)
	_, err := svc.SFTStart(context.Background(), SFTStartRequest{}) // missing model_path / dataset_path
	if err == nil {
		t.Fatal("expected admin.SFTStart's validation error to surface")
	}
}

func TestWails_SFTStatus_Good(t *testing.T) {
	const tok = "tok-sftstatus"
	want := SFTJob{JobID: "sft-2", State: "done", LastLoss: 0.42}
	svc := wailsAdminFixture(t, tok, map[string]any{
		"GET /v1/admin/sft/status": want,
	})
	job, err := svc.SFTStatus(context.Background(), "")
	if err != nil {
		t.Fatalf("SFTStatus: %v", err)
	}
	if job.State != "done" {
		t.Fatalf("SFTStatus.State = %q, want done", job.State)
	}
}

func TestWails_SFTStatus_Bad_NoTokenFileIsZeroNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	job, err := svc.SFTStatus(context.Background(), "job-x")
	if err != nil {
		t.Fatalf("SFTStatus with no token = err %v, want nil", err)
	}
	if job.JobID != "" || job.State != "" || job.Loss != nil {
		t.Fatalf("SFTStatus = %+v, want zero value", job)
	}
}

func TestWails_SFTStop_Good(t *testing.T) {
	const tok = "tok-sftstop"
	want := SFTJob{JobID: "sft-3", State: "stopped"}
	svc := wailsAdminFixture(t, tok, map[string]any{
		"POST /v1/admin/sft/stop": want,
	})
	job, err := svc.SFTStop(context.Background(), "sft-3")
	if err != nil {
		t.Fatalf("SFTStop: %v", err)
	}
	if job.State != "stopped" {
		t.Fatalf("SFTStop.State = %q, want stopped", job.State)
	}
}

func TestWails_SFTStop_Bad_NoTokenFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	_, err := svc.SFTStop(context.Background(), "sft-3")
	if err != errLemmaNotConfigured {
		t.Fatalf("SFTStop with no token = %v, want errLemmaNotConfigured", err)
	}
}

func TestWails_SFTAdapters_Good(t *testing.T) {
	const tok = "tok-sftadapters"
	want := SFTAdaptersList{Dir: "/adapters", Adapters: []SFTAdapter{{Name: "lek2-rank8"}}}
	svc := wailsAdminFixture(t, tok, map[string]any{
		"GET /v1/admin/sft/adapters": want,
	})
	list, err := svc.SFTAdapters(context.Background())
	if err != nil {
		t.Fatalf("SFTAdapters: %v", err)
	}
	if len(list.Adapters) != 1 || list.Adapters[0].Name != "lek2-rank8" {
		t.Fatalf("SFTAdapters = %+v, want one adapter named lek2-rank8", list)
	}
}

func TestWails_SFTAdapters_Bad_NoTokenFileIsZeroNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := NewWailsService(AdminConfig{})
	list, err := svc.SFTAdapters(context.Background())
	if err != nil {
		t.Fatalf("SFTAdapters with no token = err %v, want nil", err)
	}
	if list.Dir != "" || list.Adapters != nil {
		t.Fatalf("SFTAdapters = %+v, want zero value", list)
	}
}
