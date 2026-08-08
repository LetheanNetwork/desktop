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

func TestHostItemProviderFailure_GoodMapsCauseToCode(t *core.T) {
	notExist := hostItemProviderFailure(fs.ErrNotExist)
	core.AssertContains(t, notExist.Error(), string(ErrorMissingEntry))

	denied := hostItemProviderFailure(fs.ErrPermission)
	core.AssertContains(t, denied.Error(), string(ErrorCapabilityDenied))

	opaque := hostItemProviderFailure(core.E("test", "boom", nil))
	core.AssertContains(t, opaque.Error(), string(ErrorProviderUnavailable))
}

func TestReadOnlyMedium_WriteOperations_BadAllDeniedFailClosed(t *core.T) {
	root := t.TempDir()
	base, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, base.Write("existing.txt", "content"))
	medium := &readOnlyMedium{Medium: base}

	core.AssertSame(t, fs.ErrPermission, medium.Write("new.txt", "x"))
	core.AssertSame(
		t,
		fs.ErrPermission,
		medium.WriteMode("new.txt", "x", 0600),
	)
	core.AssertSame(t, fs.ErrPermission, medium.EnsureDir("dir"))
	core.AssertSame(t, fs.ErrPermission, medium.Delete("existing.txt"))
	core.AssertSame(t, fs.ErrPermission, medium.DeleteAll("existing.txt"))
	core.AssertSame(
		t,
		fs.ErrPermission,
		medium.Rename("existing.txt", "moved.txt"),
	)
	_, createErr := medium.Create("new.txt")
	core.AssertSame(t, fs.ErrPermission, createErr)
	_, appendErr := medium.Append("existing.txt")
	core.AssertSame(t, fs.ErrPermission, appendErr)
	_, writeStreamErr := medium.WriteStream("new.txt")
	core.AssertSame(t, fs.ErrPermission, writeStreamErr)

	content, readErr := medium.Read("existing.txt")
	core.AssertNoError(t, readErr)
	core.AssertEqual(t, "content", content)
}

func TestReadOnlyMedium_Close_GoodDelegatesAndToleratesNil(t *core.T) {
	base := &closingMedium{Medium: coreio.NewMemoryMedium()}
	medium := &readOnlyMedium{Medium: base}

	core.AssertNoError(t, medium.Close())
	core.AssertEqual(t, 1, base.closeCalls)

	nonCloser := &readOnlyMedium{Medium: coreio.NewMemoryMedium()}
	core.AssertNoError(t, nonCloser.Close())

	var nilMedium *readOnlyMedium
	core.AssertNoError(t, nilMedium.Close())
}

func TestSelectedFileMedium_ScopedToOwnedName_GoodAndBad(t *core.T) {
	root := t.TempDir()
	base, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, base.Write("report.txt", "private"))
	core.RequireNoError(t, base.Write("sibling.txt", "other"))
	medium := newSelectedFileMedium(base, "report.txt")

	core.AssertTrue(t, medium.IsFile("report.txt"))
	core.AssertFalse(t, medium.IsFile("sibling.txt"))
	core.AssertTrue(t, medium.Exists("report.txt"))
	core.AssertFalse(t, medium.Exists("sibling.txt"))
	core.AssertTrue(t, medium.Exists(""))
	core.AssertTrue(t, medium.Exists("."))
	core.AssertTrue(t, medium.IsDir(""))
	core.AssertTrue(t, medium.IsDir("."))
	core.AssertFalse(t, medium.IsDir("report.txt"))

	content, readErr := medium.Read("report.txt")
	core.AssertNoError(t, readErr)
	core.AssertEqual(t, "private", content)
	_, deniedReadErr := medium.Read("sibling.txt")
	core.AssertSame(t, fs.ErrPermission, deniedReadErr)

	file, openErr := medium.Open("report.txt")
	core.AssertNoError(t, openErr)
	core.AssertNoError(t, file.Close())
	_, deniedOpenErr := medium.Open("sibling.txt")
	core.AssertSame(t, fs.ErrPermission, deniedOpenErr)

	stream, streamErr := medium.ReadStream("report.txt")
	core.AssertNoError(t, streamErr)
	core.AssertNoError(t, stream.Close())
	_, deniedStreamErr := medium.ReadStream("sibling.txt")
	core.AssertSame(t, fs.ErrPermission, deniedStreamErr)

	rootInfo, statErr := medium.Stat("")
	core.AssertNoError(t, statErr)
	core.AssertTrue(t, rootInfo.IsDir())
	ownInfo, ownStatErr := medium.Stat("report.txt")
	core.AssertNoError(t, ownStatErr)
	core.AssertFalse(t, ownInfo.IsDir())
	_, deniedStatErr := medium.Stat("sibling.txt")
	core.AssertSame(t, fs.ErrPermission, deniedStatErr)

	entries, listErr := medium.List("")
	core.AssertNoError(t, listErr)
	core.RequireTrue(t, len(entries) == 1)
	core.AssertEqual(t, "report.txt", entries[0].Name())
	_, deniedListErr := medium.List("nested")
	core.AssertSame(t, fs.ErrPermission, deniedListErr)
}

func TestSelectedFileMedium_List_UglyOwnNameAbsentFromParent(t *core.T) {
	root := t.TempDir()
	base, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	medium := newSelectedFileMedium(base, "missing.txt")

	entries, listErr := medium.List(".")

	core.AssertNoError(t, listErr)
	core.AssertEqual(t, 0, len(entries))
}

