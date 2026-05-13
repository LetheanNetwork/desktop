// SPDX-Licence-Identifier: EUPL-1.2

// Wails-bindable surface for the plugin host. Generated TS
// lands at frontend/bindings/dappco.re/lthn/desktop/pkg/plugin/service.
//
// marketplace.Service.Install/Remove delegate here; the
// marketplace package keeps its catalogue role + this package
// owns lifecycle. List + Status drive any UI that wants to show
// running/stopped state.

package plugin

import (
	"context"
	"time"

	core "dappco.re/go"
)

// InstallInput drives the Install method. The marketplace flow
// passes (Code, BinaryURL, Checksum) resolved from its catalogue
// entry; LocalPath is the dev-mode side-load that bypasses fetch
// (read directly from disk).
type InstallInput struct {
	Code      string `json:"code"`
	Name      string `json:"name,omitempty"`
	Version   string `json:"version,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	BinaryURL string `json:"binary_url,omitempty"`
	LocalPath string `json:"local_path,omitempty"`
	Checksum  string `json:"checksum,omitempty"`
}

// InstallOutput is the post-install summary — the install dir
// + the resolved Status (running once Install→Start completes).
type InstallOutput struct {
	Code   string `json:"code"`
	Dir    string `json:"dir"`
	Status Status `json:"status"`
}

// Install fetches a plugin binary, writes it to
// ~/Lethean/conf/plugins/<code>/, then starts it. If the plugin
// is already installed the existing copy is overwritten (Stop
// first if running, then redeploy + Start).
func (s *Service) Install(input InstallInput) core.Result {
	m := Manifest{
		Code:      input.Code,
		Name:      input.Name,
		Version:   input.Version,
		Namespace: input.Namespace,
		BinaryURL: input.BinaryURL,
		Checksum:  input.Checksum,
		Binary:    "bin/" + input.Code,
	}
	validatedR := m.validate()
	if !validatedR.OK {
		return validatedR
	}
	validated, _ := validatedR.Value.(Manifest)
	var binary []byte
	if core.Trim(input.LocalPath) != "" {
		read := core.ReadFile(input.LocalPath)
		if !read.OK {
			return core.Fail(core.E(installOp,
				"read local: "+read.Error(), nil))
		}
		binary, _ = read.Value.([]byte)
	} else {
		if core.Trim(validated.BinaryURL) == "" {
			return core.Fail(core.E(installOp,
				"binary_url or local_path required", nil))
		}
		ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
		defer cancel()
		fetchedR := fetchBinary(ctx, validated.BinaryURL)
		if !fetchedR.OK {
			return fetchedR
		}
		binary, _ = fetchedR.Value.([]byte)
	}
	if r := verifyChecksum(binary, validated.Checksum); !r.OK {
		return core.Fail(core.E(installOp,
			"checksum: "+r.Error(), nil))
	}
	s.mu.Lock()
	if r := s.stopPlugin(validated.Code); !r.OK {
		s.mu.Unlock()
		return core.Fail(core.E(installOp, "stop existing plugin: "+r.Error(), nil))
	}
	s.mu.Unlock()
	dirR := writePlugin(validated, binary)
	if !dirR.OK {
		return dirR
	}
	dir, _ := dirR.Value.(string)
	startR := s.Start(validated.Code)
	if !startR.OK {
		return startR
	}
	return core.Ok(InstallOutput{
		Code:   validated.Code,
		Dir:    dir,
		Status: s.snapshotStatus(validated.Code),
	})
}

// Remove stops the plugin (if running) and deletes the install
// directory. After this the plugin is gone — no leftover state.
func (s *Service) Remove(code string) core.Result {
	s.mu.Lock()
	if r := s.stopPlugin(code); !r.OK {
		s.mu.Unlock()
		return core.Fail(core.E("plugin.Remove", "stop plugin: "+r.Error(), nil))
	}
	delete(s.state, code)
	s.mu.Unlock()
	if r := removePlugin(code); !r.OK {
		return core.Fail(core.E("plugin.Remove", r.Error(), nil))
	}
	return core.Ok(nil)
}

// Start spawns the plugin binary. Idempotent — calling Start on
// a running plugin is a no-op.
func (s *Service) Start(code string) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ps, ok := s.state[code]; ok && ps.state == "running" {
		return core.Ok(nil)
	}
	// Generous outer timeout — health gating happens with its
	// own narrower window inside startPlugin. This is just the
	// ceiling for spawn-plus-health.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if r := s.startPlugin(ctx, code, s.resolveToken()); !r.OK {
		return core.Fail(core.E("plugin.Start", r.Error(), nil))
	}
	return core.Ok(nil)
}

// Stop terminates the plugin process + tears down the proxy
// mount. Idempotent — stopping a stopped plugin is a no-op.
func (s *Service) Stop(code string) core.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.stopPlugin(code); !r.OK {
		return core.Fail(core.E("plugin.Stop", r.Error(), nil))
	}
	return core.Ok(nil)
}

// ListOutput wraps the InstalledPlugin slice for a JSON-friendly
// shape consistent with the rest of lthn's Wails surface.
type ListOutput struct {
	Plugins []InstalledPlugin `json:"plugins"`
}

// List enumerates every plugin currently installed on disk,
// running or not. Drives the marketplace "Installed" tab.
func (s *Service) List() core.Result {
	return core.Ok(ListOutput{Plugins: s.scanInstalled()})
}

// Status returns the runtime state for one plugin by code.
// Returns a stopped-stub when the plugin isn't tracked yet.
func (s *Service) Status(code string) core.Result {
	return core.Ok(s.snapshotStatus(code))
}

// snapshotStatus is the locked variant used internally — picks
// up s.mu and returns the projected Status.
func (s *Service) snapshotStatus(code string) Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusFor(code)
}
