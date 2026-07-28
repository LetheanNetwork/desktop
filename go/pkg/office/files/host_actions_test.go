// SPDX-License-Identifier: EUPL-1.2

package files

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"dappco.re/go/render/display/webkit/pkg/environment"
)

func TestService_HostActions_GoodUseAuditedMediumBeforeDispatch(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, medium.EnsureDir("notes"))
	core.RequireNoError(t, medium.Write("notes/readme.md", "hello"))
	capabilities := ReadWriteCapabilities()
	capabilities.Open = true
	capabilities.Reveal = true
	c := core.New()
	service := NewService(Options{
		Core: c,
		Mounts: []Mount{{
			ID:                 "documents",
			Name:               "Documents",
			Kind:               "local",
			LocalRoot:          root,
			Capabilities:       capabilities,
			Medium:             medium,
			Owned:              true,
			ContainmentAudited: true,
		}},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)

	var openedPath string
	var revealTask environment.TaskOpenFileManager
	c.Action("browser.open_file", func(_ core.Context, options core.Options) core.Result {
		openedPath = options.String(core.Concat("pa", "th"))
		return core.Ok(nil)
	})
	c.Action(
		"environment.open_file_manager",
		func(_ core.Context, options core.Options) core.Result {
			task := options.Get("task")
			core.RequireTrue(t, task.OK)
			revealTask = task.Value.(environment.TaskOpenFileManager)
			return core.Ok(nil)
		},
	)

	openResult := service.Open(FileAddress{
		MountID: "documents",
		Path:    "notes/readme.md",
	})
	revealResult := service.Reveal(FileAddress{
		MountID: "documents",
		Path:    "notes/readme.md",
	})

	core.RequireTrue(t, openResult.OK, openResult.Error())
	core.RequireTrue(t, revealResult.OK, revealResult.Error())
	expected := core.PathJoin(root, "notes", "readme.md")
	core.AssertEqual(t, expected, openedPath)
	core.AssertEqual(t, expected, revealTask.Path)
	core.AssertTrue(t, revealTask.Select)
}

func TestService_HostActions_GoodAllowAuditedLocalRoot(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	capabilities := Capabilities{Open: true, Reveal: true}
	c := core.New()
	service := NewService(Options{
		Core: c,
		Mounts: []Mount{{
			ID:                 "documents",
			Name:               "Documents",
			Kind:               "local",
			LocalRoot:          root,
			Capabilities:       capabilities,
			Medium:             medium,
			Owned:              true,
			ContainmentAudited: true,
		}},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)

	var opened string
	c.Action("browser.open_file", func(_ core.Context, options core.Options) core.Result {
		opened = options.String(core.Concat("pa", "th"))
		return core.Ok(nil)
	})

	result := service.Open(FileAddress{MountID: "documents", Path: ""})

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, root, opened)
}

func TestService_HostActions_GoodPreserveSelectedHostLeastAuthority(t *core.T) {
	root := t.TempDir()
	source, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, source.Write("selected.txt", "selected"))
	core.RequireNoError(t, source.Write("sibling.txt", "private"))
	c := core.New()
	service := NewService(Options{
		Core:    c,
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)
	var opened []string
	c.Action("browser.open_file", func(_ core.Context, options core.Options) core.Result {
		opened = append(opened, options.String(core.Concat("pa", "th")))
		return core.Ok(nil)
	})

	resolved := ResolveHostItems(service, []string{
		core.PathJoin(root, "selected.txt"),
	})
	core.RequireTrue(t, resolved.OK, resolved.Error())
	item := resolved.Value.([]HostItemView)[0]

	openResult := service.Open(FileAddress{
		MountID: item.MountID,
		Path:    item.Path,
	})
	siblingResult := service.Open(FileAddress{
		MountID: item.MountID,
		Path:    "sibling.txt",
	})

	core.RequireTrue(t, openResult.OK, openResult.Error())
	core.AssertFalse(t, siblingResult.OK)
	core.AssertContains(t, siblingResult.Error(), string(ErrorCapabilityDenied))
	core.AssertEqual(t, []string{core.PathJoin(root, "selected.txt")}, opened)
}

