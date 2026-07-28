// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the php service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3
// generate bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/php/service.

package php

import (
	core "dappco.re/go"
	"dappco.re/go/process"
	"dappco.re/go/render/engine/php"
)

const composerSource = "composer"

// DetectOutput is the shape returned to the Angular window — the
// list of Laravel projects discovered, plus the roots walked
// for "why isn't my project here?" debugging.
type DetectOutput struct {
	Roots    []string         `json:"roots"`
	Projects []ProjectSummary `json:"projects"`
	Count    int              `json:"count"`
}

// Detect walks the user's canonical Code/* roots looking for
// Laravel projects. roots overrides the canonical set when
// non-empty.
func (s *Service) Detect(roots []string, maxDepth int) core.Result {
	if len(roots) == 0 {
		roots = defaultRoots()
	}
	projects := s.detect(roots, maxDepth)
	return core.Ok(DetectOutput{
		Roots:    roots,
		Projects: projects,
		Count:    len(projects),
	})
}

// ProjectOutput wraps a single ProjectDetail for the rich
// per-project view.
type ProjectOutput struct {
	Detail ProjectDetail `json:"detail"`
}

// Project returns rich detail for one project — services + .env
// presence + storage perms. Drives the right-pane card on
// /php's selected project.
func (s *Service) Project(path string) core.Result {
	if path == "" {
		return core.Fail(core.E("php.Project", "path is required", nil))
	}
	if !php.IsLaravelProject(path) {
		return core.Fail(core.E("php.Project", "not a Laravel project: "+path, nil))
	}
	services := php.DetectServices(path)
	svcOut := make([]string, 0, len(services))
	for _, sv := range services {
		svcOut = append(svcOut, string(sv))
	}
	return core.Ok(ProjectOutput{
		Detail: ProjectDetail{
			Path:            path,
			Name:            core.PathBase(path),
			AppName:         php.GetLaravelAppName(path),
			AppURL:          php.GetLaravelAppURL(path),
			Domain:          php.ExtractDomainFromURL(php.GetLaravelAppURL(path)),
			PackageMgr:      php.DetectPackageManager(path),
			FrankenPHP:      php.IsFrankenPHPProject(path),
			Services:        svcOut,
			HasEnv:          fileExists(core.PathJoin(path, ".env")),
			HasEnvExample:   fileExists(core.PathJoin(path, ".env.example")),
			HasVendor:       dirExists(core.PathJoin(path, "vendor")),
			HasComposerLock: fileExists(core.PathJoin(path, "composer.lock")),
			HasNodeModules:  dirExists(core.PathJoin(path, "node_modules")),
			HasPackageLock:  fileExists(core.PathJoin(path, "package-lock.json")),
		},
	})
}

// ScriptsOutput is the shape returned to the Angular window — the
// composer.json scripts section + canonical artisan commands.
type ScriptsOutput struct {
	Path            string        `json:"path"`
	ComposerScripts []ScriptEntry `json:"composer_scripts"`
	ArtisanScripts  []ScriptEntry `json:"artisan_scripts"`
	HasArtisan      bool          `json:"has_artisan"`
	HasComposer     bool          `json:"has_composer"`
}

// Scripts reads composer.json's scripts section + emits the
// canonical artisan command set. Gives the UI a clickable grid
// of "things you can run in this Laravel project".
func (s *Service) Scripts(path string) core.Result {
	path = core.Trim(path)
	if path == "" {
		return core.Fail(core.E("php.Scripts", "path required", nil))
	}
	composerPath := core.PathJoin(path, "composer.json")
	hasArtisan := fileExists(core.PathJoin(path, "artisan"))
	return core.Ok(ScriptsOutput{
		Path:            path,
		ComposerScripts: composerScriptEntries(composerPath),
		ArtisanScripts:  artisanScriptEntries(hasArtisan),
		HasArtisan:      hasArtisan,
		HasComposer:     fileExists(composerPath),
	})
}

