// SPDX-License-Identifier: EUPL-1.2

package files

import (
	goio "io"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestDefaultOptions_LocalMountsUseSandboxedMedia_Good(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seedHomeDirectories(t, home, "Documents", "Downloads", "Code", "Lethean/conf/models")
	c := filesCoreFixture(t)

	result := DefaultOptions(c)

	core.RequireTrue(t, result.OK)
	options := result.Value.(Options)
	t.Cleanup(func() {
		closeOwnedMounts(t, options.Mounts)
	})
	core.AssertElementsMatch(
		t,
		[]string{"documents", "downloads", "models", "projects"},
		mountIDs(options.Mounts),
	)
	core.AssertNotNil(t, options.Runtime)
	for _, mount := range options.Mounts {
		core.AssertTrue(t, mount.Owned)
		core.AssertTrue(t, mount.ContainmentAudited)
	}
}

func TestDefaultOptions_BadProviderCreationFailure(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	homeMedium := openSandboxedMedium(t, home)
	core.RequireNoError(t, homeMedium.Write("Documents", "not a directory"))
	c := filesCoreFixture(t)

	result := DefaultOptions(c)

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
}

func TestDefaultOptions_RuntimeUsesRegisteredMedium_Ugly(t *core.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	seedHomeDirectories(t, home, "Documents")
	c := filesCoreFixture(t)

	result := DefaultOptions(c)
	core.RequireTrue(t, result.OK)
	options := result.Value.(Options)
	t.Cleanup(func() {
		closeOwnedMounts(t, options.Mounts)
	})
	core.RequireNoError(t, options.Runtime.Save(RuntimeSnapshot{
		Favourites: []Favourite{{MountID: "documents", Path: ""}},
	}))

	ioService, ok := core.ServiceFor[*coreio.Service](c, "io")
	core.RequireTrue(t, ok)
	content, err := ioService.Medium.Read("desktop/files/runtime.json")
	core.AssertNoError(t, err)
	core.AssertContains(t, content, `"mountId":"documents"`)
}

func TestService_OnShutdownClosesOwnedMounts_Ugly(t *core.T) {
	medium := &closingMedium{Medium: coreio.NewMemoryMedium()}
	service := NewService(Options{
		Mounts: []Mount{{
			ID:                 "documents",
			Name:               "Documents",
			Medium:             medium,
			Owned:              true,
			ContainmentAudited: true,
		}},
		Runtime: &stubRuntimeMetadata{},
	})
	core.RequireTrue(t, service.Register(core.New()).OK)

	first := service.OnShutdown(core.Background())
	second := service.OnShutdown(core.Background())

	core.AssertTrue(t, first.OK)
	core.AssertTrue(t, second.OK)
	core.AssertEqual(t, 1, medium.closeCalls)
}

type closingMedium struct {
	coreio.Medium
	closeCalls int
}

func (medium *closingMedium) Close() error {
	medium.closeCalls++
	return nil
}

func filesCoreFixture(t *core.T) *core.Core {
	t.Helper()
	c := core.New(core.WithName(
		"io",
		coreio.NewService(coreio.IOConfig{Root: t.TempDir()}),
	))
	ioService, ok := core.ServiceFor[*coreio.Service](c, "io")
	core.RequireTrue(t, ok)
	core.RequireTrue(t, ioService != nil)
	t.Cleanup(func() {
		core.AssertTrue(t, c.ServiceShutdown(core.Background()).OK)
	})
	return c
}

func seedHomeDirectories(t *core.T, home string, paths ...string) {
	t.Helper()
	medium := openSandboxedMedium(t, home)
	for _, path := range paths {
		core.RequireNoError(t, medium.EnsureDir(path))
	}
}

func openSandboxedMedium(t *core.T, root string) coreio.Medium {
	t.Helper()
	medium, err := coreio.NewSandboxed(root)
	core.RequireNoError(t, err)
	t.Cleanup(func() {
		if closer, ok := medium.(goio.Closer); ok {
			core.AssertNoError(t, closer.Close())
		}
	})
	return medium
}

func closeOwnedMounts(t *core.T, mounts []Mount) {
	t.Helper()
	for _, mount := range mounts {
		if !mount.Owned {
			continue
		}
		if closer, ok := mount.Medium.(goio.Closer); ok {
			core.AssertNoError(t, closer.Close())
		}
	}
}

func mountIDs(mounts []Mount) []string {
	ids := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		ids = append(ids, mount.ID)
	}
	return ids
}
