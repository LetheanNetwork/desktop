// SPDX-License-Identifier: EUPL-1.2

package files

import (
	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestResolveLocalWorkspace_GoodUsesAuditedMount(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, medium.EnsureDir("projects/desktop"))
	service := registeredService(t, []Mount{{
		ID:                 "projects",
		Name:               "Projects",
		Kind:               "local",
		LocalRoot:          root,
		Capabilities:       ReadWriteCapabilities(),
		Medium:             medium,
		Owned:              true,
		ContainmentAudited: true,
	}}, &stubRuntimeMetadata{})

	result := ResolveLocalWorkspace(service, "projects", "projects/desktop")

	core.RequireTrue(t, result.OK, result.Error())
	core.AssertEqual(t, core.PathJoin(root, "projects", "desktop"), result.Value)
}

func TestResolveLocalWorkspace_BadRejectsProviderAuthority(t *core.T) {
	remote := coreio.NewMemoryMedium()
	core.RequireNoError(t, remote.EnsureDir("projects"))
	service := registeredService(t, []Mount{{
		ID:                 "bucket",
		Name:               "Bucket",
		Kind:               "s3",
		Capabilities:       ReadWriteCapabilities(),
		Medium:             remote,
		ContainmentAudited: true,
	}}, &stubRuntimeMetadata{})

	for _, input := range []struct {
		mountID string
		path    string
	}{
		{mountID: "missing", path: ""},
		{mountID: "bucket", path: "projects"},
		{mountID: "bucket", path: "../escape"},
	} {
		result := ResolveLocalWorkspace(service, input.mountID, input.path)
		core.AssertFalse(t, result.OK)
	}
}

func TestResolveLocalWorkspace_UglyRejectsFilesAndUnavailableService(t *core.T) {
	root := t.TempDir()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	core.RequireNoError(t, medium.Write("not-a-directory.txt", "content"))
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

	core.AssertFalse(t, ResolveLocalWorkspace(nil, "documents", "").OK)
	core.AssertFalse(
		t,
		ResolveLocalWorkspace(service, "documents", "not-a-directory.txt").OK,
	)
}
