// SPDX-Licence-Identifier: EUPL-1.2

package services

import (
	core "dappco.re/go"
	coreprocess "dappco.re/go/process"
)

// ProcessHandle is the narrow go-process child lifecycle used by the manager.
type ProcessHandle interface {
	Info() coreprocess.Info
	Output() string
	Wait() core.Result
	Shutdown() core.Result
}

// ProcessRuntime is the narrow named go-process service boundary used by the
// manager. Implementations must return ProcessHandle values.
type ProcessRuntime interface {
	StartWithOptions(core.Context, coreprocess.RunOptions) core.Result
	Get(string) core.Result
}

// WorkingDirectoryResolver resolves a trusted provider-relative reference for
// native process execution.
type WorkingDirectoryResolver interface {
	Resolve(WorkingDirectory) core.Result
}

type namedProcessRuntime struct {
	service *coreprocess.Service
}

func (runtime *namedProcessRuntime) StartWithOptions(
	ctx core.Context,
	options coreprocess.RunOptions,
) core.Result {
	if runtime == nil || runtime.service == nil {
		return core.Fail(core.E(
			"services.ProcessRuntime.StartWithOptions",
			"named process service is unavailable",
			nil,
		))
	}
	return runtime.service.StartWithOptions(ctx, options)
}

func (runtime *namedProcessRuntime) Get(id string) core.Result {
	if runtime == nil || runtime.service == nil {
		return core.Fail(core.E(
			"services.ProcessRuntime.Get",
			"named process service is unavailable",
			nil,
		))
	}
	return runtime.service.Get(id)
}

type emptyWorkingDirectoryResolver struct{}

func (emptyWorkingDirectoryResolver) Resolve(directory WorkingDirectory) core.Result {
	if directory.MountID == "" && directory.Path == "" {
		return core.Ok("")
	}
	return core.Fail(&Failure{
		Code:      ErrorWorkingDirectoryUnsupported,
		Operation: "services.WorkingDirectory.Resolve",
		Message:   "This service working directory is not available for local execution.",
	})
}
