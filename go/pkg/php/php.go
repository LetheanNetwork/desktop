// SPDX-Licence-Identifier: EUPL-1.2

// Package php is the lthn-side Laravel project discovery + script
// runner surface. Walks the user's canonical Code/* trees to find
// Laravel projects (artisan + composer.json + laravel/framework),
// surfaces per-project detected services (FrankenPHP, Vite,
// Horizon, Reverb, Redis), and dispatches composer/artisan
// invocations to process.Service so the user can stream output.
//
// Ported from core/ide's php_bridge.go + phpbridge.go. Uses
// dappco.re/go/php's detect helpers (IsLaravelProject,
// GetLaravelAppName, GetLaravelAppURL, DetectPackageManager,
// IsFrankenPHPProject, DetectServices, ExtractDomainFromURL) as
// canonical — no re-implementation here.
//
// Caching from core/ide is intentionally dropped — the Laravel
// project walk is bounded (max depth 3, skip vendor/node_modules)
// and cheap on a healthy SSD.

package php

import (
	"context"
	"sort"

	core "dappco.re/go"
	"dappco.re/go/php/pkg/php"
	"dappco.re/go/process"
)

// Service owns the php surface. Holds *core.Core for late
// resolution of the process service (boot order).
type Service struct {
	core *core.Core
}

// NewService constructs the php surface against a Core container.
// Wired via application.NewService(php.NewService(c)) in
// pkg/desktop/desktop.go.
//
// Usage example:
//
//	svc := php.NewService(c)
func NewService(c *core.Core) *Service { return &Service{core: c} }

// Register constructs the php service for Core registration.
//
// Usage example:
//
//	core.New(core.WithService(php.Register))
func Register(c *core.Core) core.Result {
	return core.Ok(NewService(c))
}

// proc resolves the process service at call time. Returns nil
// when the service isn't registered.
func (s *Service) proc() *process.Service {
	if s == nil || s.core == nil {
		return nil
	}
	ps, _ := core.ServiceFor[*process.Service](s.core, "process")
	return ps
}

// ProjectSummary is the lightweight per-project record returned
// by Detect — enough for the project picker without firing N
// detail probes.
type ProjectSummary struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	AppName    string `json:"app_name"`
	AppURL     string `json:"app_url"`
	PackageMgr string `json:"package_mgr"`
	FrankenPHP bool   `json:"frankenphp"`
}

// ProjectDetail is the rich per-project record returned by
// Project — full state + detected services + presence probes.
type ProjectDetail struct {
	Path            string   `json:"path"`
	Name            string   `json:"name"`
	AppName         string   `json:"app_name"`
	AppURL          string   `json:"app_url"`
	Domain          string   `json:"domain"`
	PackageMgr      string   `json:"package_mgr"`
	FrankenPHP      bool     `json:"frankenphp"`
	Services        []string `json:"services"`
	HasEnv          bool     `json:"has_env"`
	HasEnvExample   bool     `json:"has_env_example"`
	HasVendor       bool     `json:"has_vendor"`
	HasComposerLock bool     `json:"has_composer_lock"`
	HasNodeModules  bool     `json:"has_node_modules"`
	HasPackageLock  bool     `json:"has_package_lock"`
}

// ScriptEntry is one runnable item — either a composer.json
// script or a canonical artisan command.
type ScriptEntry struct {
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Lines       int      `json:"lines,omitempty"`
	Source      string   `json:"source"`
	ArtisanArgs []string `json:"artisan_args,omitempty"`
}

