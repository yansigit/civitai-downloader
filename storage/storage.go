package storage

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
)

// StorageBackend defines an interface for saving files, metadata, and descriptions
type StorageBackend interface {
	EnsureDirectory(path string) error
	UploadFile(filePath, fileName, locationId, note string) error
}

type RemoteStorageBackend struct {
	AuthToken string
}

func (rsb *RemoteStorageBackend) SaveMetadata(metadata []byte, destinationPath, modelName string) error {
	// Placeholder implementation for remote storage
	return fmt.Errorf("SaveMetadata is not supported for RemoteStorageBackend")
}

func (rsb *RemoteStorageBackend) SaveDescription(description string, destinationPath, modelName string) error {
	// Placeholder implementation for remote storage
	return fmt.Errorf("SaveDescription is not supported for RemoteStorageBackend")
}

func (rsb *RemoteStorageBackend) UploadFile(filePath, fileName, locationId, note string) error {
	// Validate file name length
	if len(fileName) > 500 {
		return fmt.Errorf("file name exceeds the maximum length of 500 characters")
	}

	// Construct the endpoint URL
	baseURL := "https://w.fuckingfast.net/"
	url := fmt.Sprintf("%s%s", baseURL, fileName)
	if locationId != "" {
		url += fmt.Sprintf("?locationId=%s", locationId)
	} else if note != "" {
		encodedNote := base64.StdEncoding.EncodeToString([]byte(note))
		url += fmt.Sprintf("?note=%s", encodedNote)
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create HTTP request
	// Get file info to set Content-Length header
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	req, err := http.NewRequest("PUT", url, file)
	req.ContentLength = fileInfo.Size()
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add Authorization header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rsb.AuthToken))

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed with status: %s", resp.Status)
	}

	return nil
}

func (rsb *RemoteStorageBackend) EnsureDirectory(path string) error {
	// Placeholder implementation for remote storage
	return nil
}

// LocalStorageBackend implements the StorageBackend interface for local filesystem
type LocalStorageBackend struct{}

func (lsb *LocalStorageBackend) UploadFile(filePath, fileName, locationId, note string) error {
	// Placeholder implementation for local storage
	return fmt.Errorf("UploadFile is not supported for LocalStorageBackend")
}

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
