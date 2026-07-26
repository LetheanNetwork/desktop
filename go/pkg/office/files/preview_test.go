// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

func TestService_Preview_GoodText(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello\nworld"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "readme.md",
	})

	core.RequireTrue(t, result.OK)
	preview := result.Value.(FilePreview)
	core.AssertEqual(t, "hello\nworld", preview.Content)
	core.AssertEqual(t, "text/markdown", preview.MIME)
	core.AssertEqual(t, 2, preview.Lines)
	core.AssertFalse(t, preview.Binary)
}

func TestService_Preview_UglyBounded(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("large.txt", core.Repeat("x", 33)))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)
	service.limits.MaxPreviewBytes = 32

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "large.txt",
	})

	core.RequireTrue(t, result.OK)
	preview := result.Value.(FilePreview)
	core.AssertEqual(t, int64(32), preview.BytesRead)
	core.AssertTrue(t, preview.Truncated)
	core.AssertEqual(t, "documents", preview.MountID)
	core.AssertEqual(t, "large.txt", preview.RelativePath)
}

func TestService_Preview_BadBinary(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("image.bin", "png\x00payload"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "image.bin",
	})

	core.RequireTrue(t, result.OK)
	preview := result.Value.(FilePreview)
	core.AssertTrue(t, preview.Binary)
	core.AssertEqual(t, "", preview.Content)
	core.AssertEqual(t, "application/octet-stream", preview.MIME)
}

func TestService_Preview_BadProviderError(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello"))
	service := registeredMemoryService(
		t,
		"documents",
		&failingMedium{Medium: medium, readStreamErr: fs.ErrPermission},
		ReadWriteCapabilities(),
	)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "readme.md",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorCapabilityDenied))
}

func TestService_Preview_UglyRejectsDirectory(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.EnsureDir("notes"))
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "notes",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorUnsupportedEntry))
}
