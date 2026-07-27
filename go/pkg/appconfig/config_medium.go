// SPDX-License-Identifier: EUPL-1.2

package appconfig

import (
	"io/fs"
	"sync"

	core "dappco.re/go"
	"dappco.re/go/config"
	coreio "dappco.re/go/io"
)

const configMediumServiceName = "config-io"

// ConfigServiceOptions configures the CoreGO config service on a registered
// Medium. Path is provider-relative to the trusted config Medium.
type ConfigServiceOptions struct {
	Path      string
	EnvPrefix string
}

// NewConfigService resolves the named configuration Medium and constructs the
// CoreGO config service over an atomic commit adapter.
//
// Usage example:
//
//	core.WithName("config", appconfig.NewConfigService(
//	    appconfig.ConfigServiceOptions{Path: "lthn.yaml", EnvPrefix: "LTHN"},
//	))
func NewConfigService(
	options ConfigServiceOptions,
) func(*core.Core) core.Result {
	return func(c *core.Core) core.Result {
		if c == nil {
			return core.Fail(core.E(
				"appconfig.NewConfigService",
				"Core is required",
				nil,
			))
		}
		ioService, ok := core.ServiceFor[*coreio.Service](
			c,
			configMediumServiceName,
		)
		if !ok || ioService == nil || ioService.Medium == nil {
			return core.Fail(core.E(
				"appconfig.NewConfigService",
				"registered configuration Medium is required",
				nil,
			))
		}
		if !validConfigPath(options.Path) {
			return core.Fail(core.E(
				"appconfig.NewConfigService",
				"provider-relative configuration path is invalid",
				nil,
			))
		}
		factory := config.NewConfigServiceWith(config.ServiceOptions{
			Path:      options.Path,
			EnvPrefix: options.EnvPrefix,
			Medium: newAtomicConfigMedium(
				ioService.Medium,
				options.Path,
			),
		})
		return factory(c)
	}
}

type atomicConfigMedium struct {
	coreio.Medium

	path string
	mu   sync.Mutex
}

func newAtomicConfigMedium(
	medium coreio.Medium,
	path string,
) *atomicConfigMedium {
	return &atomicConfigMedium{Medium: medium, path: path}
}

// WriteMode atomically commits the configured document. Other paths retain
// the wrapped Medium's ordinary semantics.
func (medium *atomicConfigMedium) WriteMode(
	path string,
	content string,
	mode fs.FileMode,
) error {
	if medium == nil || medium.Medium == nil {
		return core.E(
			"appconfig.atomicConfigMedium.WriteMode",
			"configuration Medium is unavailable",
			nil,
		)
	}
	if path != medium.path {
		return medium.Medium.WriteMode(path, content, mode)
	}

	medium.mu.Lock()
	defer medium.mu.Unlock()

	stagingDir := ".lthn-config"
	stagedPath := core.JoinPath(
		stagingDir,
		core.Concat(core.PathBase(path), ".new"),
	)
	backupPath := core.JoinPath(
		stagingDir,
		core.Concat(core.PathBase(path), ".backup"),
	)
	if err := medium.Medium.EnsureDir(stagingDir); err != nil {
		return core.E(
			"appconfig.atomicConfigMedium.WriteMode",
			"create configuration staging directory",
			err,
		)
	}
	if medium.Medium.IsFile(stagedPath) {
		if err := medium.Medium.Delete(stagedPath); err != nil {
			return core.E(
				"appconfig.atomicConfigMedium.WriteMode",
				"remove stale configuration staging document",
				err,
			)
		}
	}
	if err := medium.Medium.WriteMode(stagedPath, content, mode); err != nil {
		return core.E(
			"appconfig.atomicConfigMedium.WriteMode",
			"write configuration staging document",
			err,
		)
	}
	readBack, err := medium.Medium.Read(stagedPath)
	if err != nil || readBack != content {
		_ = medium.Medium.Delete(stagedPath)
		if err == nil {
			err = core.E(
				"appconfig.atomicConfigMedium.WriteMode",
				"configuration staging read-back differs",
				nil,
			)
		}
		return err
	}

	hadDocument := medium.Medium.IsFile(path)
	if hadDocument && medium.Medium.IsFile(backupPath) {
		if err := medium.Medium.Delete(backupPath); err != nil {
			_ = medium.Medium.Delete(stagedPath)
			return core.E(
				"appconfig.atomicConfigMedium.WriteMode",
				"remove stale configuration backup",
				err,
			)
		}
	}
	if hadDocument {
		if err := medium.Medium.Rename(path, backupPath); err != nil {
			_ = medium.Medium.Delete(stagedPath)
			return core.E(
				"appconfig.atomicConfigMedium.WriteMode",
				"stage previous configuration document",
				err,
			)
		}
	}
	if err := medium.Medium.Rename(stagedPath, path); err != nil {
		var recoveryErr error
		if hadDocument {
			recoveryErr = medium.Medium.Rename(backupPath, path)
		}
		_ = medium.Medium.Delete(stagedPath)
		if recoveryErr != nil {
			return core.E(
				"appconfig.atomicConfigMedium.WriteMode",
				"configuration commit and recovery failed",
				recoveryErr,
			)
		}
		return core.E(
			"appconfig.atomicConfigMedium.WriteMode",
			"commit configuration document",
			err,
		)
	}
	if hadDocument {
		if err := medium.Medium.Delete(backupPath); err != nil {
			return core.E(
				"appconfig.atomicConfigMedium.WriteMode",
				"remove configuration backup",
				err,
			)
		}
	}
	return nil
}

// BaseConfigMedium returns the registered provider beneath the atomic adapter.
// It exists for trusted composition and tests; it is not renderer-bound.
func BaseConfigMedium(service *config.Service) coreio.Medium {
	if service == nil || service.Config() == nil {
		return nil
	}
	medium := service.Config().Medium()
	if atomic, ok := medium.(*atomicConfigMedium); ok {
		return atomic.Medium
	}
	return medium
}

func validConfigPath(path string) bool {
	if path == "" || core.PathIsAbs(path) || core.Contains(path, `\`) {
		return false
	}
	clean := core.CleanPath(path, "/")
	return clean == path &&
		clean != "." &&
		clean != ".." &&
		!core.HasPrefix(clean, "../")
}