func TestService_HostActions_BadRejectBeforeDispatch(t *core.T) {
	root := t.TempDir()
	localMedium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, localMedium.Write("present.txt", "hello"))
	localCapabilities := Capabilities{Open: true, Reveal: true}
	c := core.New()
	service := NewService(Options{
		Core: c,
		Mounts: []Mount{
			{
				ID:                 "documents",
				Name:               "Documents",
				Kind:               "local",
				LocalRoot:          root,
				Capabilities:       localCapabilities,
				Medium:             localMedium,
				Owned:              true,
				ContainmentAudited: true,
			},
			{
				ID:                 "bucket",
				Name:               "Bucket",
				Kind:               "s3",
				Capabilities:       localCapabilities,
				Medium:             coreio.NewMemoryMedium(),
				ContainmentAudited: true,
			},
			{
				ID:                 "read-only",
				Name:               "Read only",
				Kind:               "local",
				LocalRoot:          root,
				Medium:             localMedium,
				ContainmentAudited: true,
			},
		},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)
	dispatches := 0
	c.Action("browser.open_file", func(_ core.Context, _ core.Options) core.Result {
		dispatches++
		return core.Ok(nil)
	})
	c.Action(
		"environment.open_file_manager",
		func(_ core.Context, _ core.Options) core.Result {
			dispatches++
			return core.Ok(nil)
		},
	)

	tests := []struct {
		name   string
		action func() core.Result
		code   ErrorCode
	}{
		{
			name: "remote provider",
			action: func() core.Result {
				return service.Open(FileAddress{MountID: "bucket", Path: "report.txt"})
			},
			code: ErrorBoundaryRejected,
		},
		{
			name: "missing open capability",
			action: func() core.Result {
				return service.Open(FileAddress{MountID: "read-only", Path: "present.txt"})
			},
			code: ErrorCapabilityDenied,
		},
		{
			name: "missing reveal capability",
			action: func() core.Result {
				return service.Reveal(FileAddress{MountID: "read-only", Path: "present.txt"})
			},
			code: ErrorCapabilityDenied,
		},
		{
			name: "path traversal",
			action: func() core.Result {
				return service.Open(FileAddress{MountID: "documents", Path: "../secret.txt"})
			},
			code: ErrorBoundaryRejected,
		},
		{
			name: "missing entry",
			action: func() core.Result {
				return service.Reveal(FileAddress{MountID: "documents", Path: "missing.txt"})
			},
			code: ErrorMissingEntry,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *core.T) {
			result := test.action()
			core.AssertFalse(t, result.OK)
			core.AssertContains(t, result.Error(), string(test.code))
		})
	}
	core.AssertEqual(t, 0, dispatches)
}

func TestService_HostActions_UglyUnavailableCoreOrActionFailsClosed(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, medium.Write("report.txt", "hello"))
	capabilities := Capabilities{Open: true, Reveal: true}

	for name, c := range map[string]*core.Core{
		"nil core":       nil,
		"missing action": core.New(),
	} {
		t.Run(name, func(t *core.T) {
			service := NewService(Options{
				Core: c,
				Mounts: []Mount{{
					ID:                 "documents",
					Name:               "Documents",
					Kind:               "local",
					LocalRoot:          root,
					Capabilities:       capabilities,
					Medium:             medium,
					ContainmentAudited: true,
				}},
				Runtime: &stubRuntimeMetadata{},
			})
			core.RequireTrue(t, service.Register(c).OK)

			result := service.Open(FileAddress{
				MountID: "documents",
				Path:    "report.txt",
			})

			core.AssertFalse(t, result.OK)
			core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
		})
	}
}