func composerScriptEntries(composerPath string) []ScriptEntry {
	if read := core.ReadFile(composerPath); read.OK {
		bytes, _ := read.Value.([]byte)
		return parseComposerScripts(bytes)
	}
	return []ScriptEntry{}
}

func parseComposerScripts(bytes []byte) []ScriptEntry {
	var manifest struct {
		Scripts core.RawMessage `json:"scripts"`
	}
	if result := core.JSONUnmarshal(bytes, &manifest); !result.OK || len(manifest.Scripts) == 0 {
		return []ScriptEntry{}
	}
	var parsed map[string]core.RawMessage
	if result := core.JSONUnmarshal(manifest.Scripts, &parsed); !result.OK {
		return []ScriptEntry{}
	}
	return sortedComposerScripts(parsed)
}

func sortedComposerScripts(parsed map[string]core.RawMessage) []ScriptEntry {
	names := make([]string, 0, len(parsed))
	for name := range parsed {
		names = append(names, name)
	}
	core.SliceSort(names)

	entries := []ScriptEntry{}
	for _, name := range names {
		entry := composerScriptEntry(name, parsed[name])
		if entry.Command != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

func composerScriptEntry(name string, raw core.RawMessage) ScriptEntry {
	entry := ScriptEntry{Name: name, Source: composerSource}
	var asString string
	if result := core.JSONUnmarshal(raw, &asString); result.OK {
		entry.Command = asString
		entry.Lines = 1
		return entry
	}
	var asArray []string
	if result := core.JSONUnmarshal(raw, &asArray); result.OK {
		entry.Command = firstComposerCommand(asArray)
		entry.Lines = len(asArray)
	}
	return entry
}

func firstComposerCommand(lines []string) string {
	for _, line := range lines {
		// Composer pseudo-directives are metadata, not runnable shell commands.
		if core.Contains(line, "::") {
			continue
		}
		return line
	}
	if len(lines) > 0 {
		return lines[0]
	}
	return ""
}

func artisanScriptEntries(hasArtisan bool) []ScriptEntry {
	if !hasArtisan {
		return []ScriptEntry{}
	}
	return append([]ScriptEntry{}, canonicalArtisan...)
}

// RunInput drives the Run method.
type RunInput struct {
	Path    string   `json:"path"`
	Mode    string   `json:"mode"` // "composer" | "artisan" | "raw"
	Name    string   `json:"name,omitempty"`
	Args    []string `json:"args,omitempty"`
	Command string   `json:"command,omitempty"`
}

// RunOutput is returned after a successful spawn — the new
// process ID + the resolved command + args so the UI can
// echo what it just kicked off.
type RunOutput struct {
	ID      string   `json:"id"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// Run spawns a composer / artisan / raw invocation via
// process.Service in the project's cwd. Returns the new
// process ID so the UI can subscribe to its output.
func (s *Service) Run(input RunInput) core.Result {
	path := core.Trim(input.Path)
	mode := core.Lower(core.Trim(input.Mode))
	if path == "" || mode == "" {
		return core.Fail(core.E(runOp, "path and mode required", nil))
	}
	var command string
	var args []string
	switch mode {
	case "composer":
		name := core.Trim(input.Name)
		if name == "" {
			return core.Fail(core.E(runOp, "name required for composer mode", nil))
		}
		command = "composer"
		args = []string{"run-script", name}
	case "artisan":
		if len(input.Args) == 0 {
			return core.Fail(core.E(runOp, "args required for artisan mode", nil))
		}
		command = "php"
		args = append([]string{"artisan"}, input.Args...)
	case "raw":
		cmd := core.Trim(input.Command)
		if cmd == "" {
			return core.Fail(core.E(runOp, "command required for raw mode", nil))
		}
		command = "sh"
		args = []string{"-c", cmd}
	default:
		return core.Fail(core.E(runOp, "unknown mode "+mode, nil))
	}
	procR := s.runProc(path, command, args)
	if !procR.OK {
		return procR
	}
	p := procR.Value.(*process.Process)
	return core.Ok(RunOutput{ID: p.ID, Command: command, Args: args})
}
