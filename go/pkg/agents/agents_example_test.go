// SPDX-Licence-Identifier: EUPL-1.2

package agents_test

import (
	"dappco.re/lthn/desktop/pkg/agents"
)

func ExampleService_Prep() {
	// Prepare a workspace — clone/branch + assemble the agent prompt (issue,
	// brain recall, consumers, wiki, git log), versioned to a snapshot.
	svc := agents.New(agents.Config{})
	r := svc.Prep(agents.PrepRequest{Repo: "go-io", Issue: 15})
	if r.OK {
		_ = r.Value.(agents.PrepResult)
	}
}

func ExampleService_Workspaces() {
	// List every tracked workspace + its status — the Activity feed.
	svc := agents.New(agents.Config{})
	r := svc.Workspaces()
	if r.OK {
		_ = r.Value.([]agents.Workspace)
	}
}

func ExampleService_Resume() {
	// Answer a blocked run and relaunch it (workspace = its Name from Workspaces).
	svc := agents.New(agents.Config{})
	r := svc.Resume(agents.ResumeRequest{Workspace: "core/go-io/task-4", Answer: "Use the shared notifier"})
	if r.OK {
		_ = r.Value.(agents.ResumeResult)
	}
}

func ExampleNew() {
	// Zero-value talks to the hub MCP plane (:9202) for the channel listener;
	// CLI ops resolve the lthn-agent binary at call time.
	_ = agents.New(agents.Config{})
	_ = agents.New(agents.Config{MCPURL: "http://127.0.0.1:9202", MCPToken: "my-token"})
}
