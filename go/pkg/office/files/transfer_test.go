// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

type symlinkMedium struct {
	coreio.Medium
}

func (medium *symlinkMedium) Stat(relativePath string) (fs.FileInfo, error) {
	if relativePath == "escape" {
		return coreio.NewFileInfo(
			"escape",
			0,
			fs.ModeSymlink,
			core.Now(),
			false,
		), nil
	}
	return medium.Medium.Stat(relativePath)
}

type reportedSizeMedium struct {
	coreio.Medium
	path string
	size int64
}

func (medium *reportedSizeMedium) Stat(relativePath string) (fs.FileInfo, error) {
	info, err := medium.Medium.Stat(relativePath)
	if err != nil || relativePath != medium.path {
		return info, err
	}
	return coreio.NewFileInfo(
		info.Name(),
		medium.size,
		info.Mode(),
		info.ModTime(),
		info.IsDir(),
	), nil
}

func TestService_CopyAcrossMedia_Good(t *core.T) {
	source := coreio.NewMemoryMedium()
	destination := coreio.NewMemoryMedium()
	core.RequireNoError(t, source.Write("project/readme.md", "hello"))
	core.RequireNoError(t, source.Write(
		"project/src/main.go",
		"package main",
	))
	service := registeredService(t, []Mount{
		memoryMount("projects", source, ReadWriteCapabilities()),
		memoryMount("backup", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{
			MountID: "projects",
			Path:    "project",
		},
		Destination: FileAddress{
			MountID: "backup",
			Path:    "archive/project",
		},
	})

	core.RequireTrue(t, result.OK)
	content, err := destination.Read("archive/project/src/main.go")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "package main", content)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationCompleted, operation.Status)
}

func TestService_Copy_UglyRejectsLink(t *core.T) {
	source := &symlinkMedium{Medium: coreio.NewMemoryMedium()}
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount(
			"destination",
			coreio.NewMemoryMedium(),
			ReadWriteCapabilities(),
		),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "source", Path: "escape"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "escape",
		},
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorUnsupportedEntry))
}

func TestService_MoveAcrossMedia_BadSourceDeleteIsPartial(t *core.T) {
	source := &failingMedium{
		Medium:    coreio.NewMemoryMedium(),
		deleteErr: fs.ErrPermission,
	}
	core.RequireNoError(t, source.Write("report.md", "done"))
	destination := coreio.NewMemoryMedium()
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Move(TransferInput{
		Source: FileAddress{MountID: "source", Path: "report.md"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "report.md",
		},
	})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationPartial, operation.Status)
	core.AssertEqual(t, ErrorPartialMove, operation.Code)
	content, err := destination.Read("report.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "done", content)
	core.AssertTrue(t, source.IsFile("report.md"))
}

func TestService_MoveSameMedium_Good(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("draft.txt", "hello"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.Move(TransferInput{
		Source: FileAddress{
			MountID: "documents",
			Path:    "draft.txt",
		},
		Destination: FileAddress{
			MountID: "documents",
			Path:    "archive/final.txt",
		},
	})

	core.RequireTrue(t, result.OK)
	content, err := medium.Read("archive/final.txt")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "hello", content)
	core.AssertFalse(t, medium.IsFile("draft.txt"))
}

func TestService_Copy_BadDestinationConflict(t *core.T) {
	source := coreio.NewMemoryMedium()
	destination := coreio.NewMemoryMedium()
	core.RequireNoError(t, source.Write("report.md", "new"))
	core.RequireNoError(t, destination.Write("report.md", "existing"))
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "source", Path: "report.md"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "report.md",
		},
	})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, OperationConflict, operation.Status)
	content, err := destination.Read("report.md")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "existing", content)
}

