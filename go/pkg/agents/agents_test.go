// SPDX-Licence-Identifier: EUPL-1.2

package agents_test

import (
	"net/http"
	"net/http/httptest"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/agents"
)

// --- Status (method on *Service) ---

func TestAgents_Service_Status_Good(t *core.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		core.AssertEqual(t, "/v1/tools/agentic_status", r.URL.Path)
		core.AssertEqual(t, http.MethodPost, r.Method)
		_, _ = w.Write([]byte(`{"success":true,"data":{"total":3,"running":1,"queued":1,"completed":1,"failed":0}}`))
	}))
	defer srv.Close()
	r := agents.New(agents.Config{BaseURL: srv.URL}).Status()
	core.AssertTrue(t, r.OK)
	c := r.Value.(agents.StatusCounts)
	core.AssertEqual(t, 3, c.Total)
	core.AssertEqual(t, 1, c.Running)
}

func TestAgents_Service_Status_Bad(t *core.T) {
	// Nothing listening → transport error → Fail (engine down), no panic.
	r := agents.New(agents.Config{BaseURL: "http://127.0.0.1:1"}).Status()
	core.AssertFalse(t, r.OK, "Status must Fail when serve is unreachable")
}

func TestAgents_Service_Status_Ugly(t *core.T) {
	// Tool reports success=false → Fail cleanly with the message.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
	}))
	defer srv.Close()
	r := agents.New(agents.Config{BaseURL: srv.URL}).Status()
	core.AssertFalse(t, r.OK)
}

// --- Dispatch (method on *Service) ---

func TestAgents_Service_Dispatch_Good(t *core.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		core.AssertEqual(t, "/v1/tools/agentic_dispatch", r.URL.Path)
		core.AssertEqual(t, http.MethodPost, r.Method)
		_, _ = w.Write([]byte(`{"success":true,"data":{"success":true,"agent":"codex","repo":"go-io","workspace_dir":".core/workspace/core/go-io/task-1","pid":1234}}`))
	}))
	defer srv.Close()
	r := agents.New(agents.Config{BaseURL: srv.URL}).Dispatch(agents.DispatchRequest{Repo: "go-io", Task: "fix tests", Agent: "codex"})
	core.AssertTrue(t, r.OK)
	out := r.Value.(agents.DispatchResult)
	core.AssertEqual(t, "go-io", out.Repo)
	core.AssertEqual(t, 1234, out.PID)
}

func TestAgents_Service_Dispatch_Bad(t *core.T) {
	// Missing repo/task → validated before any network → Fail (no panic).
	r := agents.New(agents.Config{BaseURL: "http://127.0.0.1:1"}).Dispatch(agents.DispatchRequest{Agent: "codex"})
	core.AssertFalse(t, r.OK, "Dispatch must Fail without repo+task")
}

func TestAgents_Service_Dispatch_Ugly(t *core.T) {
	// Tool reports failure → Fail cleanly with the message.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"error":"no such repo"}`))
	}))
	defer srv.Close()
	r := agents.New(agents.Config{BaseURL: srv.URL}).Dispatch(agents.DispatchRequest{Repo: "nope", Task: "x"})
	core.AssertFalse(t, r.OK)
}
