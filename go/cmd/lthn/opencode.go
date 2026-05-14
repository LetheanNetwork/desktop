// SPDX-Licence-Identifier: EUPL-1.2

package main

import (
	goio "io"
	"net/http"
	"strings"
	"time"

	core "dappco.re/go"
)

// cmdOpenCode dispatches `lthn opencode <verb>` — thin HTTP client
// over the opencode control endpoints exposed by `lthn serve`. Every
// verb posts/gets/deletes against http://localhost:8000/v1/api/opencode/
// so sandbox state lives in one process (the serve daemon).
//
// Requires `lthn serve` to be running on localhost:8000. When the
// serve daemon isn't up, the dial fails with a clear error.
//
// Verbs:
//
//	start              spawn a new sandbox, print the id
//	stop ID            stop + remove a sandbox by id
//	status             list running sandboxes
//	inspect ID         print one sandbox's record
func cmdOpenCode(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn opencode: missing verb (start / stop / status / inspect)\n")
		return 2
	}
	switch args[0] {
	case "start":
		return opencodeStart()
	case "stop":
		return opencodeStop(args[1:])
	case "status":
		return opencodeStatus()
	case "inspect":
		return opencodeInspect(args[1:])
	default:
		core.Print(core.Stderr(), "lthn opencode: unknown verb %q\n", args[0])
		return 2
	}
}

// opencodeBase is the control surface root on localhost.
const opencodeBase = "http://localhost:8000/v1/api/opencode/sandbox"

// httpClient is the shared client for control calls. Short timeout —
// spawn can take a moment for the docker pull, but lthn serve returns
// once the container is created (not when opencode-serve is healthy).
var httpClient = &http.Client{Timeout: 30 * time.Second}

func opencodeStart() int {
	req, _ := http.NewRequest(http.MethodPost, opencodeBase, nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode start: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode start: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

func opencodeStop(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode stop: usage: lthn opencode stop ID\n")
		return 2
	}
	req, _ := http.NewRequest(http.MethodDelete, opencodeBase+"/"+args[0], nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode stop: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode stop: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

func opencodeStatus() int {
	req, _ := http.NewRequest(http.MethodGet, opencodeBase, nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode status: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode status: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

func opencodeInspect(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode inspect: usage: lthn opencode inspect ID\n")
		return 2
	}
	req, _ := http.NewRequest(http.MethodGet, opencodeBase+"/"+args[0], nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode inspect: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode inspect: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// doRequest runs the HTTP call and returns (body, status, error).
func doRequest(req *http.Request) (string, int, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := goio.ReadAll(resp.Body)
	return strings.TrimSpace(string(raw)), resp.StatusCode, nil
}
