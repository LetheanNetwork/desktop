// SPDX-License-Identifier: EUPL-1.2

package files

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

type stubRuntimeMetadata struct {
	snapshot RuntimeSnapshot
	loadErr  error
	saveErr  error
}

func (runtime *stubRuntimeMetadata) Load() (RuntimeSnapshot, error) {
	if runtime.loadErr != nil {
		return RuntimeSnapshot{}, runtime.loadErr
	}
	return normaliseRuntimeSnapshot(runtime.snapshot), nil
}

func (runtime *stubRuntimeMetadata) Save(snapshot RuntimeSnapshot) error {
	if runtime.saveErr != nil {
		return runtime.saveErr
	}
	runtime.snapshot = normaliseRuntimeSnapshot(snapshot)
	return nil
}

func memoryMount(
	id string,
	medium coreio.Medium,
	capabilities Capabilities,
) Mount {
	return Mount{
		ID:                 id,
		Name:               id,
		Kind:               "memory",
		Capabilities:       capabilities,
		Medium:             medium,
		ContainmentAudited: true,
	}
}

func registeredMemoryService(
	t *core.T,
	id string,
	medium coreio.Medium,
	capabilities Capabilities,
) *Service {
	service := NewService(Options{
		Mounts:  []Mount{memoryMount(id, medium, capabilities)},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(core.New()).OK)
	return service
}

func registeredService(
	t *core.T,
	mounts []Mount,
	runtime RuntimeMetadata,
) *Service {
	service := NewService(Options{Mounts: mounts, Runtime: runtime})
	core.RequireTrue(t, service.Register(core.New()).OK)
	return service
}

func TestService_RegisterMounts_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	service := NewService(Options{
		Mounts: []Mount{{
			ID:                 "documents",
			Name:               "Documents",
			Kind:               "memory",
			Capabilities:       ReadWriteCapabilities(),
			Medium:             medium,
			ContainmentAudited: true,
		}},
		Runtime: &stubRuntimeMetadata{},
		Limits:  DefaultLimits(),
	})

	result := service.Register(core.New())

	core.RequireTrue(t, result.OK)
	got, err := service.mount("documents")
	core.AssertNoError(t, err)
	core.AssertSame(t, medium, got.Medium)
}

func TestService_RegisterMounts_BadDuplicateID(t *core.T) {
	service := NewService(Options{
		Mounts: []Mount{
			{
				ID:                 "documents",
				Name:               "A",
				Medium:             coreio.NewMemoryMedium(),
				ContainmentAudited: true,
			},
			{
				ID:                 "documents",
				Name:               "B",
				Medium:             coreio.NewMemoryMedium(),
				ContainmentAudited: true,
			},
		},
		Runtime: &stubRuntimeMetadata{},
	})

	result := service.Register(core.New())

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorInvalidMount))
}

func TestService_RegisterMounts_BadRequiresAuditedMedium(t *core.T) {
	service := NewService(Options{
		Mounts: []Mount{{
			ID:     "documents",
			Name:   "Documents",
			Medium: coreio.NewMemoryMedium(),
		}},
		Runtime: &stubRuntimeMetadata{},
	})

	result := service.Register(core.New())

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorBoundaryRejected))
}

func TestService_RegisterMounts_UglyRequiresRuntime(t *core.T) {
	service := NewService(Options{
		Mounts: []Mount{memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		)},
	})

	result := service.Register(core.New())

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorInvalidInput))
}

func TestNormaliseRelativePath_Ugly(t *core.T) {
	for _, input := range []string{
		"/etc/passwd",
		`C:\Windows\win.ini`,
		"../secret",
		"a/../../secret",
		"a\\b",
		"a/\x00b",
		"a/\u202eb",
		".lthn-files/trash/x",
	} {
		_, err := normaliseRelativePath(input, false)
		core.AssertError(t, err)
		core.AssertContains(t, err.Error(), string(ErrorBoundaryRejected), input)
	}
}

func TestNormaliseRelativePath_RootGood(t *core.T) {
	got, err := normaliseRelativePath("", true)

	core.AssertNoError(t, err)
	core.AssertEqual(t, "", got)
}

func TestValidateEntryName_Good(t *core.T) {
	for _, name := range []string{"Ideas", "résumé 2026.md", ".hidden"} {
		core.AssertNoError(t, validateEntryName(name), name)
	}
}

func TestValidateEntryName_Bad(t *core.T) {
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\u202eb"} {
		err := validateEntryName(name)
		core.AssertError(t, err)
		core.AssertContains(t, err.Error(), "files.", name)
	}
}
