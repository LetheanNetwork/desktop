// SPDX-License-Identifier: EUPL-1.2

package files_test

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
	"dappco.re/lthn/desktop/pkg/office/files"
)

func ExampleService_ListMounts() {
	service, _ := exampleFilesService()

	result := service.ListMounts()
	catalogue := result.Value.(files.MountCatalogue)
	core.Println(catalogue.Mounts[0].Name)
	// Output: Documents
}

func ExampleService_ListDirectory() {
	service, medium := exampleFilesService()
	_ = medium.Write("notes/readme.md", "hello")

	result := service.ListDirectory(files.ListDirectoryInput{
		MountID: "documents",
	})
	snapshot := result.Value.(files.DirectorySnapshot)
	core.Println(snapshot.Entries[0].Name)
	// Output: notes
}

func ExampleService_Preview() {
	service, medium := exampleFilesService()
	_ = medium.Write("notes/readme.md", "hello")

	result := service.Preview(files.PreviewInput{
		MountID: "documents",
		Path:    "notes/readme.md",
	})
	preview := result.Value.(files.FilePreview)
	core.Println(preview.Content)
	// Output: hello
}

func ExampleService_CreateDirectory() {
	service, _ := exampleFilesService()

	result := service.CreateDirectory(files.CreateDirectoryInput{
		MountID: "documents",
		Name:    "Ideas",
	})
	operation := result.Value.(files.FileOperationResult)
	core.Println(operation.Status)
	// Output: completed
}

func ExampleResolveHostItems() {
	result := files.ResolveHostItems(nil, []string{"/tmp/report.txt"})
	core.Println(result.OK)
	// Output: false
}

func ExampleResolveLocalWorkspace() {
	medium := coreio.NewMemoryMedium()
	_ = medium.EnsureDir("desktop")
	service := files.NewService(files.Options{
		Mounts: []files.Mount{{
			ID:                 "projects",
			Name:               "Projects",
			Kind:               "local",
			LocalRoot:          "/workspace",
			Capabilities:       files.ReadWriteCapabilities(),
			Medium:             medium,
			ContainmentAudited: true,
		}},
		Runtime: files.NewMemoryRuntimeMetadata(),
	})
	_ = service.Register(core.New())

	result := files.ResolveLocalWorkspace(service, "projects", "desktop")
	core.Println(result.Value)
	// Output: /workspace/desktop
}

func exampleFilesService() (*files.Service, coreio.Medium) {
	medium := coreio.NewMemoryMedium()
	service := files.NewService(files.Options{
		Mounts: []files.Mount{{
			ID:                 "documents",
			Name:               "Documents",
			Kind:               "memory",
			Capabilities:       files.ReadWriteCapabilities(),
			Medium:             medium,
			ContainmentAudited: true,
		}},
		Runtime: files.NewMemoryRuntimeMetadata(),
	})
	_ = service.Register(core.New())
	return service, medium
}
