// SPDX-Licence-Identifier: EUPL-1.2

// Wails3 Service shape for the models package — wraps the free
// List function so Wails generates a TS binding at
// frontend/bindings/dappco.re/lthn/desktop/pkg/models/. Bound by
// application.NewService(models.NewWailsService()) in
// pkg/desktop/desktop.go; the package-level List() stays for
// non-WebView callers.

package models

import (
	"context"

	core "dappco.re/go"
	"dappco.re/lthn/desktop/pkg/paths"
)

type WailsService struct{}

func NewWailsService() *WailsService { return &WailsService{} }

func (s *WailsService) ServiceName() string { return "Models" }
func (s *WailsService) ServiceStartup(_ context.Context, _ any) core.Result {
	return core.Ok(nil)
}
func (s *WailsService) ServiceShutdown() core.Result { return core.Ok(nil) }

// List scans the local model snapshot directory and returns one
// Entry per direct child.
func (s *WailsService) List() core.Result {
	r := List()
	if !r.OK {
		return core.Fail(core.E("models.WailsService.List", "list models failed", r.Value.(error)))
	}
	entries, _ := r.Value.([]Entry)
	return core.Ok(entries)
}

// DiskFree returns the free bytes available at the models
// directory. Drives the model-browser footer's "free" slot so the
// number reflects the real disk underneath ~/Lethean/conf/models/.
// Returns 0 when the syscall errors or the platform isn't wired —
// the WebView falls back to its design literal in that case.
//
// Usage example (TS):
//
//	import { DiskFree } from "@desktop/models/wailsservice";
//	const bytes = await DiskFree();
func (s *WailsService) DiskFree() core.Result {
	dirR := paths.ModelsDir()
	if !dirR.OK {
		return core.Ok(int64(0))
	}
	dir, _ := dirR.Value.(string)
	return core.Ok(diskFreeBytes(dir))
}
