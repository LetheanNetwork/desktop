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

func TestPreviewMIME_GoodMapsKnownExtensions(t *core.T) {
	cases := map[string]string{
		"notes.md":       "text/markdown",
		"notes.MARKDOWN": "text/markdown",
		"data.json":      "application/json",
		"config.yaml":    "application/yaml",
		"config.yml":     "application/yaml",
		"page.html":      "text/html",
		"page.htm":       "text/html",
		"style.css":      "text/css",
		"script.js":      "text/javascript",
		"script.mjs":     "text/javascript",
		"module.ts":      "text/typescript",
		"main.go":        "text/x-go",
		"photo.png":      "image/png",
		"photo.jpg":      "image/jpeg",
		"photo.jpeg":     "image/jpeg",
		"animation.gif":  "image/gif",
		"icon.svg":       "image/svg+xml",
		"document.pdf":   "application/pdf",
	}
	for path, expected := range cases {
		t.Run(path, func(t *core.T) {
			core.AssertEqual(t, expected, previewMIME(path, false))
		})
	}
}

func TestPreviewMIME_GoodBinaryFallbackOverridesUnknownExtension(
	t *core.T,
) {
	core.AssertEqual(
		t,
		"application/octet-stream",
		previewMIME("blob.dat", true),
	)
}

func TestPreviewMIME_GoodTextFallbackForUnknownExtension(t *core.T) {
	core.AssertEqual(t, "text/plain", previewMIME("blob.dat", false))
}

func TestService_RecordRecent_BadLoadFailure(t *core.T) {
	medium := coreio.NewMemoryMedium()
	core.RequireNoError(t, medium.Write("readme.md", "hello"))
	loadErr := core.E("test.inject", "load failed", nil)
	service := registeredMemoryService(
		t,
		"documents",
		medium,
		ReadWriteCapabilities(),
	)
	service.runtime = &stubRuntimeMetadata{loadErr: loadErr}

	result := service.Preview(PreviewInput{
		MountID: "documents",
		Path:    "readme.md",
	})

	core.AssertFalse(t, result.OK)
	core.AssertContains(t, result.Error(), string(ErrorProviderUnavailable))
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
