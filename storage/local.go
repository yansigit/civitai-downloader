package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// LocalStorageBackend saves files to local filesystem
type LocalStorageBackend struct{}

func (lsb *LocalStorageBackend) EnsureDirectory(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create directory '%s': %w", path, err)
		}
	}
	return nil
}

func (lsb *LocalStorageBackend) SaveFile(db *sql.DB, modelFileID int64, fileCategoryForChunks string, content []byte, baseStoragePath, relativePath, fileName string) (string, error) {
	fullDir := filepath.Join(baseStoragePath, relativePath)
	if err := lsb.EnsureDirectory(fullDir); err != nil {
		return "", fmt.Errorf("ensure directory failed: %w", err)
	}
	fullPath := filepath.Join(fullDir, fileName)
	f, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("create file failed: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(content); err != nil {
		os.Remove(fullPath)
		return "", fmt.Errorf("write file failed: %w", err)
	}
	return fullPath, nil
}
