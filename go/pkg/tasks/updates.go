// SPDX-Licence-Identifier: EUPL-1.2

package tasks

import (
	core "dappco.re/go"

	"dappco.re/lthn/desktop/pkg/auth"
)

// package-update detector. Where the lint bridge (detect.go) turns core-lint
// findings into tasks, this turns OUTDATED DEPENDENCIES into tasks: it asks
// each ecosystem's own tooling what is outdated and files one task per
// ecosystem that has any. The task is the work; the lib `package-update` plan
// template (dispatched via --plan-template) is the fix recipe. Same Detect
// contract, a different finding source. Spec:
// plans/code/lthn/desktop/RFC.lint-tasks.md §6.

// updateReporter tags package-update tasks, distinct from lint's reporter so
// the queue (and dedup) can tell maintenance sources apart.
const updateReporter = "package-update"

// ecosystems are the dependency managers Detect understands. A scan runs only
// when its manifest is present in the repo; scan() returns "name: cur → latest"
// lines for the outdated direct dependencies.
var ecosystems = []struct {
	name     string // matches DetectInput.Lang ("go"/"php")
	label    string // human label in the task
	manifest string // presence gate
	scan     func(path string) []string
}{
	{"go", "Go", "go.mod", outdatedGoModules},
	{"php", "PHP", "composer.json", outdatedComposerPackages},
}

// DetectUpdates checks a repo's manifests for outdated dependencies and files a
// package-update task per ecosystem that has any. System-level (the Wails
// Service.DetectUpdates wrapper gates the GUI call). One task per ecosystem
// (not per package) — the task is "these deps are behind", actioned in one go.
//
//	r := tasks.DetectUpdates(c, tasks.DetectInput{Repo: "core/go-io", Path: "/Users/snider/Code/core/go-io"})
//	if r.OK { out := r.Value.(tasks.DetectOutput); _ = out }
func DetectUpdates(c *core.Core, input DetectInput) core.Result {
	if core.Trim(input.Repo) == "" || core.Trim(input.Path) == "" {
		return core.Fail(core.E("tasks.DetectUpdates", "repo and path are required", nil))
	}
	seen := existingFingerprints(c, input.Repo, updateReporter)
	out := DetectOutput{Repo: input.Repo}
	for _, eco := range ecosystems {
		if core.Trim(input.Lang) != "" && input.Lang != eco.name {
			continue // caller narrowed to a single ecosystem
		}
		if !core.Stat(core.PathJoin(input.Path, eco.manifest)).OK {
			continue // ecosystem not present in this repo
		}
		outdated := eco.scan(input.Path)
		if len(outdated) == 0 {
			continue
		}
		out.Findings += len(outdated)
		fp := core.Concat("package-update:", eco.name, "@", input.Repo)
		if seen[fp] {
			out.Skipped++
			continue
		}
		seen[fp] = true
		if Create(c, CreateInput{
			Project:     input.Repo,
			Summary:     core.Concat("[", fp, "] ", core.Itoa(len(outdated)), " ", eco.label, " dependencies outdated"),
			Description: updateDescription(eco.label, outdated),
			Severity:    SeverityMinor,
			Reporter:    updateReporter,
		}).OK {
			out.Created++
		}
	}
	return core.Ok(out)
}

// DetectUpdates (Wails) — gated GUI / scheduler trigger.
func (s *Service) DetectUpdates(input DetectInput) core.Result {
	id, ok := auth.Require(s.core, "tasks.Service.DetectUpdates", auth.TierOperator, auth.TierRenderer)
	if !ok {
		return core.Fail(core.E("tasks.Service.DetectUpdates",
			"tasks.tier_not_permitted: "+id.Tier.String(), nil))
	}
	return DetectUpdates(s.core, input)
}

// DetectUpdates (Wails IPC) — stamp TierRenderer → delegate to the substrate.
func (w *WailsService) DetectUpdates(input DetectInput) core.Result {
	defer stampRenderer(w.svc.core)()
	return w.svc.DetectUpdates(input)
}

// outdatedGoModules lists outdated Go modules via `go -C <path> list -u -m`.
// The -f template emits a line only for modules with an available update, so
// the output is the outdated set (one "path: cur → latest" per line).
func outdatedGoModules(path string) []string {
	out := captureCommand("go", "-C", path, "list", "-u", "-m", "-f",
		"{{if .Update}}{{.Path}}: {{.Version}} → {{.Update.Version}}{{end}}", "all")
	return arrowLines(out)
}

// outdatedComposerPackages lists outdated direct PHP packages via
// `composer outdated --direct --format=json`. Returns "name: cur → latest"
// lines; empty when composer is absent or nothing is outdated.
func outdatedComposerPackages(path string) []string {
	out := captureCommand("composer", "outdated", "--direct", "--format=json",
		"--working-dir="+path, "--no-interaction")
	doc := jsonObjectSlice(out)
	if doc == "" {
		return nil
	}
	var parsed struct {
		Installed []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Latest  string `json:"latest"`
		} `json:"installed"`
	}
	if pr := core.JSONUnmarshalString(doc, &parsed); !pr.OK {
		return nil
	}
	lines := make([]string, 0, len(parsed.Installed))
	for _, p := range parsed.Installed {
		lines = append(lines, core.Concat(p.Name, ": ", p.Version, " → ", p.Latest))
	}
	return lines
}

// arrowLines keeps the non-empty output lines that describe an update (those
// containing the "→" arrow), dropping blanks and tool chatter.
func arrowLines(out string) []string {
	var lines []string
	for _, ln := range core.Split(out, "\n") {
		if t := core.Trim(ln); t != "" && core.Contains(t, "→") {
			lines = append(lines, t)
		}
	}
	return lines
}

// updateDescription is the sealed task body: the count, the fix pointer, and
// the outdated list.
func updateDescription(label string, outdated []string) string {
	b := core.Concat(core.Itoa(len(outdated)), " outdated ", label,
		" dependencies. Dispatch with the Package Update template to bump within constraints, run the tests, and commit.\n")
	for _, line := range outdated {
		b = core.Concat(b, "\n- ", line)
	}
	return b
}
