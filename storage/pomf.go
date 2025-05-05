package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/yansigit/civitai-downloader/progress"
)

// PomfStorageBackend implements file storage using pomf.lain.la
// It satisfies the StorageBackend interface.
type PomfStorageBackend struct{}

// EnsureDirectory is a no-op for PomfStorageBackend
func (psb *PomfStorageBackend) EnsureDirectory(path string) error {
	return nil
}

// SaveFile uploads a file to pomf.lain.la using an io.Reader
func (psb *PomfStorageBackend) SaveFile(body io.Reader, baseStoragePath, relativePath, fileName string) (string, error) {
	// Prepare multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("files[]", fileName)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, body); err != nil {
		return "", err
	}
	writer.Close()

	// Show upload progress bar if terminal
	var reqBody io.Reader = &buf
	if progress.IsTerminal() {
		bar := progress.NewProgressBar(int64(buf.Len()), "[Uploading to Pomf] ", "yellow")
		reqBody = progress.NewProgressReader(&buf, bar)
		defer func() { os.Stdout.Write([]byte("\n")) }()
	}

	// Send request
	req, err := http.NewRequest("POST", "https://pomf.lain.la/upload.php", reqBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pomf upload failed: %d", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Files   []struct {
			URL string `json:"url"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.Success || len(result.Files) == 0 {
		return "", fmt.Errorf("pomf upload no files returned")
	}
	return result.Files[0].URL, nil
}
