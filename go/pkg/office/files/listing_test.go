// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"
	"os"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

// osSymlink is a thin wrapper over os.Symlink — no core.Symlink helper
// exists yet (mirrors the same convention used by pkg/models and
// pkg/downloader's tests).
func osSymlink(target, link string) error { return os.Symlink(target, link) }

func TestService_ListDirectory_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("notes/readme.md", "hello\nworld"))
	core.RequireNoError(t, medium.Write(".hidden", "quiet"))
	core.RequireNoError(t, medium.EnsureDir("empty"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.ListDirectory(ListDirectoryInput{
		MountID: "documents",
		Path:    "",
		Limit:   100,
	})

	core.RequireTrue(t, result.OK)
	snapshot := result.Value.(DirectorySnapshot)
	core.AssertEqual(t, "documents", snapshot.Mount.ID)
	core.AssertEqual(t, "", snapshot.Path)
	core.AssertEqual(t, EntryDirectory, snapshot.Entries[0].Kind)
	core.AssertEqual(t, "empty", snapshot.Entries[0].Name)
	core.AssertEqual(t, EntryDirectory, snapshot.Entries[1].Kind)
	core.AssertEqual(t, "notes", snapshot.Entries[1].RelativePath)
	core.AssertEqual(t, ".hidden", snapshot.Entries[2].Name)
	core.AssertTrue(t, snapshot.Entries[2].Hidden)
	core.AssertEqual(t, 3, snapshot.TotalKnown)
}

func TestService_ListDirectory_BadProviderError(t *core.T) {
	service := registeredMemoryService(
		t,
		"documents",
		&failingMedium{
			Medium:  coreio.NewMemoryMedium(),
			listErr: fs.ErrPermission,
		},
		ReadWriteCapabilities(),
	)

	result := service.ListDirectory(ListDirectoryInput{MountID: "documents"})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
}

func TestService_ListDirectory_UglyPaginatesDeterministically(t *core.T) {
	medium := coreio.NewMemoryMedium()
	for _, name := range []string{"charlie.txt", "alpha.txt", "bravo.txt"} {
		core.RequireNoError(t, medium.Write(name, name))
	}
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	first := service.ListDirectory(ListDirectoryInput{
		MountID: "documents",
		Limit:   2,
	})
	core.RequireTrue(t, first.OK)
	firstPage := first.Value.(DirectorySnapshot)
	core.AssertEqual(t, []string{"alpha.txt", "bravo.txt"}, []string{
		firstPage.Entries[0].Name,
		firstPage.Entries[1].Name,
	})
	core.AssertEqual(t, "2", firstPage.NextCursor)

	second := service.ListDirectory(ListDirectoryInput{
		MountID: "documents",
		Cursor:  firstPage.NextCursor,
		Limit:   2,
	})
	core.RequireTrue(t, second.OK)
	secondPage := second.Value.(DirectorySnapshot)
	core.AssertEqual(t, 1, len(secondPage.Entries))
	core.AssertEqual(t, "charlie.txt", secondPage.Entries[0].Name)
	core.AssertEqual(t, "", secondPage.NextCursor)
}

func TestBreadcrumbs_GoodBuildsCumulativePathsForNestedEntry(t *core.T) {
	got := breadcrumbs("notes/2026/report.md")

	core.RequireTrue(t, len(got) == 3)
	core.AssertEqual(t, "notes", got[0].Name)
	core.AssertEqual(t, "notes", got[0].Path)
	core.AssertEqual(t, "2026", got[1].Name)
	core.AssertEqual(t, "notes/2026", got[1].Path)
	core.AssertEqual(t, "report.md", got[2].Name)
	core.AssertEqual(t, "notes/2026/report.md", got[2].Path)
}

func TestBreadcrumbs_GoodEmptyPathHasNoRows(t *core.T) {
	got := breadcrumbs("")

	core.AssertEqual(t, 0, len(got))
}

func TestListOffset_Bad(t *core.T) {
	for name, cursor := range map[string]string{
		"non-numeric": "abc",
		"negative":    "-1",
	} {
		t.Run(name, func(t *core.T) {
			_, err := listOffset(cursor)

			core.AssertError(t, err)
			core.AssertContains(t, err.Error(), string(ErrorInvalidInput))
		})
	}
}

func TestListOffset_GoodEmptyCursorIsZero(t *core.T) {
	got, err := listOffset("")

	core.AssertNoError(t, err)
	core.AssertEqual(t, 0, got)
}

func TestService_ListLimit_Bad(t *core.T) {
	service := &Service{limits: DefaultLimits()}

	_, err := service.listLimit(-1)

	core.AssertError(t, err)
	core.AssertContains(t, err.Error(), string(ErrorInvalidInput))
}

func TestService_ListLimit_GoodDefaultsAndCaps(t *core.T) {
	service := &Service{limits: Limits{MaxListEntries: 500}}

	defaulted, err := service.listLimit(0)
	core.AssertNoError(t, err)
	core.AssertEqual(t, defaultListLimit, defaulted)

	capped, err := service.listLimit(1000)
	core.AssertNoError(t, err)
	core.AssertEqual(t, 500, capped)
}

func TestService_ListLimit_GoodZeroLimitsFallBackToDefaultLimits(
	t *core.T,
) {
	service := &Service{}

	got, err := service.listLimit(1000000)

	core.AssertNoError(t, err)
	core.AssertEqual(t, DefaultLimits().MaxListEntries, got)
}

func TestService_ListDirectory_BadCursorBeyondRange(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("a.txt", "a"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.ListDirectory(ListDirectoryInput{
		MountID: "documents",
		Cursor:  "50",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorInvalidInput))
}

func TestFileEntry_GoodClassifiesSymlink(t *core.T) {
	root := t.TempDir()
	core.RequireNoError(
		t,
		os.WriteFile(core.PathJoin(root, "target.txt"), []byte("x"), 0600),
	)
	core.RequireNoError(t, osSymlink(
		core.PathJoin(root, "target.txt"),
		core.PathJoin(root, "link.txt"),
	))
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	entries, listErr := medium.List("")
	core.RequireNoError(t, listErr)
	var linkEntry fs.DirEntry
	for _, candidate := range entries {
		if candidate.Name() == "link.txt" {
			linkEntry = candidate
		}
	}
	core.RequireTrue(t, linkEntry != nil)
	info, infoErr := linkEntry.Info()
	core.RequireNoError(t, infoErr)

	entry := fileEntry("", linkEntry, info)

	core.AssertEqual(t, EntryLink, entry.Kind)
	core.AssertEqual(t, "link.txt", entry.RelativePath)
}

func TestService_ListMounts_GoodUsesRuntimeMetadata(t *core.T) {
	runtime := &stubRuntimeMetadata{
		snapshot: RuntimeSnapshot{
			Favourites: []Favourite{{MountID: "documents", Path: ""}},
			Recent: []Recent{{
				MountID:  "documents",
				Path:     "readme.md",
				Name:     "readme.md",
				Kind:     EntryFile,
				OpenedAt: "2026-07-26T12:00:00Z",
			}},
		},
	}
	service := NewService(Options{
		Mounts: []Mount{memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		)},
		Runtime: runtime,
	})
	core.RequireTrue(t, service.Register(core.New()).OK)

	result := service.ListMounts()

	core.RequireTrue(t, result.OK)
	catalogue := result.Value.(MountCatalogue)
	core.AssertEqual(t, 1, len(catalogue.Mounts))
	core.AssertEqual(t, "documents", catalogue.Mounts[0].ID)
	core.AssertEqual(t, 1, len(catalogue.Favourites))
	core.AssertEqual(t, 1, len(catalogue.Recent))
	core.AssertEqual(t, "readme.md", catalogue.Recent[0].Path)
}

func TestService_ListMounts_BadNilServiceOrMissingRuntime(t *core.T) {
	var nilService *Service
	nilResult := nilService.ListMounts()
	core.AssertFalse(t, nilResult.OK)
	core.AssertContains(t, nilResult.Error(), string(ErrorInvalidInput))

	unregistered := &Service{}
	unregisteredResult := unregistered.ListMounts()
	core.AssertFalse(t, unregisteredResult.OK)
	core.AssertContains(
		t,
		unregisteredResult.Error(),
		string(ErrorInvalidInput),
	)
}

func TestService_ListMounts_BadRuntimeLoadFailure(t *core.T) {
	loadErr := core.E("test.inject", "load failed", nil)
	service := NewService(Options{
		Mounts: []Mount{memoryMount(
			"documents",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		)},
		Runtime: &stubRuntimeMetadata{loadErr: loadErr},
	})
	core.RequireTrue(t, service.Register(core.New()).OK)

	result := service.ListMounts()

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
}
