// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

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

func TestService_ListMounts_GoodUsesRuntimeMetadata(t *core.T) {
	runtime := &stubRuntimeMetadata{
		snapshot: RuntimeSnapshot{
			Favourites: []Favourite{{MountID: "documents", Path: ""}},
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
	core.AssertEqual(t, 0, len(catalogue.Recent))
}
