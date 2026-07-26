// SPDX-License-Identifier: EUPL-1.2

// Wails surface for the canonical Files service. Each method is deliberately a
// one-return delegation so mount resolution, capability checks, and Medium
// access remain in the tested service helpers.
package files

import core "dappco.re/go"

// ListMounts returns the registered provider-neutral mount catalogue.
func (s *Service) ListMounts() core.Result {
	return s.listMounts()
}

// ListDirectory returns a deterministic bounded directory page.
func (s *Service) ListDirectory(input ListDirectoryInput) core.Result {
	return s.listDirectory(input)
}

// Preview returns a bounded text or binary file preview.
func (s *Service) Preview(input PreviewInput) core.Result {
	return s.preview(input)
}

// CreateDirectory creates one child directory through its registered Medium.
func (s *Service) CreateDirectory(input CreateDirectoryInput) core.Result {
	return s.createDirectory(input)
}

// Rename replaces one entry's final name without accepting a destination root.
func (s *Service) Rename(input RenameInput) core.Result {
	return s.rename(input)
}

// Copy stages and commits a bounded transfer through registered media.
func (s *Service) Copy(input TransferInput) core.Result {
	return s.copy(input)
}

// Move renames within one mount or copies then removes across mounts.
func (s *Service) Move(input TransferInput) core.Result {
	return s.move(input)
}

// Trash moves an entry into the mount's owned trash namespace.
func (s *Service) Trash(input TrashInput) core.Result {
	return s.trash(input)
}

// ListTrash projects trusted runtime receipts through registered media.
func (s *Service) ListTrash() core.Result {
	return s.listTrash()
}

// Restore reverses a trusted trash receipt when its destination is available.
func (s *Service) Restore(input RestoreInput) core.Result {
	return s.restore(input)
}

// Delete permanently deletes one explicitly confirmed address or receipt.
func (s *Service) Delete(input DeleteInput) core.Result {
	return s.delete(input)
}
