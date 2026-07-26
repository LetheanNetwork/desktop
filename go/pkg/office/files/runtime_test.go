// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestMediumRuntimeMetadata_RoundTripGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(
		medium,
		"desktop/files/runtime.json",
	)
	snapshot := RuntimeSnapshot{
		Version: 1,
		Favourites: []Favourite{{
			MountID: "documents",
			Path:    "",
		}},
		Recent: []Recent{{
			MountID:  "documents",
			Path:     "notes/readme.md",
			Name:     "readme.md",
			Kind:     EntryFile,
			OpenedAt: "2026-07-26T12:00:00Z",
		}},
		Trash: []TrashReceipt{},
	}

	core.RequireNoError(t, runtime.Save(snapshot))
	got, err := runtime.Load()

	core.AssertNoError(t, err)
	core.AssertEqual(t, snapshot, got)
	core.AssertTrue(t, medium.IsFile("desktop/files/runtime.json"))
}

func TestMediumRuntimeMetadata_BadUnavailableDoesNotReset(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(
		t,
		base.Write("desktop/files/runtime.json", `{"version":1}`),
	)
	runtime := NewMediumRuntimeMetadata(
		&failingMedium{Medium: base, readErr: fs.ErrPermission},
		"desktop/files/runtime.json",
	)

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertTrue(t, base.IsFile("desktop/files/runtime.json"))
}

func TestMediumRuntimeMetadata_BadMalformedDocument(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(
		t,
		medium.Write("runtime.json", `{"version":1,"recent":`),
	)
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	_, err := runtime.Load()

	core.AssertError(t, err)
	core.AssertTrue(t, medium.IsFile("runtime.json"))
}

func TestMediumRuntimeMetadata_UglyRejectsUnknownVersion(t *core.T) {
	medium := coreio.NewMemoryMedium()
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	err := runtime.Save(RuntimeSnapshot{Version: 2})

	core.AssertError(t, err)
	core.AssertFalse(t, medium.IsFile("runtime.json"))
}

func TestMediumRuntimeMetadata_UglyRecoversValidBackup(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write(
		".lthn-files/staging/runtime/older.backup.json",
		`{"version":1,"favourites":[],"recent":[],"trash":[]}`,
	))
	runtime := NewMediumRuntimeMetadata(medium, "runtime.json")

	got, err := runtime.Load()

	core.AssertNoError(t, err)
	core.AssertEqual(t, 1, got.Version)
	core.AssertTrue(t, medium.IsFile("runtime.json"))
}

func TestService_PreviewRecordsRecentGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello"))
	runtime := NewMemoryRuntimeMetadata()
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "readme.md",
	})
	snapshot, err := runtime.Load()

	core.RequireTrue(t, result.OK)
	core.AssertNoError(t, err)
	core.AssertEqual(t, "readme.md", snapshot.Recent[0].Path)
	core.AssertEqual(t, EntryFile, snapshot.Recent[0].Kind)
}

func TestService_PreviewRecordsRecent_BadSaveFailure(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello"))
	runtime := &stubRuntimeMetadata{saveErr: fs.ErrPermission}
	service := registeredService(t, []Mount{
		memoryMount("documents", medium, ReadWriteCapabilities()),
	}, runtime)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "readme.md",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
}
