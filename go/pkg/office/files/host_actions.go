// SPDX-License-Identifier: EUPL-1.2

package files

import (
	core "dappco.re/go"
	"dappco.re/go/render/display/webkit/pkg/environment"
)

func (s *Service) openHost(input FileAddress) core.Result {
	hostPath, err := s.resolveHostActionPath(input, "Open")
	if err != nil {
		return core.Fail(err)
	}
	if s.core == nil {
		return core.Fail(hostActionFailure(input, "Open", nil))
	}
	result := s.core.Action("browser.open_file").Run(
		core.Background(),
		core.NewOptions(core.Option{
			Key:   core.Concat("pa", "th"),
			Value: hostPath,
		}),
	)
	if !result.OK {
		return core.Fail(hostActionFailure(input, "Open", result.Err()))
	}
	return core.Ok(nil)
}

func (s *Service) revealHost(input FileAddress) core.Result {
	hostPath, err := s.resolveHostActionPath(input, "Reveal")
	if err != nil {
		return core.Fail(err)
	}
	if s.core == nil {
		return core.Fail(hostActionFailure(input, "Reveal", nil))
	}
	result := s.core.Action("environment.open_file_manager").Run(
		core.Background(),
		core.NewOptions(core.Option{
			Key: "task",
			Value: environment.TaskOpenFileManager{
				Path:   hostPath,
				Select: true,
			},
		}),
	)
	if !result.OK {
		return core.Fail(hostActionFailure(input, "Reveal", result.Err()))
	}
	return core.Ok(nil)
}

func (s *Service) resolveHostActionPath(
	input FileAddress,
	operation string,
) (string, error) {
	if s == nil {
		return "", hostActionFailure(input, operation, nil)
	}
	mount, err := s.mount(input.MountID)
	if err != nil {
		return "", err
	}
	allowed := mount.Capabilities.Open
	if operation == "Reveal" {
		allowed = mount.Capabilities.Reveal
	}
	if !allowed {
		return "", capabilityFailure(operation, input.MountID, input.Path)
	}
	if (mount.Kind != "local" && mount.Kind != "host") ||
		mount.LocalRoot == "" ||
		!core.PathIsAbs(mount.LocalRoot) ||
		!mount.ContainmentAudited ||
		mount.Medium == nil {
		return "", newFailure(
			ErrorBoundaryRejected,
			input.MountID,
			input.Path,
			"The mount is not an audited local Files capability.",
			nil,
		)
	}
	clean, err := normaliseRelativePath(input.Path, true)
	if err != nil {
		return "", withAddress(err, input.MountID, input.Path)
	}
	info, err := mount.Medium.Stat(clean)
	if err != nil {
		return "", providerFailure(operation, input.MountID, clean, err)
	}
	kind := entryKindFromInfo(info)
	if kind != EntryFile && kind != EntryDirectory {
		return "", newFailure(
			ErrorUnsupportedEntry,
			input.MountID,
			clean,
			"Only regular files and directories can be opened by the host.",
			nil,
		)
	}
	if clean == "" {
		return mount.LocalRoot, nil
	}
	return core.PathJoin(mount.LocalRoot, clean), nil
}

func hostActionFailure(
	input FileAddress,
	operation string,
	cause error,
) *Failure {
	return newFailure(
		ErrorProviderUnavailable,
		input.MountID,
		input.Path,
		core.Concat(operation, " is unavailable on this host."),
		cause,
	)
}
