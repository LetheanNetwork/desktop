// SPDX-License-Identifier: EUPL-1.2

package files

import core "dappco.re/go"

const (
	internalNamespace = ".lthn-files"
	maxMountIDBytes   = 64
)

func normaliseRelativePath(raw string, allowRoot bool) (string, error) {
	if raw == "" {
		if allowRoot {
			return "", nil
		}
		return "", newFailure(
			ErrorInvalidInput,
			"",
			"",
			"path is required",
			nil,
		)
	}
	if core.HasPrefix(raw, "/") ||
		core.Contains(raw, "\\") ||
		(len(raw) > 1 && raw[1] == ':') {
		return "", newFailure(
			ErrorBoundaryRejected,
			"",
			"",
			"absolute paths are not accepted",
			nil,
		)
	}
	parts := core.Split(raw, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", newFailure(
				ErrorBoundaryRejected,
				"",
				"",
				"path traversal is not accepted",
				nil,
			)
		}
		if hasUnsafePathRune(part) {
			return "", newFailure(
				ErrorBoundaryRejected,
				"",
				"",
				"path contains a control character",
				nil,
			)
		}
		clean = append(clean, part)
	}
	if len(clean) > 0 && clean[0] == internalNamespace {
		return "", newFailure(
			ErrorBoundaryRejected,
			"",
			"",
			"internal Files paths are not addressable",
			nil,
		)
	}
	return core.Join("/", clean...), nil
}

func validateEntryName(name string) error {
	if name == "" {
		return newFailure(
			ErrorInvalidInput,
			"",
			"",
			"entry name is required",
			nil,
		)
	}
	if name == "." ||
		name == ".." ||
		core.Contains(name, "/") ||
		core.Contains(name, "\\") ||
		hasUnsafePathRune(name) {
		return newFailure(
			ErrorBoundaryRejected,
			"",
			"",
			"entry name is not accepted",
			nil,
		)
	}
	return nil
}

func validateMountID(id string) error {
	if id == "" || len(id) > maxMountIDBytes {
		return newFailure(
			ErrorInvalidMount,
			id,
			"",
			"mount ID is invalid",
			nil,
		)
	}
	for _, character := range []byte(id) {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '-' {
			continue
		}
		return newFailure(
			ErrorInvalidMount,
			id,
			"",
			"mount ID is invalid",
			nil,
		)
	}
	return nil
}

func hasUnsafePathRune(value string) bool {
	for _, character := range value {
		switch {
		case character <= 0x1f:
			return true
		case character >= 0x7f && character <= 0x9f:
			return true
		case character == 0x200e || character == 0x200f:
			return true
		case character >= 0x202a && character <= 0x202e:
			return true
		case character >= 0x2066 && character <= 0x2069:
			return true
		}
	}
	return false
}