func TestService_Copy_UglyEnforcesByteLimit(t *core.T) {
	source := coreio.NewMemoryMedium()
	core.RequireNoError(t, source.Write("large.bin", core.Repeat("x", 33)))
	service := NewService(Options{
		Mounts: []Mount{
			memoryMount("source", source, ReadWriteCapabilities()),
			memoryMount(
				"destination",
				coreio.NewMemoryMedium(),
				ReadWriteCapabilities(),
			),
		},
		Runtime: &stubRuntimeMetadata{},
		Limits: Limits{
			MaxListEntries:    200,
			MaxPreviewBytes:   512,
			MaxRecursiveDepth: 4,
			MaxRecursiveItems: 10,
			MaxTransferBytes:  32,
		},
	})
	core.RequireTrue(t, service.Register(core.New()).OK)

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "source", Path: "large.bin"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "large.bin",
		},
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorLimitExceeded))
}

func TestService_Copy_UglyRejectsSameAddressAndNestedDestination(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("project/readme.md", "hello"))
	service := registeredMemoryService(
		t,
		"projects",
		medium,
		ReadWriteCapabilities(),
	)

	for _, destination := range []string{"project", "project/backup"} {
		result := service.Copy(TransferInput{
			Source: FileAddress{MountID: "projects", Path: "project"},
			Destination: FileAddress{
				MountID: "projects",
				Path:    destination,
			},
		})
		core.AssertFalse(t, result.OK)
		core.AssertContains(t, result.Error(), string(ErrorBoundaryRejected))
	}
}

func TestService_Copy_BadUnownedNamespace(t *core.T) {
	source := coreio.NewMemoryMedium()
	destination := coreio.NewMemoryMedium()
	core.RequireNoError(t, source.Write("report.md", "content"))
	core.RequireNoError(
		t,
		destination.Write(".lthn-files/unrelated", "foreign"),
	)
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "source", Path: "report.md"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "report.md",
		},
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
	core.AssertFalse(t, destination.IsFile("report.md"))
	owner, err := destination.Read(".lthn-files/unrelated")
	core.AssertNoError(t, err)
	core.AssertEqual(t, "foreign", owner)
}

func TestService_Copy_BadGrowingProvider(t *core.T) {
	base := coreio.NewMemoryMedium()
	core.RequireNoError(t, base.Write("report.md", "content grew"))
	source := &reportedSizeMedium{
		Medium: base,
		path:   "report.md",
		size:   3,
	}
	destination := coreio.NewMemoryMedium()
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "source", Path: "report.md"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "report.md",
		},
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorLimitExceeded))
	core.AssertFalse(t, destination.IsFile("report.md"))
}

func TestService_Copy_BadWriteStreamCleansStaging(t *core.T) {
	source := coreio.NewMemoryMedium()
	core.RequireNoError(t, source.Write("report.md", "content"))
	baseDestination := coreio.NewMemoryMedium()
	destination := &failingMedium{
		Medium:         baseDestination,
		writeStreamErr: fs.ErrPermission,
	}
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Copy(TransferInput{
		Source: FileAddress{MountID: "source", Path: "report.md"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "report.md",
		},
	})

	core.AssertFalse(t, result.OK)
	core.AssertFalse(t, baseDestination.IsFile("report.md"))
	entries, err := baseDestination.List(".lthn-files/staging")
	core.AssertNoError(t, err)
	core.AssertEqual(t, 0, len(entries))
}

func TestService_MoveAcrossMedia_BadConflictNamesMove(t *core.T) {
	source := coreio.NewMemoryMedium()
	destination := coreio.NewMemoryMedium()
	core.RequireNoError(t, source.Write("report.md", "new"))
	core.RequireNoError(t, destination.Write("report.md", "existing"))
	service := registeredService(t, []Mount{
		memoryMount("source", source, ReadWriteCapabilities()),
		memoryMount("destination", destination, ReadWriteCapabilities()),
	}, &stubRuntimeMetadata{})

	result := service.Move(TransferInput{
		Source: FileAddress{MountID: "source", Path: "report.md"},
		Destination: FileAddress{
			MountID: "destination",
			Path:    "report.md",
		},
	})

	core.RequireTrue(t, result.OK)
	operation := result.Value.(FileOperationResult)
	core.AssertEqual(t, "move", operation.Operation)
	core.AssertEqual(t, OperationConflict, operation.Status)
}

