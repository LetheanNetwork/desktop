// SPDX-Licence-Identifier: EUPL-1.2

package deploys

import (
	"testing"

	core "dappco.re/go"
)

// TestServiceName — ServiceName returns the correct Wails binding namespace.
func TestServiceName(t *testing.T) {
	svc := &Service{}
	if svc.ServiceName() != "Deploys" {
		t.Fatalf("expected ServiceName='Deploys', got %q", svc.ServiceName())
	}
}

// TestDeployRecord_Fields — DeployRecord carries all fields the frontend expects.
func TestDeployRecord_Fields(t *testing.T) {
	rec := DeployRecord{
		ID:      "deploy-20260516-1432",
		Env:     "preview",
		By:      "Tobi",
		Commit:  "b8e034",
		Outcome: "success",
		Dur:     "58s",
	}
	if rec.ID == "" || rec.Env == "" || rec.By == "" || rec.Commit == "" {
		t.Fatal("DeployRecord missing required fields")
	}
}

// TestDeployRow_Fields — DeployRow carries all wire fields.
func TestDeployRow_Fields(t *testing.T) {
	row := DeployRow{
		Ts:      "14m",
		Env:     "staging",
		By:      "Tobi",
		Commit:  "a1b2c3",
		Outcome: "success",
		Dur:     "1m 04s",
	}
	if row.Ts == "" || row.Env == "" || row.Outcome == "" {
		t.Fatal("DeployRow missing required fields")
	}
}

// TestEnvRow_Fields — EnvRow carries all wire fields.
func TestEnvRow_Fields(t *testing.T) {
	env := EnvRow{
		Name:    "production",
		URL:     "lthn.ai",
		Version: "v0.1.8",
		Commit:  "4a82c1",
		Age:     "4d",
		Health:  "ok",
	}
	if env.Name == "" || env.Health == "" {
		t.Fatal("EnvRow missing required fields")
	}
}

// TestCreateInput_Fields — CreateInput carries all required fields.
func TestCreateInput_Fields(t *testing.T) {
	in := CreateInput{
		Env:     "preview",
		By:      "Tobi",
		Commit:  "b8e034",
		Outcome: "success",
		Dur:     "58s",
	}
	if in.Env == "" || in.By == "" || in.Commit == "" || in.Outcome == "" || in.Dur == "" {
		t.Fatal("CreateInput missing required fields")
	}
}

// TestListInput_Fields — ListInput carries env filter + limit.
func TestListInput_Fields(t *testing.T) {
	in := ListInput{Env: "preview", Limit: 10}
	if in.Limit <= 0 {
		t.Fatal("ListInput.Limit should be positive")
	}
}

// TestListOutput_Fields — ListOutput carries history + envs + total.
func TestListOutput_Fields(t *testing.T) {
	out := ListOutput{
		History: []DeployRow{{Ts: "4d", Env: "production", By: "Tobi", Commit: "abc", Outcome: "success", Dur: "1m"}},
		Envs:    []EnvRow{{Name: "production", Health: "ok"}},
		Total:   1,
	}
	if len(out.History) == 0 {
		t.Fatal("ListOutput.History must not be empty")
	}
	if len(out.Envs) == 0 {
		t.Fatal("ListOutput.Envs must not be empty")
	}
	if out.Total != 1 {
		t.Fatalf("ListOutput.Total expected 1, got %d", out.Total)
	}
}

// TestGetInput_Fields — GetInput carries the required ID.
func TestGetInput_Fields(t *testing.T) {
	in := GetInput{ID: "deploy-20260516-1432"}
	if in.ID == "" {
		t.Fatal("GetInput.ID must not be empty")
	}
}

// TestGetOutput_Fields — GetOutput carries record + notes.
func TestGetOutput_Fields(t *testing.T) {
	out := GetOutput{
		Record: DeployRecord{ID: "deploy-20260516-1432", Env: "preview"},
		Notes:  "Deploy went smoothly.",
	}
	if out.Record.ID == "" {
		t.Fatal("GetOutput.Record.ID must not be empty")
	}
}

// --- Service.List ---

func TestService_List_Good_ReturnsHistoryAndEnvs(t *testing.T) {
	deploysHomeFixture(t)
	rec1 := DeployRecord{
		ID: "deploy-20260516-1000", Env: "staging", By: "Ada", Commit: "aaa",
		Outcome: "success", Dur: "1m", Timestamp: mustParseTime(t, "2026-05-16T10:00:00Z"),
	}
	rec2 := DeployRecord{
		ID: "deploy-20260516-1200", Env: "production", By: "Tobi", Commit: "bbb",
		Outcome: "failed", Dur: "2m", Timestamp: mustParseTime(t, "2026-05-16T12:00:00Z"),
	}
	writeDeployFixture(t, rec1, "")
	writeDeployFixture(t, rec2, "")

	svc := &Service{}
	r := svc.List(ListInput{})
	if !r.OK {
		t.Fatalf("List failed: %v", r.Error())
	}
	out := r.Value.(ListOutput)
	if out.Total != 2 {
		t.Fatalf("expected total 2, got %d", out.Total)
	}
	if len(out.History) != 2 {
		t.Fatalf("expected 2 history rows, got %d", len(out.History))
	}
	if len(out.Envs) != 2 {
		t.Fatalf("expected 2 env rows, got %d", len(out.Envs))
	}
}

