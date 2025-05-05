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

// FileditchStorageBackend implements file storage using fileditch.com
type FileditchStorageBackend struct{}

// EnsureDirectory is a no-op for FileditchStorageBackend
func (fsb *FileditchStorageBackend) EnsureDirectory(path string) error {
	return nil
}

// SaveFile uploads a file to fileditch.com using an io.Reader
func (fsb *FileditchStorageBackend) SaveFile(body io.Reader, baseStoragePath, relativePath, fileName string) (string, error) {
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
		bar := progress.NewProgressBar(int64(buf.Len()), "[Uploading to Fileditch] ", "yellow")
		reqBody = progress.NewProgressReader(&buf, bar)
		defer func() { os.Stdout.Write([]byte("\n")) }()
	}

	req, err := http.NewRequest("POST", "https://fileditch.com/upload.php", reqBody)
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
		return "", fmt.Errorf("fileditch upload failed: %d", resp.StatusCode)
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
		return "", fmt.Errorf("fileditch upload no files returned")
	}
	return result.Files[0].URL, nil
}