func TestSelectedFileMedium_Close_GoodDelegatesAndToleratesNil(t *core.T) {
	base := &closingMedium{Medium: coreio.NewMemoryMedium()}
	medium := &selectedFileMedium{Medium: base, name: "report.txt"}

	core.AssertNoError(t, medium.Close())
	core.AssertEqual(t, 1, base.closeCalls)

	nonCloser := &selectedFileMedium{
		Medium: coreio.NewMemoryMedium(),
		name:   "report.txt",
	}
	core.AssertNoError(t, nonCloser.Close())

	var nilMedium *selectedFileMedium
	core.AssertNoError(t, nilMedium.Close())
}

func TestRemoveHostMounts_GoodClosesOnlyListedIDsAndTolerantOfMissing(
	t *core.T,
) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	kept := &closingMedium{Medium: coreio.NewMemoryMedium()}
	removed := &closingMedium{Medium: coreio.NewMemoryMedium()}
	service.hostMu.Lock()
	service.hostMounts["host-kept"] = Mount{
		ID:                 "host-kept",
		Name:               "kept",
		Kind:               "host",
		Medium:             kept,
		ContainmentAudited: true,
	}
	service.hostMounts["host-removed"] = Mount{
		ID:                 "host-removed",
		Name:               "removed",
		Kind:               "host",
		Medium:             removed,
		ContainmentAudited: true,
	}
	service.hostOrder = []string{"host-kept", "host-removed"}
	service.hostMu.Unlock()

	service.removeHostMounts([]HostItemView{
		{MountID: "host-removed"},
		{MountID: "host-unknown"},
	})

	core.AssertEqual(t, 1, removed.closeCalls)
	core.AssertEqual(t, 0, kept.closeCalls)
	core.AssertEqual(
		t,
		[]string{"host-kept"},
		service.hostOrder,
	)
	core.AssertEqual(t, 1, len(service.hostMountSnapshot()))

	var nilService *Service
	nilService.removeHostMounts([]HostItemView{{MountID: "x"}})
	service.removeHostMounts(nil)
	core.AssertEqual(t, 1, len(service.hostMountSnapshot()))
}

func TestResolveHostItems_BadNilFactoryFailsClosed(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	service.hostMediumFactory = nil

	result := ResolveHostItems(service, []string{
		core.PathJoin(t.TempDir(), "unreachable.txt"),
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
}

func TestResolveHostItems_GoodDeepestLocalMountWins(t *core.T) {
	root := t.TempDir()
	outer, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, outer.EnsureDir("projects"))
	core.RequireNoError(t, outer.Write("projects/readme.md", "hello"))
	inner, err := coreio.NewSandboxed(core.PathJoin(root, "projects"))
	core.RequireNoError(t, err)
	service := registeredService(t, []Mount{
		{
			ID:                 "home",
			Name:               "Home",
			Kind:               "local",
			LocalRoot:          root,
			Capabilities:       ReadWriteCapabilities(),
			Medium:             outer,
			Owned:              true,
			ContainmentAudited: true,
		},
		{
			ID:                 "projects",
			Name:               "Projects",
			Kind:               "local",
			LocalRoot:          core.PathJoin(root, "projects"),
			Capabilities:       ReadWriteCapabilities(),
			Medium:             inner,
			Owned:              true,
			ContainmentAudited: true,
		},
	}, &stubRuntimeMetadata{})

	result := ResolveHostItems(service, []string{
		core.PathJoin(root, "projects", "readme.md"),
	})

	core.RequireTrue(t, result.OK, result.Error())
	items := result.Value.([]HostItemView)
	core.RequireTrue(t, len(items) == 1)
	core.AssertEqual(t, "projects", items[0].MountID)
	core.AssertEqual(t, "readme.md", items[0].Path)
}

func TestResolveHostItems_BadRejectsLocalRootItself(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
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

	// Selecting an authorised mount's root itself is not a nameable
	// host item — the view construction rejects the empty relative
	// path rather than aliasing the whole mount.
	result := ResolveHostItems(service, []string{root})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorInvalidInput))
}

func TestResolveHostItems_BadMissingLocalFile(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
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
		core.PathJoin(root, "ghost.md"),
	})

	core.AssertFalse(t, result.OK)
}

func TestResolveHostItems_GoodOpensSelectedDirectory(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	root := t.TempDir()
	source, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, source.EnsureDir("bundle"))
	core.RequireNoError(t, source.Write("bundle/asset.txt", "content"))

	result := ResolveHostItems(service, []string{
		core.PathJoin(root, "bundle"),
	})

	core.RequireTrue(t, result.OK, result.Error())
	items := result.Value.([]HostItemView)
	core.RequireTrue(t, len(items) == 1)
	core.AssertTrue(t, core.HasPrefix(items[0].MountID, "host-"))
	core.AssertEqual(t, EntryDirectory, items[0].Kind)
	core.AssertEqual(t, "", items[0].Path)
}

func TestResolveHostItems_UglyDirectoryReopenFails(t *core.T) {
	service := registeredService(t, nil, &stubRuntimeMetadata{})
	root := t.TempDir()
	source, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, source.EnsureDir("bundle"))
	target := core.PathJoin(root, "bundle")
	// The parent open succeeds so the Stat + kind checks run; the
	// second factory call — reopening the selected directory as its
	// own medium — fails, which must fail the whole selection closed.
	service.hostMediumFactory = func(path string) (coreio.Medium, error) {
		if path == target {
			return nil, fs.ErrPermission
		}
		return coreio.NewSandboxed(path)
	}

	result := ResolveHostItems(service, []string{target})

	core.AssertFalse(t, result.OK)
	core.AssertEqual(t, 0, len(service.hostMountSnapshot()))
}
