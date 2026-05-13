// SPDX-Licence-Identifier: EUPL-1.2

// Wails surface for the php service. Methods are bound on the
// *Service receiver and exposed to the frontend via wails3
// generate bindings — the TS shape lands at
// frontend/bindings/dappco.re/lthn/desktop/pkg/php/service.

package php

import (
	"sort"

	core "dappco.re/go"
	"dappco.re/go/php/pkg/php"
	"dappco.re/go/process"
)

// DetectOutput is the shape returned to the Lit window — the
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

// ScriptsOutput is the shape returned to the Lit window — the
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
	composerScripts := []ScriptEntry{}
	if read := core.ReadFile(composerPath); read.OK {
		var manifest struct {
			Scripts core.RawMessage `json:"scripts"`
		}
		bytes, _ := read.Value.([]byte)
		if result := core.JSONUnmarshal(bytes, &manifest); result.OK && len(manifest.Scripts) > 0 {
			var parsed map[string]core.RawMessage
			if result := core.JSONUnmarshal(manifest.Scripts, &parsed); result.OK {
				names := make([]string, 0, len(parsed))
				for n := range parsed {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, name := range names {
					raw := parsed[name]
					entry := ScriptEntry{Name: name, Source: "composer"}
					var asString string
					if result := core.JSONUnmarshal(raw, &asString); result.OK {
						entry.Command = asString
						entry.Lines = 1
					} else {
						var asArray []string
						if result := core.JSONUnmarshal(raw, &asArray); result.OK {
							entry.Lines = len(asArray)
							for _, line := range asArray {
								// pseudo-directive lines (Composer\\Config::...) skipped
								if core.Contains(line, "::") {
									continue
								}
								entry.Command = line
								break
							}
							if entry.Command == "" && len(asArray) > 0 {
								entry.Command = asArray[0]
							}
						}
					}
					if entry.Command == "" {
						continue
					}
					composerScripts = append(composerScripts, entry)
				}
			}
		}
	}
	hasArtisan := fileExists(core.PathJoin(path, "artisan"))
	canonical := []ScriptEntry{}
	if hasArtisan {
		canonical = append(canonical, canonicalArtisan...)
	}
	return core.Ok(ScriptsOutput{
		Path:            path,
		ComposerScripts: composerScripts,
		ArtisanScripts:  canonical,
		HasArtisan:      hasArtisan,
		HasComposer:     fileExists(composerPath),
	})
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
		return core.Fail(core.E("php.Run", "path and mode required", nil))
	}
	var command string
	var args []string
	switch mode {
	case "composer":
		name := core.Trim(input.Name)
		if name == "" {
			return core.Fail(core.E("php.Run", "name required for composer mode", nil))
		}
		command = "composer"
		args = []string{"run-script", name}
	case "artisan":
		if len(input.Args) == 0 {
			return core.Fail(core.E("php.Run", "args required for artisan mode", nil))
		}
		command = "php"
		args = append([]string{"artisan"}, input.Args...)
	case "raw":
		cmd := core.Trim(input.Command)
		if cmd == "" {
			return core.Fail(core.E("php.Run", "command required for raw mode", nil))
		}
		command = "sh"
		args = []string{"-c", cmd}
	default:
		return core.Fail(core.E("php.Run", "unknown mode "+mode, nil))
	}
	procR := s.runProc(path, command, args)
	if !procR.OK {
		return procR
	}
	p := procR.Value.(*process.Process)
	return core.Ok(RunOutput{ID: p.ID, Command: command, Args: args})
}
