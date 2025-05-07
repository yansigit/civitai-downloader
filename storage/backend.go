package storage

import (
	"database/sql"
)

// StorageBackend defines an interface for saving files
// EnsureDirectory: create or verify a directory path
// UploadFile: upload a file by local path
// SaveFile: save via content []byte, return URL or path.
//
//	DB params are for backends that support chunk logging (like Pomf).
//	Other params like baseStoragePath, relativePath are for path-based backends.
//
// methods params: (body/filePath, basePath/folderId, relativePath, fileName/locationId, optional note)
type StorageBackend interface {
	EnsureDirectory(path string) error
	SaveFile(db *sql.DB, modelFileID int64, fileCategoryForChunks string, content []byte, baseStoragePath, relativePath, fileName string) (string, error)
}
