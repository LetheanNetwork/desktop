// SPDX-License-Identifier: EUPL-1.2

package files

import (
	"bytes"
	goio "io"
	"io/fs"
	"path"
	"unicode/utf8"

	core "dappco.re/go"
)

func (s *Service) preview(input PreviewInput) core.Result {
	mount, err := s.mount(input.MountID)
	if err != nil {
		return core.Fail(err)
	}
	if !mount.Capabilities.Preview {
		return core.Fail(capabilityFailure(
			"Preview",
			mount.ID,
			input.Path,
		))
	}
	relativePath, err := normaliseRelativePath(input.Path, false)
	if err != nil {
		return core.Fail(withAddress(err, mount.ID, input.Path))
	}
	info, err := mount.Medium.Stat(relativePath)
	if err != nil {
		return core.Fail(providerFailure(
			"Preview",
			mount.ID,
			relativePath,
			err,
		))
	}
	mode := info.Mode()
	if info.IsDir() || mode&fs.ModeSymlink != 0 || !mode.IsRegular() {
		return core.Fail(newFailure(
			ErrorUnsupportedEntry,
			mount.ID,
			relativePath,
			"Only regular files can be previewed.",
			nil,
		))
	}
	reader, err := mount.Medium.ReadStream(relativePath)
	if err != nil {
		return core.Fail(providerFailure(
			"Preview",
			mount.ID,
			relativePath,
			err,
		))
	}
	maximum := s.limits.MaxPreviewBytes
	if maximum <= 0 {
		maximum = DefaultLimits().MaxPreviewBytes
	}
	payload, readErr := goio.ReadAll(goio.LimitReader(reader, maximum+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil {
		if readErr == nil {
			readErr = closeErr
		}
		return core.Fail(providerFailure(
			"Preview",
			mount.ID,
			relativePath,
			readErr,
		))
	}
	truncated := int64(len(payload)) > maximum
	if truncated {
		payload = payload[:maximum]
	}
	binary := !utf8.Valid(payload) || bytes.IndexByte(payload, 0) >= 0
	content := ""
	lines := 0
	if !binary {
		content = string(payload)
		if len(payload) > 0 {
			lines = bytes.Count(payload, []byte{'\n'}) + 1
		}
	}
	return core.Ok(FilePreview{
		MountID:      mount.ID,
		RelativePath: relativePath,
		Name:         path.Base(relativePath),
		Content:      content,
		MIME:         previewMIME(relativePath, binary),
		BytesRead:    int64(len(payload)),
		SizeBytes:    info.Size(),
		Lines:        lines,
		Truncated:    truncated,
		Binary:       binary,
	})
}

func previewMIME(relativePath string, binary bool) string {
	extension := core.Lower(path.Ext(relativePath))
	switch extension {
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".go":
		return "text/x-go"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	}
	if binary {
		return "application/octet-stream"
	}
	return "text/plain"
}
