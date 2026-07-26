// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestService_CreateDirectory_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir("notes"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.CreateDirectory(CreateDirectoryInput{
		MountID:    "documents",
		ParentPath: "notes",
		Name:       "Ideas",
	})

	core.RequireTrue(t, result.OK)
	_, err := medium.Stat("notes/Ideas")
	core.AssertNoError(t, err)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationCompleted, operation.Status)
	core.AssertEqual(t, "notes/Ideas", operation.Affected[0].Path)
}

func TestService_CreateDirectory_BadConflict(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("Ideas", "already a file"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.CreateDirectory(CreateDirectoryInput{
		MountID: "documents",
		Name:    "Ideas",
	})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationConflict, operation.Status)
	core.AssertEqual(t, ErrorConflict, operation.Code)
	core.AssertNotNil(t, operation.Conflict)
}

func TestService_CreateDirectory_BadCapability(t *core.T) {
	medium := coreio.NewMemoryMedium()
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		Capabilities{List: true},
	)

	result := service.CreateDirectory(CreateDirectoryInput{
		MountID: "documents",
		Name:    "Ideas",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
	core.AssertFalse(t, medium.IsDir("Ideas"))
}

func TestService_Rename_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("draft.txt", "hello"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.Rename(RenameInput{
		MountID: "documents",
		Path:    "draft.txt",
		Name:    "final.txt",
	})

	core.RequireTrue(t, result.OK)
	content, err := medium.Read("final.txt")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "hello", content)
	core.AssertFalse(t, medium.IsFile("draft.txt"))
}

func TestService_Rename_UglyRejectsRootAndInternal(t *core.T) {
	service := registeredMemoryService(
		t,
		"documents",
		coreio.NewMemoryMedium(),
		ReadWriteCapabilities(),
	)
	for _, relativePath := range []string{"", ".lthn-files"} {
		result := service.Rename(RenameInput{
			MountID: "documents",
			Path:    relativePath,
			Name:    "moved",
		})
		core.AssertFalse(t, result.OK, relativePath)
		core.AssertContains(
			t,
			result.Error(),
			string(ErrorBoundaryRejected),
			relativePath,
		)
	}
}

func TestService_Rename_BadProviderFailure(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("draft.txt", "hello"))
	service := registeredMemoryService(
		t,
		"documents",
		&failingMedium{Medium: base, renameErr: fs.ErrPermission},
		ReadWriteCapabilities(),
	)

	result := service.Rename(RenameInput{
		MountID: "documents",
		Path:    "draft.txt",
		Name:    "final.txt",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
	core.AssertTrue(t, base.IsFile("draft.txt"))
}

func TestService_Rename_EmitsRelativeEventGood(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("draft.txt", "hello"))
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		core.AssertTrue(t, c.ServiceShutdown(core.Background()).OK)
	})
	service := NewService(Options{
		Mounts: []Mount{memoryMount(
			"documents",
			medium,
			ReadWriteCapabilities(),
		)},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)
	var received FileEvent
	Subscribe(c, func(_ *core.Core, event FileEvent) {
		received = event
	})

	result := service.Rename(RenameInput{
		MountID: "documents",
		Path:    "draft.txt",
		Name:    "final.txt",
	})

	core.RequireTrue(t, result.OK)
	core.AssertEqual(t, "rename", received.Operation)
	core.AssertElementsMatch(
		t,
		[]string{"draft.txt", "final.txt"},
		received.Paths,
	)
	core.AssertNotContains(t, core.JSONMarshalString(received), "/Users/")
}