// canonicalArtisan is the set of artisan commands surfaced for
// every Laravel project — dev-time + ops-time defaults so the
// user doesn't have to scan composer.json to find them.
var canonicalArtisan = []ScriptEntry{
	{Name: "serve", Command: "php artisan serve", Source: "artisan-canonical", ArtisanArgs: []string{"serve"}},
	{Name: "tinker", Command: "php artisan tinker", Source: "artisan-canonical", ArtisanArgs: []string{"tinker"}},
	{Name: "migrate", Command: "php artisan migrate", Source: "artisan-canonical", ArtisanArgs: []string{"migrate"}},
	{Name: "migrate:fresh", Command: "php artisan migrate:fresh --seed", Source: "artisan-canonical", ArtisanArgs: []string{"migrate:fresh", "--seed"}},
	{Name: "route:list", Command: "php artisan route:list", Source: "artisan-canonical", ArtisanArgs: []string{"route:list"}},
	{Name: "cache:clear", Command: "php artisan cache:clear", Source: "artisan-canonical", ArtisanArgs: []string{"cache:clear"}},
	{Name: "config:cache", Command: "php artisan config:cache", Source: "artisan-canonical", ArtisanArgs: []string{"config:cache"}},
	{Name: "queue:work", Command: "php artisan queue:work", Source: "artisan-canonical", ArtisanArgs: []string{"queue:work"}},
	{Name: "pail", Command: "php artisan pail", Source: "artisan-canonical", ArtisanArgs: []string{"pail"}},
	{Name: "horizon", Command: "php artisan horizon", Source: "artisan-canonical", ArtisanArgs: []string{"horizon"}},
}

// defaultRoots returns the user's canonical Code/* roots —
// Code/lab + Code/core + Code/host-uk under $HOME.
func defaultRoots() []string {
	home := core.UserHomeDir()
	if !home.OK {
		return nil
	}
	homeDir, _ := home.Value.(string)
	if homeDir == "" {
		return nil
	}
	return []string{
		core.PathJoin(homeDir, "Code", "lab"),
		core.PathJoin(homeDir, "Code", "core"),
		core.PathJoin(homeDir, "Code", "host-uk"),
	}
}

// detect walks `roots` looking for Laravel projects. Skips
// vendor/node_modules/.git/build/external to keep the probe
// bounded. Default maxDepth=3 — deep enough to find lab/<site>/
// shaped trees, shallow enough to stay under a second.
func (s *Service) detect(roots []string, maxDepth int) []ProjectSummary {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	out := []ProjectSummary{}
	for _, root := range roots {
		if err := core.PathWalkDir(root, func(path string, d core.FsDirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			rel := "."
			if relResult := core.PathRel(root, path); relResult.OK {
				rel = relResult.Value.(string)
			}
			depth := len(core.Split(rel, string(core.PathSeparator))) - 1
			name := d.Name()
			if name == "node_modules" || name == "vendor" || name == ".git" || name == "build" || name == "external" {
				return core.PathSkipDir
			}
			if depth > maxDepth {
				return core.PathSkipDir
			}
			if !php.IsLaravelProject(path) {
				return nil
			}
			out = append(out, ProjectSummary{
				Path:       path,
				Name:       core.PathBase(path),
				AppName:    php.GetLaravelAppName(path),
				AppURL:     php.GetLaravelAppURL(path),
				PackageMgr: php.DetectPackageManager(path),
				FrankenPHP: php.IsFrankenPHPProject(path),
			})
			return core.PathSkipDir
		}); err != nil {
			core.Warn("php project detection walk failed", "root", root, "err", err)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// fileExists reports whether `path` resolves to a regular file.
func fileExists(path string) bool {
	stat := core.Stat(path)
	if !stat.OK {
		return false
	}
	info, ok := stat.Value.(interface{ IsDir() bool })
	if !ok {
		return false
	}
	return !info.IsDir()
}

// dirExists reports whether `path` resolves to a directory.
func dirExists(path string) bool {
	stat := core.Stat(path)
	if !stat.OK {
		return false
	}
	info, ok := stat.Value.(interface{ IsDir() bool })
	if !ok {
		return false
	}
	return info.IsDir()
}

// runProc invokes process.Service.Start at the project root with
// the named command + args. Returns the spawned Process so the
// caller can grab its ID.
func (s *Service) runProc(cwd, command string, args []string) core.Result {
	ps := s.proc()
	if ps == nil {
		return core.Fail(core.E("php.runProc", "process service unavailable", nil))
	}
	opts := process.RunOptions{
		Command: command,
		Args:    args,
		Dir:     cwd,
	}
	r := ps.StartWithOptions(context.Background(), opts)
	if !r.OK {
		return core.Fail(core.E("php.runProc", r.Error(), nil))
	}
	p, ok := r.Value.(*process.Process)
	if !ok || p == nil {
		return core.Fail(core.E("php.runProc", "process service returned non-process", nil))
	}
	return core.Ok(p)
}
