// SPDX-License-Identifier: EUPL-1.2

package files

import (
	goio "io"
	"io/fs"

	core "dappco.re/go"
	coreio "dappco.re/go/io"
)

type failingMedium struct {
	coreio.Medium
	readErr        error
	writeErr       error
	writeModeErr   error
	ensureDirErr   error
	deleteErr      error
	deleteAllErr   error
	renameErr      error
	listErr        error
	statErr        error
	openErr        error
	createErr      error
	appendErr      error
	readStreamErr  error
	writeStreamErr error
}

func (medium *failingMedium) Read(path string) (string, error) {
	if medium.readErr != nil {
		return "", medium.readErr
	}
	return medium.Medium.Read(path)
}

func (medium *failingMedium) Write(path, content string) error {
	if medium.writeErr != nil {
		return medium.writeErr
	}
	return medium.Medium.Write(path, content)
}

func (medium *failingMedium) WriteMode(
	path string,
	content string,
	mode fs.FileMode,
) error {
	if medium.writeModeErr != nil {
		return medium.writeModeErr
	}
	return medium.Medium.WriteMode(path, content, mode)
}

func (medium *failingMedium) EnsureDir(path string) error {
	if medium.ensureDirErr != nil {
		return medium.ensureDirErr
	}
	return medium.Medium.EnsureDir(path)
}

func (medium *failingMedium) Delete(path string) error {
	if medium.deleteErr != nil {
		return medium.deleteErr
	}
	return medium.Medium.Delete(path)
}

func (medium *failingMedium) DeleteAll(path string) error {
	if medium.deleteAllErr != nil {
		return medium.deleteAllErr
	}
	return medium.Medium.DeleteAll(path)
}

func (medium *failingMedium) Rename(oldPath, newPath string) error {
	if medium.renameErr != nil {
		return medium.renameErr
	}
	return medium.Medium.Rename(oldPath, newPath)
}

func (medium *failingMedium) List(path string) ([]fs.DirEntry, error) {
	if medium.listErr != nil {
		return nil, medium.listErr
	}
	return medium.Medium.List(path)
}

func (medium *failingMedium) Stat(path string) (fs.FileInfo, error) {
	if medium.statErr != nil {
		return nil, medium.statErr
	}
	return medium.Medium.Stat(path)
}

func (medium *failingMedium) Open(path string) (fs.File, error) {
	if medium.openErr != nil {
		return nil, medium.openErr
	}
	return medium.Medium.Open(path)
}

func (medium *failingMedium) Create(path string) (goio.WriteCloser, error) {
	if medium.createErr != nil {
		return nil, medium.createErr
	}
	return medium.Medium.Create(path)
}

func (medium *failingMedium) Append(path string) (goio.WriteCloser, error) {
	if medium.appendErr != nil {
		return nil, medium.appendErr
	}
	return medium.Medium.Append(path)
}

func (medium *failingMedium) ReadStream(path string) (goio.ReadCloser, error) {
	if medium.readStreamErr != nil {
		return nil, medium.readStreamErr
	}
	return medium.Medium.ReadStream(path)
}

func (medium *failingMedium) WriteStream(path string) (goio.WriteCloser, error) {
	if medium.writeStreamErr != nil {
		return nil, medium.writeStreamErr
	}
	return medium.Medium.WriteStream(path)
}

func TestServiceName(t *core.T) {
	service := &Service{}

	core.AssertEqual(t, "Files", service.ServiceName())
}
