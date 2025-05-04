package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/downloader"
)

// StorageBackend defines an interface for saving files, metadata, and descriptions
type StorageBackend interface {
	SaveFile(sourceURL, destinationPath string, model downloader.Model, version downloader.ModelVersion, config config.Config) error
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
func (lsb *LocalStorageBackend) SaveFile(sourceURL, destinationPath string, model downloader.Model, version downloader.ModelVersion, config config.Config) error {
	// Construct the full file path
	// Use the downloader.DownloadAll function to download the file
	file := version.Files[0]
	err := downloader.DownloadAll(file, destinationPath, model, version, &config)
	if err != nil {
		return fmt.Errorf("failed to download file from %s: %w", sourceURL, err)
	}

	return nil
}

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
