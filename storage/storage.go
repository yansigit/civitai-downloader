package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// StorageBackend defines an interface for saving files, metadata, and descriptions
type StorageBackend interface {
	SaveMetadata(metadata []byte, destinationPath, modelName string) error
	SaveDescription(description string, destinationPath, modelName string) error
	EnsureDirectory(path string) error
}

// LocalStorageBackend implements the StorageBackend interface for local filesystem
type LocalStorageBackend struct{}

// EnsureDirectory ensures that the specified directory exists
func (lsb *LocalStorageBackend) EnsureDirectory(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}
	return nil
}

// SaveFile saves a file from a source URL to the local filesystem

// SaveMetadata saves metadata to a file in the local filesystem
func (lsb *LocalStorageBackend) SaveMetadata(metadata []byte, destinationPath, modelName string) error {
	filePath := filepath.Join(destinationPath, fmt.Sprintf("%s.metadata.json", modelName))
	return os.WriteFile(filePath, metadata, 0644)
}

// SaveDescription saves a description to a file in the local filesystem
func (lsb *LocalStorageBackend) SaveDescription(description string, destinationPath, modelName string) error {
	filePath := filepath.Join(destinationPath, fmt.Sprintf("%s.description.txt", modelName))
	return os.WriteFile(filePath, []byte(description), 0644)
}
