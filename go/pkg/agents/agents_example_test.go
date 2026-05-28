// SPDX-Licence-Identifier: EUPL-1.2

package agents_test

import (
	"dappco.re/lthn/desktop/pkg/agents"
)

func ExampleService_Status() {
	svc := agents.New(agents.Config{})
	r := svc.Status()
	if r.OK {
		_ = r.Value.(agents.StatusResult)
	}
}

func ExampleService_Resume() {
	// Answer a blocked run and relaunch it — the workspace is the blocked
	// run's Name from Status (org/repo/task-N).
	svc := agents.New(agents.Config{})
	r := svc.Resume(agents.ResumeRequest{Workspace: "core/go-io/task-4", Answer: "Use the shared notifier"})
	if r.OK {
		_ = r.Value.(agents.ResumeResult)
	}
}

func ExampleNew() {
	// Talk to the crew's loopback lthn-agent serve (DefaultBaseURL), or
	// point at an explicit address.
	_ = agents.New(agents.Config{})
	_ = agents.New(agents.Config{BaseURL: "http://127.0.0.1:9101"})
}
