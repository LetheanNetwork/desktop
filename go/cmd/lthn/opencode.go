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
		core.Print(core.Stderr(), "lthn opencode: missing verb (start / stop / status / inspect / profile / merge-host-config / providers / enable / disable / enabled / tui / web / upgrade / import / imports)\n")
		return 2
	}
	switch args[0] {
	case "start":
		return opencodeStart(args[1:])
	case "stop":
		return opencodeStop(args[1:])
	case "status":
		return opencodeStatus()
	case "inspect":
		return opencodeInspect(args[1:])
	case "profile":
		return opencodeProfile(args[1:])
	case "merge-host-config":
		return opencodeMergeHostConfig(args[1:])
	case "providers":
		return opencodeProviders(args[1:])
	case "enable":
		return opencodeEnable(args[1:])
	case "disable":
		return opencodeDisable()
	case "enabled":
		return opencodeEnabled()
	case "tui":
		return opencodeTUI(args[1:])
	case "upgrade":
		return opencodeUpgrade()
	case "web":
		return opencodeWeb(args[1:])
	case "import":
		return opencodeImport()
	case "imports":
		return opencodeImports(args[1:])
	default:
		core.Print(core.Stderr(), "lthn opencode: unknown verb %q\n", args[0])
		return 2
	}
}

// opencodeBase is the control surface root on localhost.
const (
	opencodeBase           = "http://localhost:8000/v1/api/opencode/sandbox"
	opencodeProfBase       = "http://localhost:8000/v1/api/opencode/profile"
	opencodeHostConfigBase = "http://localhost:8000/v1/api/opencode/host-config"
	opencodeEnableBase     = "http://localhost:8000/v1/api/opencode/enable"
	opencodeDisableBase    = "http://localhost:8000/v1/api/opencode/disable"
	opencodeEnabledBase    = "http://localhost:8000/v1/api/opencode/enabled"
)

// httpClient is the shared client for control calls. Generous timeout —
// spawn waits for health + applies PATCH /global/config synchronously.
var httpClient = &http.Client{Timeout: 60 * time.Second}

