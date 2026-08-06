// SPDX-Licence-Identifier: EUPL-1.2

// coverage_gap_test.go — closes the remaining reachable branches in
// runner_group.go (handleChat malformed-body + violatesChatCaps'
// per-message and cumulative-size axes) and sdk.go (ExportSpec's
// no-api-service + write-failure branches, GenerateSDK's
// tool-not-on-PATH branch). See handleModels_test.go for the two
// runner_group.go branches that are NOT reachable through the real
// runner.Service and are documented there instead of faked here.

package api_test

import (
	"net/http/httptest"

	core "dappco.re/go"
	coreapi "dappco.re/go/api"

	lthnapi "dappco.re/lthn/desktop/pkg/api"
	"dappco.re/lthn/desktop/pkg/runner"
)

// TestRunnerGroup_Chat_400OnMalformedJSON — handleChat's own
// c.ShouldBindJSON error branch (distinct from handleGenerate's, which
// TestRunnerGroup_Generate_400OnMissingPrompt already covers).
func TestRunnerGroup_Chat_400OnMalformedJSON(t *core.T) {
	engine := newTestEngine(t)
	req := core.NewHTTPRequest(core.MethodPost, "/v1/runner/chat",
		core.NewReader(`{"messages": not-json`)).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
}

// TestRunnerGroup_Chat_PerMessageOversized_Bad — a single message whose
// Role+Content size exceeds runner.MaxPromptBytes trips
// violatesChatCaps' per-message axis (distinct from
// TestRunnerGroup_Chat_AtCapNoReject_Good, which sits exactly AT the
// cap and must NOT trip it, and from
// TestRunnerGroup_Chat_PerMessageCaps_Bad, which trips the count axis).
func TestRunnerGroup_Chat_PerMessageOversized_Bad(t *core.T) {
	engine := newTestEngine(t)

	role := "u"
	contentLen := runner.MaxPromptBytes - len(role) + 1 // one byte over the per-message cap
	content := make([]byte, contentLen)
	for i := range content {
		content[i] = 'X'
	}
	body := []byte(`{"messages":[{"role":"u","content":"`)
	body = append(body, content...)
	body = append(body, []byte(`"}]}`)...)

	req := core.NewHTTPRequest(core.MethodPost, "/v1/runner/chat",
		core.NewReader(string(body))).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	respBody := w.Body.String()
	core.AssertTrue(t, core.Contains(respBody, "exceeds") && core.Contains(respBody, "byte cap"),
		"reject body must name the per-message cap that fired; got: "+respBody)
}

// TestRunnerGroup_Chat_CumulativeOversized_Bad — several messages each
// individually under runner.MaxPromptBytes, and under
// runner.MaxChatMessages in count, but whose running total crosses
// runner.MaxChatTotalBytes trips violatesChatCaps' cumulative axis.
func TestRunnerGroup_Chat_CumulativeOversized_Bad(t *core.T) {
	engine := newTestEngine(t)

	// Each message ~600 KiB (well under the 1 MiB per-message cap);
	// 14 of them cross the 8 MiB cumulative cap while staying far under
	// the 2000-message count cap.
	const perMsg = 600 << 10
	const count = 14
	content := make([]byte, perMsg)
	for i := range content {
		content[i] = 'Y'
	}
	buf := []byte(`{"messages":[`)
	for i := 0; i < count; i++ {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, []byte(`{"role":"u","content":"`)...)
		buf = append(buf, content...)
		buf = append(buf, []byte(`"}`)...)
	}
	buf = append(buf, []byte(`]}`)...)

	req := core.NewHTTPRequest(core.MethodPost, "/v1/runner/chat",
		core.NewReader(string(buf))).Value.(*core.Request)
	req.Header.Set(contentTypeHeader, applicationJSON)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	core.AssertEqual(t, core.StatusBadRequest, w.Code)
	respBody := w.Body.String()
	core.AssertTrue(t, core.Contains(respBody, "cumulative message size"),
		"reject body must name the cumulative cap that fired; got: "+respBody)
}

// TestSDK_ExportSpec_NoAPIServiceOnCore_Bad — ExportSpec's own
// apiSvc-not-registered branch, distinct from TestSDK_ExportSpec_
// NilCoreFails (core itself nil) — here the Core is real but never had
// the coreapi.Service wired on it (mirrors TestRunnerGroup_Register_
// NoAPIServiceIsOk's Register-side twin).
func TestSDK_ExportSpec_NoAPIServiceOnCore_Bad(t *core.T) {
	c := core.New()
	defer c.ServiceShutdown(core.Background())
	r := lthnapi.ExportSpec(c, "yaml", "/tmp/unused.yaml", lthnapi.DefaultSpecInfo())
	core.AssertEqual(t, false, r.OK)
	core.AssertTrue(t, core.Contains(r.Error(), "api service not registered"),
		"expected the not-registered diagnostic, got: "+r.Error())
}

// TestSDK_ExportSpec_WriteFailsWhenPathIsDirectory_Bad — ExportSpec
// forwards coreapi.ExportSpecToFile's write failure. A directory
// already occupying the destination path is a portable, hermetic way
// to force the underlying WriteAtomic to fail (EISDIR) without touching
// filesystem permission bits.
func TestSDK_ExportSpec_WriteFailsWhenPathIsDirectory_Bad(t *core.T) {
	c := core.New(
		core.WithName("api", coreapi.NewService(coreapi.ApiConfig{})),
	)
	_ = c.ServiceStartup(core.Background(), nil)
	defer c.ServiceShutdown(core.Background())

	dir := t.TempDir()
	blocker := core.PathJoin(dir, "openapi.yaml")
	if mk := core.MkdirAll(blocker, 0o755); !mk.OK {
		t.Fatalf("seed blocking directory: %s", mk.Error())
	}

	r := lthnapi.ExportSpec(c, "yaml", blocker, lthnapi.DefaultSpecInfo())
	if r.OK {
		t.Fatal("ExportSpec must fail when the destination path is occupied by a directory")
	}
}

// TestSDK_GenerateSDK_ToolNotOnPath_Bad — GenerateSDK's gen.Available()
// == false branch. Clearing PATH to an empty directory makes the
// underlying core.App{}.Find lookup fail deterministically regardless
// of whether openapi-generator-cli happens to be installed on the host
// running the test — hermetic, no network, no dependency on the
// generation itself (which the package-level doc comment on sdk_test.go
// already explains is out of scope: it shells out to a JDK-backed CLI).
func TestSDK_GenerateSDK_ToolNotOnPath_Bad(t *core.T) {
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)

	specPath := core.PathJoin(t.TempDir(), "openapi.yaml")
	if w := core.WriteFile(specPath, []byte("openapi: 3.0.0\n"), 0o600); !w.OK {
		t.Fatalf("seed spec file: %s", w.Error())
	}

	r := lthnapi.GenerateSDK(core.Background(), specPath, t.TempDir(), "typescript-fetch", "lthn-api")
	if r.OK {
		t.Fatal("GenerateSDK must fail when openapi-generator-cli is not on PATH")
	}
	if !core.Contains(r.Error(), "openapi-generator-cli not on PATH") {
		t.Fatalf("expected the not-on-PATH diagnostic, got: %s", r.Error())
	}
}