func TestService_List_Bad_FiltersByEnvAndCapsLimit(t *testing.T) {
	deploysHomeFixture(t)
	rec1 := DeployRecord{
		ID: "deploy-20260516-1000", Env: "staging", By: "Ada", Commit: "aaa",
		Outcome: "success", Dur: "1m", Timestamp: mustParseTime(t, "2026-05-16T10:00:00Z"),
	}
	rec2 := DeployRecord{
		ID: "deploy-20260516-1200", Env: "production", By: "Tobi", Commit: "bbb",
		Outcome: "success", Dur: "2m", Timestamp: mustParseTime(t, "2026-05-16T12:00:00Z"),
	}
	writeDeployFixture(t, rec1, "")
	writeDeployFixture(t, rec2, "")

	svc := &Service{}
	r := svc.List(ListInput{Env: "production", Limit: 1})
	if !r.OK {
		t.Fatalf("List failed: %v", r.Error())
	}
	out := r.Value.(ListOutput)
	if out.Total != 1 {
		t.Fatalf("expected filtered total 1, got %d", out.Total)
	}
	if len(out.History) != 1 || out.History[0].Env != "production" {
		t.Fatalf("expected 1 production row, got %+v", out.History)
	}
	// Envs are derived from the full unfiltered set.
	if len(out.Envs) != 2 {
		t.Fatalf("expected unfiltered env count 2, got %d", len(out.Envs))
	}
}

func TestService_List_Ugly_ReadAllFailurePropagates(t *testing.T) {
	deploysBrokenDirFixture(t)
	svc := &Service{}
	r := svc.List(ListInput{})
	if r.OK {
		t.Fatal("expected List to fail when readAll fails")
	}
}

// --- Service.Get ---

func TestService_Get_Good_ReturnsRecordAndNotes(t *testing.T) {
	deploysHomeFixture(t)
	rec := DeployRecord{
		ID: "deploy-20260516-1432", Env: "preview", By: "Tobi", Commit: "b8e034",
		Outcome: "success", Dur: "58s", Timestamp: mustParseTime(t, "2026-05-16T14:32:00Z"),
	}
	writeDeployFixture(t, rec, "All good.")

	svc := &Service{}
	r := svc.Get(GetInput{ID: rec.ID})
	if !r.OK {
		t.Fatalf("Get failed: %v", r.Error())
	}
	out := r.Value.(GetOutput)
	if out.Record.Env != "preview" {
		t.Fatalf("expected Env='preview', got %q", out.Record.Env)
	}
	if out.Notes != "All good.\n" {
		t.Fatalf("unexpected notes: %q", out.Notes)
	}
}

func TestService_Get_Bad_InvalidID(t *testing.T) {
	deploysHomeFixture(t)
	svc := &Service{}
	r := svc.Get(GetInput{ID: "../etc/passwd"})
	if r.OK {
		t.Fatal("expected Get to reject a traversal ID")
	}
}

func TestService_Get_Ugly_FileNotFound(t *testing.T) {
	deploysHomeFixture(t)
	svc := &Service{}
	r := svc.Get(GetInput{ID: "deploy-does-not-exist-0000"})
	if r.OK {
		t.Fatal("expected Get to fail for a missing file")
	}
}

func TestService_Get_Ugly_ParseFailurePropagates(t *testing.T) {
	deploysHomeFixture(t)
	dirR := deploysDir()
	if !dirR.OK {
		t.Fatalf("deploysDir: %v", dirR.Error())
	}
	dir := dirR.Value.(string)
	if r := core.WriteFile(core.PathJoin(dir, "deploy-broken-0000.md"), []byte("---\n[not, a, map]\n---\n"), 0o600); !r.OK {
		t.Fatalf("WriteFile: %v", r.Error())
	}

	svc := &Service{}
	r := svc.Get(GetInput{ID: "deploy-broken-0000"})
	if r.OK {
		t.Fatal("expected Get to fail on malformed frontmatter")
	}
}

// --- Service.Create ---

func TestService_Create_Good_PersistsRecord(t *testing.T) {
	deploysHomeFixture(t)
	svc := &Service{}
	r := svc.Create(CreateInput{
		Env: "preview", By: "Tobi", Commit: "b8e034",
		Outcome: "success", Dur: "58s", Notes: "All good.",
	})
	if !r.OK {
		t.Fatalf("Create failed: %v", r.Error())
	}
	out := r.Value.(CreateOutput)
	if out.ID == "" {
		t.Fatal("expected non-empty generated ID")
	}

	// Round-trip via Get confirms the write actually landed.
	getR := svc.Get(GetInput{ID: out.ID})
	if !getR.OK {
		t.Fatalf("round-trip Get failed: %v", getR.Error())
	}
}

func TestService_Create_Bad_MissingRequiredFields(t *testing.T) {
	deploysHomeFixture(t)
	svc := &Service{}
	cases := []CreateInput{
		{},
		{Env: "preview"},
		{Env: "preview", By: "Tobi"},
		{Env: "preview", By: "Tobi", Commit: "abc"},
		{Env: "preview", By: "Tobi", Commit: "abc", Outcome: "success"},
	}
	for i, in := range cases {
		if r := svc.Create(in); r.OK {
			t.Fatalf("case %d: expected failure for incomplete input %+v", i, in)
		}
	}
}

func TestService_Create_Ugly_DirFailurePropagates(t *testing.T) {
	deploysBrokenDirFixture(t)
	svc := &Service{}
	r := svc.Create(CreateInput{Env: "preview", By: "Tobi", Commit: "abc", Outcome: "success", Dur: "1s"})
	if r.OK {
		t.Fatal("expected Create to fail when the deploys dir cannot be created")
	}
}