// opencodeStart accepts `--profile NAME` (or `--profile=NAME`).
//
// Usage example:
//
//	lthn opencode start                       # uses default profile
//	lthn opencode start --profile code-review # uses named profile
func opencodeStart(args []string) int {
	profile := ""
	for i := 0; i < len(args); i++ {
		k, v, valid := core.ParseFlag(args[i])
		if !valid {
			continue
		}
		if k == "profile" {
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			profile = v
		}
	}
	payload := core.JSONMarshalString(map[string]string{"profile": profile})
	req, _ := http.NewRequest(http.MethodPost, opencodeBase, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
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

// opencodeProfile dispatches `lthn opencode profile <verb>`.
//
// Verbs:
//
//	list                list all stored profiles
//	show NAME           show one profile
//	save PATH           upsert a profile from a JSON file
//	delete NAME         delete a profile (cannot delete "default")
func opencodeProfile(args []string) int {
	if len(args) == 0 {
		core.Print(core.Stderr(), "lthn opencode profile: missing verb (list / show / save / delete)\n")
		return 2
	}
	switch args[0] {
	case "list":
		return profileList()
	case "show":
		return profileShow(args[1:])
	case "save":
		return profileSave(args[1:])
	case "delete":
		return profileDelete(args[1:])
	default:
		core.Print(core.Stderr(), "lthn opencode profile: unknown verb %q\n", args[0])
		return 2
	}
}

func profileList() int {
	req, _ := http.NewRequest(http.MethodGet, opencodeProfBase, nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode profile list: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode profile list: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

func profileShow(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode profile show: usage: lthn opencode profile show NAME\n")
		return 2
	}
	req, _ := http.NewRequest(http.MethodGet, opencodeProfBase+"/"+args[0], nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode profile show: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode profile show: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

func profileSave(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode profile save: usage: lthn opencode profile save PATH (a JSON file)\n")
		return 2
	}
	raw := core.ReadFile(args[0])
	if !raw.OK {
		core.Print(core.Stderr(), "lthn opencode profile save: %s\n", raw.Error())
		return 1
	}
	data, _ := raw.Value.([]byte)
	req, _ := http.NewRequest(http.MethodPost, opencodeProfBase, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode profile save: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode profile save: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

func profileDelete(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode profile delete: usage: lthn opencode profile delete NAME\n")
		return 2
	}
	req, _ := http.NewRequest(http.MethodDelete, opencodeProfBase+"/"+args[0], nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode profile delete: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode profile delete: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeMergeHostConfig writes the named profile's provider block
// into ~/.config/opencode/opencode.json. Flags:
//
//	--profile NAME    profile to merge (default: "default")
//	--force           overwrite existing provider.lthn block
//
// Usage example:
//
//	lthn opencode merge-host-config              # default profile
//	lthn opencode merge-host-config --profile dev
//	lthn opencode merge-host-config --force      # confirm overwrite
func opencodeMergeHostConfig(args []string) int {
	profile := ""
	force := false
	for i := 0; i < len(args); i++ {
		k, v, valid := core.ParseFlag(args[i])
		if !valid {
			continue
		}
		switch k {
		case "profile":
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			profile = v
		case "force":
			force = true
		}
	}
	payload := core.JSONMarshalString(map[string]any{
		"profile": profile,
		"force":   force,
	})
	req, _ := http.NewRequest(http.MethodPost, opencodeHostConfigBase, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode merge-host-config: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code == http.StatusConflict {
		core.Print(core.Stderr(), "lthn opencode merge-host-config: conflict — %s\n", body)
		core.Print(core.Stderr(), "hint: pass --force to overwrite the existing provider.lthn block.\n")
		return 2
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode merge-host-config: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeProviders returns the loaded provider list for a running
// sandbox — pass-through of opencode-serve's /provider response.
//
// Usage example:
//
//	lthn opencode providers oc-1735843891234
func opencodeProviders(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode providers: usage: lthn opencode providers ID\n")
		return 2
	}
	req, _ := http.NewRequest(http.MethodGet, opencodeBase+"/"+args[0]+"/providers", nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode providers: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode providers: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeImport runs the host-opencode import cycle. Prints the
// resulting summary (counts of projects + providers imported).
//
// Usage example:
//
//	lthn opencode import
//	# {"projects":9,"providers":21,"providers_with_auth":1}
func opencodeImport() int {
	req, _ := http.NewRequest(http.MethodPost,
		"http://localhost:8000/v1/api/opencode/import", nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode import: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode import: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeImports lists imported rows. Default lists projects;
// `lthn opencode imports providers` lists provider definitions.
//
// Usage example:
//
//	lthn opencode imports               # projects, newest first
//	lthn opencode imports providers     # provider definitions
func opencodeImports(args []string) int {
	url := "http://localhost:8000/v1/api/opencode/imports"
	if len(args) > 0 && args[0] == "providers" {
		url += "/providers"
	}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode imports: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode imports: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeWeb prints the direct-bind URL for the named sandbox's
// web UI (with Basic auth credentials embedded). Useful in serve
// mode where the lthn-window path doesn't apply — paste the URL
// into your browser. In GUI mode the same flow lands in an lthn
// window via the "Open in window" button on the integrations card.
//
// Usage example:
//
//	lthn opencode web oc-1735843891234
//	# → http://opencode:hex@127.0.0.1:61830/
func opencodeWeb(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode web: usage: lthn opencode web ID\n")
		return 2
	}
	req, _ := http.NewRequest(http.MethodGet, opencodeBase+"/"+args[0]+"/web", nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode web: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode web: HTTP %d — %s\n", code, body)
		return 1
	}
	// Print the raw URL on its own line so iTerm2 / Terminal /
	// VS Code / WezTerm etc. auto-detect it as a hyperlink (cmd-
	// click in most terminals opens in the user's default browser).
	// We avoid the JSON envelope here on purpose — clickability
	// beats machine-parseable for an interactive `lthn opencode
	// web` invocation.
	var resp struct {
		URL string `json:"url"`
	}
	if r := core.JSONUnmarshalString(body, &resp); !r.OK || resp.URL == "" {
		// Unknown shape — fall back to printing the raw body so we
		// don't swallow anything useful the server returned.
		core.Print(core.Stdout(), "%s\n", body)
		return 0
	}
	core.Print(core.Stdout(), "%s\n", resp.URL)
	return 0
}

// opencodeUpgrade pulls lthn/dev:latest + restarts any running
// sandboxes on the new image when the digest changed.
//
// Usage example:
//
//	lthn opencode upgrade
//	# {"updated":true,"digest":"sha256:...","restarted":["oc-..."]}
func opencodeUpgrade() int {
	req, _ := http.NewRequest(http.MethodPost,
		"http://localhost:8000/v1/api/opencode/upgrade", nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode upgrade: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode upgrade: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeTUI spawns `<runtime> exec -it <container> opencode` in
// the user's default terminal — macOS / Linux / Windows aware.
//
// Usage example:
//
//	lthn opencode tui oc-1735843891234
func opencodeTUI(args []string) int {
	if len(args) < 1 {
		core.Print(core.Stderr(), "lthn opencode tui: usage: lthn opencode tui ID\n")
		return 2
	}
	req, _ := http.NewRequest(http.MethodPost, opencodeBase+"/"+args[0]+"/tui", nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode tui: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode tui: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeEnable persists `opencode.serve.enabled = true` + spawns
// a sandbox if none is running. Idempotent.
//
// Usage example:
//
//	lthn opencode enable                # default profile
//	lthn opencode enable --profile code-review
func opencodeEnable(args []string) int {
	profile := ""
	for i := 0; i < len(args); i++ {
		k, v, valid := core.ParseFlag(args[i])
		if !valid {
			continue
		}
		if k == "profile" {
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			profile = v
		}
	}
	payload := core.JSONMarshalString(map[string]string{"profile": profile})
	req, _ := http.NewRequest(http.MethodPost, opencodeEnableBase, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode enable: %s\n", err)
		core.Print(core.Stderr(), "hint: is `lthn serve` running on :8000?\n")
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode enable: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeDisable persists `opencode.serve.enabled = false` + stops
// any running sandboxes.
func opencodeDisable() int {
	req, _ := http.NewRequest(http.MethodPost, opencodeDisableBase, nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode disable: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode disable: HTTP %d — %s\n", code, body)
		return 1
	}
	core.Print(core.Stdout(), "%s\n", body)
	return 0
}

// opencodeEnabled prints the persisted enabled flag.
func opencodeEnabled() int {
	req, _ := http.NewRequest(http.MethodGet, opencodeEnabledBase, nil)
	body, code, err := doRequest(req)
	if err != nil {
		core.Print(core.Stderr(), "lthn opencode enabled: %s\n", err)
		return 1
	}
	if code != http.StatusOK {
		core.Print(core.Stderr(), "lthn opencode enabled: HTTP %d — %s\n", code, body)
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
