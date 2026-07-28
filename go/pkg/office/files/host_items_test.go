// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestResolveHostItems_GoodReusesAuthorisedLocalMount(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, medium.EnsureDir("notes"))
	core.RequireNoError(t, medium.Write("notes/readme.md", "hello"))
	service := registeredService(t, []Mount{{
		ID:                 "documents",
		Name:               "Documents",
		Kind:               "local",
		LocalRoot:          root,
		Capabilities:       ReadWriteCapabilities(),
		Medium:             medium,
		Owned:              true,
		ContainmentAudited: true,
	}}, &stubRuntimeMetadata{})

	result := ResolveHostItems(service, []string{
		core.PathJoin(root, "notes", "readme.md"),
	})

	core.RequireTrue(t, result.OK, result.Error())
	items, ok := result.Value.([]HostItemView)
	core.RequireTrue(t, ok)
	core.RequireTrue(t, len(items) == 1)
	core.AssertEqual(t, "documents", items[0].MountID)
	core.AssertEqual(t, "notes/readme.md", items[0].Path)
	core.AssertEqual(t, "readme.md", items[0].Name)
	core.AssertEqual(t, EntryFile, items[0].Kind)
	core.AssertEqual(t, "text/markdown", items[0].MediaType)
}

func TestResolveHostItems_GoodCreatesOpaqueReadOnlySessionMount(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	root := t.TempDir()
	source, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, source.Write("report.txt", "private"))

	result := ResolveHostItems(service, []string{
		core.PathJoin(root, "report.txt"),
	})

	core.RequireTrue(t, result.OK, result.Error())
	items := result.Value.([]HostItemView)
	core.RequireTrue(t, len(items) == 1)
	item := items[0]
	core.AssertTrue(t, core.HasPrefix(item.MountID, "host-"))
	core.AssertNotEqual(t, root, item.MountID)
	core.AssertEqual(t, "report.txt", item.Path)
	mount, mountErr := service.mount(item.MountID)
	core.RequireNoError(t, mountErr)
	core.AssertTrue(t, mount.Capabilities.List)
	core.AssertTrue(t, mount.Capabilities.Preview)
	core.AssertTrue(t, mount.Capabilities.Open)
	core.AssertTrue(t, mount.Capabilities.Reveal)
	core.AssertFalse(t, mount.Capabilities.Write)
	core.AssertFalse(t, mount.Capabilities.CopyFrom)
	core.AssertEqual(t, root, mount.LocalRoot)
	content, readErr := mount.Medium.Read(item.Path)
	core.RequireNoError(t, readErr)
	core.AssertEqual(t, "private", content)
	core.AssertError(t, mount.Medium.Write(item.Path, "changed"))
	entries, listErr := mount.Medium.List("")
	core.RequireNoError(t, listErr)
	core.RequireTrue(t, len(entries) == 1)
	core.AssertEqual(t, "report.txt", entries[0].Name())

	catalogue := service.ListMounts()
	core.RequireTrue(t, catalogue.OK)
	mounts := catalogue.Value.(MountCatalogue).Mounts
	core.RequireTrue(t, len(mounts) == 1)
	core.AssertEqual(t, item.MountID, mounts[0].ID)
	core.AssertEqual(t, "report.txt", mounts[0].Name)
}

func TestResolveHostItems_GoodSelectedDirectoryOwnsOnlyItsTree(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	root := t.TempDir()
	source, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, source.EnsureDir("selected"))
	core.RequireNoError(t, source.Write("selected/inside.txt", "inside"))
	core.RequireNoError(t, source.Write("sibling.txt", "outside"))

	result := ResolveHostItems(service, []string{
		core.PathJoin(root, "selected"),
	})

	core.RequireTrue(t, result.OK, result.Error())
	item := result.Value.([]HostItemView)[0]
	core.AssertEqual(t, "", item.Path)
	core.AssertEqual(t, EntryDirectory, item.Kind)
	mount, mountErr := service.mount(item.MountID)
	core.RequireNoError(t, mountErr)
	core.AssertEqual(t, core.PathJoin(root, "selected"), mount.LocalRoot)
	core.AssertTrue(t, mount.Capabilities.Open)
	core.AssertTrue(t, mount.Capabilities.Reveal)
	core.AssertTrue(t, mount.Medium.IsFile("inside.txt"))
	core.AssertFalse(t, mount.Medium.Exists("../sibling.txt"))
}

func TestResolveHostItems_BadRejectsBatchBeforeCreatingMounts(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	root := t.TempDir()
	source, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, source.Write("first.txt", "one"))
	first := core.PathJoin(root, "first.txt")

	for name, paths := range map[string][]string{
		"empty":     {},
		"relative":  {"../secret.txt"},
		"duplicate": {first, first},
		"missing":   {first, core.PathJoin(root, "missing.txt")},
	} {
		t.Run(name, func(t *core.T) {
			result := ResolveHostItems(service, paths)
			core.AssertFalse(t, result.OK)
			expected := ErrorInvalidInput
			if name == "missing" {
				expected = ErrorMissingEntry
			}
			core.AssertContains(t, result.Error(), string(expected))
			core.AssertEqual(t, 0, len(service.hostMountSnapshot()))
		})
	}
}

func TestResolveHostItems_BadRejectsOversizedDrop(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	paths := make([]string, maxHostItems+1)
	for index := range paths {
		paths[index] = core.Concat("/tmp/item-", core.Itoa(index))
	}

	result := ResolveHostItems(service, paths)

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorLimitExceeded))
}

func TestResolveHostItems_BadDoesNotTreatRemoteProviderAsHostRoot(t *core.T) {
	root := t.TempDir()
	service := NewService(Options{
		Mounts: []Mount{{
			ID:                 "bucket",
			Name:               "Bucket",
			Kind:               "s3",
			LocalRoot:          root,
			Capabilities:       ReadWriteCapabilities(),
			Medium:             coreio.NewMemoryMedium(),
			ContainmentAudited: true,
		}},
		Runtime: &stubRuntimeMetadata{},
	})

	result := service.Register(core.New())

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorBoundaryRejected))
}

func TestResolveHostItems_UglyUnavailableMediumFailsClosed(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	service.hostMediumFactory = func(string) (coreio.Medium, error) {
		return nil, fs.ErrPermission
	}

	result := ResolveHostItems(service, []string{
		core.PathJoin(t.TempDir(), "blocked.txt"),
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
	core.AssertEqual(t, 0, len(service.hostMountSnapshot()))
}

func TestResolveHostItems_UglyCapsSessionMounts(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	service.hostMu.Lock()
	for index := 0; index < maxHostMounts; index++ {
		id := core.Concat("host-existing-", core.Itoa(index))
		service.hostMounts[id] = Mount{
			ID:                 id,
			Name:               id,
			Kind:               "host",
			Medium:             coreio.NewMemoryMedium(),
			ContainmentAudited: true,
		}
		service.hostOrder = append(service.hostOrder, id)
	}
	service.hostMu.Unlock()
	root := t.TempDir()
	source, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, source.Write("next.txt", "next"))

	result := ResolveHostItems(service, []string{
		core.PathJoin(root, "next.txt"),
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorLimitExceeded))
	core.AssertEqual(t, maxHostMounts, len(service.hostMountSnapshot()))
}

func TestResolveHostItems_UglyNilServiceFailsClosed(t *core.T) {
	result := ResolveHostItems(nil, []string{"/tmp/report.txt"})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
}
