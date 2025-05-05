package storage

import "io"

// StorageBackend defines an interface for saving files
// EnsureDirectory: create or verify a directory path
// UploadFile: upload a file by local path
// SaveFile: save via io.Reader, return URL or path
// methods params: (body/filePath, basePath/folderId, relativePath, fileName/locationId, optional note)
type StorageBackend interface {
	EnsureDirectory(path string) error
	SaveFile(body io.Reader, baseStoragePath, relativePath, fileName string) (string, error)
}