func TestService_TransferLimits_GoodFillsOnlyUnsetFields(t *core.T) {
	defaults := DefaultLimits()

	zeroed := &Service{}
	zeroedLimits := zeroed.transferLimits()
	core.AssertEqual(t, defaults.MaxRecursiveDepth, zeroedLimits.MaxRecursiveDepth)
	core.AssertEqual(t, defaults.MaxRecursiveItems, zeroedLimits.MaxRecursiveItems)
	core.AssertEqual(t, defaults.MaxTransferBytes, zeroedLimits.MaxTransferBytes)

	explicit := &Service{limits: Limits{
		MaxRecursiveDepth: 3,
		MaxRecursiveItems: 7,
		MaxTransferBytes:  11,
	}}
	got := explicit.transferLimits()
	core.AssertEqual(t, 3, got.MaxRecursiveDepth)
	core.AssertEqual(t, 7, got.MaxRecursiveItems)
	core.AssertEqual(t, int64(11), got.MaxTransferBytes)
}

// --- internal-namespace initialisation fault arms -------------------
// Register drives initialiseInternalNamespace per mount; each fault
// is injected through the shared failingMedium double, and the
// adoption arms use a pre-seeded memory medium. Warnings are the
// contract on every fault path — the mount stays usable, only
// internalReady stays false.

// registerWithMedium composes a single Trash-capable mount over the
// supplied medium and runs the real Register path.
func registerWithMedium(t *core.T, medium coreio.Medium) *Service {
	t.Helper()
	c := core.New()
	core.RequireTrue(t, c.ServiceStartup(core.Background(), nil).OK)
	t.Cleanup(func() {
		core.AssertTrue(t, c.ServiceShutdown(core.Background()).OK)
	})
	service := NewService(Options{
		Mounts:  []Mount{memoryMount("docs", medium, ReadWriteCapabilities())},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(c).OK)
	return service
}

// TestTransfer_InitialiseInternalNamespace_Good — a namespace this
// service owns from an earlier run is adopted, and a fresh medium
// gets the namespace created with the owner marker.
func TestTransfer_InitialiseInternalNamespace_Good(t *core.T) {
	owned := coreio.NewMemoryMedium()
	core.RequireNoError(t, owned.EnsureDir(internalNamespace))
	core.RequireNoError(t, owned.WriteMode(internalOwnerPath, internalOwnerDocument, 0600))
	core.AssertTrue(t, registerWithMedium(t, owned).internalReady["docs"])

	core.AssertTrue(t, registerWithMedium(t, coreio.NewMemoryMedium()).internalReady["docs"])
}

// TestTransfer_InitialiseInternalNamespace_Bad — a Stat failure that
// is not NotExist, a failed namespace creation, and a failed owner
// marker each leave the namespace unavailable without failing
// registration.
func TestTransfer_InitialiseInternalNamespace_Bad(t *core.T) {
	statBroken := &failingMedium{Medium: coreio.NewMemoryMedium(), statErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, statBroken).internalReady["docs"])

	dirBroken := &failingMedium{Medium: coreio.NewMemoryMedium(), ensureDirErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, dirBroken).internalReady["docs"])

	markerBroken := &failingMedium{Medium: coreio.NewMemoryMedium(), writeModeErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, markerBroken).internalReady["docs"])
}

// TestTransfer_InitialiseInternalNamespace_Ugly — an existing
// namespace with a foreign (or unreadable) owner marker is never
// adopted: the service must not treat someone else's directory as
// its trash substrate.
func TestTransfer_InitialiseInternalNamespace_Ugly(t *core.T) {
	foreign := coreio.NewMemoryMedium()
	core.RequireNoError(t, foreign.EnsureDir(internalNamespace))
	core.RequireNoError(t, foreign.WriteMode(internalOwnerPath, `{"owner":"someone-else"}`, 0600))
	core.AssertFalse(t, registerWithMedium(t, foreign).internalReady["docs"])

	unreadable := coreio.NewMemoryMedium()
	core.RequireNoError(t, unreadable.EnsureDir(internalNamespace))
	core.RequireNoError(t, unreadable.WriteMode(internalOwnerPath, internalOwnerDocument, 0600))
	wrapped := &failingMedium{Medium: unreadable, readErr: fs.ErrPermission}
	core.AssertFalse(t, registerWithMedium(t, wrapped).internalReady["docs"])
}
