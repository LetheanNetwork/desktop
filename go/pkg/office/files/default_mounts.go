// SPDX-License-Identifier: EUPL-1.2

package files

import (
	goio "io"
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

type localMountSpec struct {
	id       string
	name     string
	icon     string
	brand    bool
	segments []string
}

// DefaultOptions composes runtime state on the registered application I/O
// service and opens only existing, narrow local content roots.
func DefaultOptions(c *core.Core) core.Result {
	if c == nil {
		return core.Fail(defaultOptionsFailure(
			"",
			"Core is unavailable",
			nil,
		))
	}
	ioService, ok := core.ServiceFor[*coreio.Service](c, "io")
	if !ok || ioService == nil || ioService.Medium == nil {
		return core.Fail(defaultOptionsFailure(
			"",
			"registered I/O Medium is unavailable",
			nil,
		))
	}
	homeResult := core.UserHomeDir()
	if !homeResult.OK {
		return core.Fail(defaultOptionsFailure(
			"",
			"user home configuration is unavailable",
			homeResult.Err(),
		))
	}
	home, ok := homeResult.Value.(string)
	if !ok || home == "" {
		return core.Fail(defaultOptionsFailure(
			"",
			"user home configuration is invalid",
			nil,
		))
	}

	specs := []localMountSpec{
		{id: "documents", name: "Documents", icon: "file-lines", segments: []string{"Documents"}},
		{id: "downloads", name: "Downloads", icon: "download", segments: []string{"Downloads"}},
		{id: "projects", name: "Code", icon: "code", segments: []string{"Code"}},
		{
			id:       "models",
			name:     "lthn / models",
			icon:     "cube",
			brand:    true,
			segments: []string{"Lethean", "conf", "models"},
		},
		{id: "recordings", name: "Recordings", icon: "microphone", segments: []string{"Recordings"}},
		{id: "screenshots", name: "Screenshots", icon: "camera", segments: []string{"Screenshots"}},
	}

	mounts := make([]Mount, 0, len(specs))
	for _, spec := range specs {
		rootSegments := append([]string{home}, spec.segments...)
		medium, err := coreio.NewSandboxed(core.PathJoin(rootSegments...))
		if err != nil {
			if core.Is(err, fs.ErrNotExist) {
				continue
			}
			closeComposedMounts(mounts)
			return core.Fail(defaultOptionsFailure(
				spec.id,
				"local mount could not be opened",
				err,
			))
		}
		mounts = append(mounts, Mount{
			ID:                 spec.id,
			Name:               spec.name,
			Kind:               "local",
			LocalRoot:          core.PathJoin(rootSegments...),
			Icon:               spec.icon,
			Brand:              spec.brand,
			Capabilities:       ReadWriteCapabilities(),
			Medium:             medium,
			Owned:              true,
			ContainmentAudited: true,
		})
	}

	return core.Ok(Options{
		Mounts: mounts,
		Runtime: NewMediumRuntimeMetadata(
			ioService.Medium,
			"desktop/files/runtime.json",
		),
		Limits: DefaultLimits(),
		Core:   c,
	})
}

func defaultOptionsFailure(
	mountID string,
	message string,
	cause error,
) *Failure {
	return newFailure(
		ErrorProviderUnavailable,
		mountID,
		"",
		message,
		cause,
	)
}

func closeComposedMounts(mounts []Mount) {
	for _, mount := range mounts {
		if closer, ok := mount.Medium.(goio.Closer); ok {
			if err := closer.Close(); err != nil {
				core.Warn(
					"files.default_options.close",
					"mount",
					mount.ID,
					"error",
					err,
				)
			}
		}
	}
}
