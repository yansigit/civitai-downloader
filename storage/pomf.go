package storage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/yansigit/civitai-downloader/progress"
)

const (
	pomfMaxFileSizeThreshold = 900 * 1024 * 1024 // 900 MB
	defaultChunkSize         = 900 * 1024 * 1024 // 900 MB
)

// PomfStorageBackend implements file storage using pomf.lain.la
// It satisfies the StorageBackend interface.
type PomfStorageBackend struct{}

// EnsureDirectory is a no-op for PomfStorageBackend
func (psb *PomfStorageBackend) EnsureDirectory(path string) error {
	return nil
}

// uploadBytesToPomf handles the actual upload of a byte slice to Pomf.
func (psb *PomfStorageBackend) uploadBytesToPomf(content []byte, fileName string) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("files[]", fileName)
	if err != nil {
		return "", fmt.Errorf("failed to create form file for %s: %w", fileName, err)
	}
	if _, err := io.Copy(part, bytes.NewReader(content)); err != nil {
		return "", fmt.Errorf("failed to copy content for %s: %w", fileName, err)
	}
	writer.Close()

	var reqBody io.Reader = &buf
	if progress.IsTerminal() {
		bar := progress.NewProgressBar(int64(len(content)), fmt.Sprintf("[Uploading %s to Pomf] ", fileName), "yellow")
		reqBody = progress.NewProgressReader(bytes.NewReader(buf.Bytes()), bar)
		defer func() { os.Stdout.Write([]byte("\n")) }()
	}

	req, err := http.NewRequest("POST", "https://pomf.lain.la/upload.php", reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %w", fileName, err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed for %s: %w", fileName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pomf upload failed for %s: status %d, body: %s", fileName, resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Success bool `json:"success"`
		Files   []struct {
			URL string `json:"url"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode pomf response for %s: %w", fileName, err)
	}
	if !result.Success || len(result.Files) == 0 {
		return "", fmt.Errorf("pomf upload reported no files returned for %s (success: %t)", fileName, result.Success)
	}
	return result.Files[0].URL, nil
}

// SaveFile uploads content to pomf.lain.la.
// If the content is larger than pomfMaxFileSizeThreshold, it splits it into chunks
// and logs each chunk using storage.LogAssociatedFile.
// modelFileID is the version_id from archived_models.
// fileCategoryForChunks is used as the base for FileCategory in LogAssociatedFile (e.g., "model" -> "model_chunk").
// Note: This signature change might require adjustments to the StorageBackend interface
// and its other implementations. The baseStoragePath and relativePath parameters were removed
// as they are not directly used by this Pomf implementation.
func (psb *PomfStorageBackend) SaveFile(db *sql.DB, modelFileID int64, fileCategoryForChunks string, content []byte, baseStoragePath, relativePath, fileName string) (string, error) {
	// baseStoragePath and relativePath are ignored by PomfStorageBackend
	if len(content) > pomfMaxFileSizeThreshold {
		numChunks := (len(content) + defaultChunkSize - 1) / defaultChunkSize
		var uploadedChunkURLs []string
		var firstChunkURL string

		fmt.Printf("File %s is large (%d bytes), splitting into %d chunks of max %d bytes.\n", fileName, len(content), numChunks, defaultChunkSize)

		for i := 0; i < numChunks; i++ {
			start := i * defaultChunkSize
			end := (i + 1) * defaultChunkSize
			if end > len(content) {
				end = len(content)
			}
			chunkContent := content[start:end]
			chunkFilename := fmt.Sprintf("%s.part%d", fileName, i+1)

			fmt.Printf("Uploading chunk %s (%d/%d, %d bytes)...\n", chunkFilename, i+1, numChunks, len(chunkContent))
			pomfURL, err := psb.uploadBytesToPomf(chunkContent, chunkFilename)
			if err != nil {
				fmt.Printf("ERROR: Failed to upload chunk %s for %s: %v\n", chunkFilename, fileName, err)
				return "", fmt.Errorf("failed to upload chunk %d (%s) for %s: %w", i+1, chunkFilename, fileName, err)
			}
			uploadedChunkURLs = append(uploadedChunkURLs, pomfURL)
			if i == 0 {
				firstChunkURL = pomfURL
			}

			assocInfo := AssociatedFileInfo{
				ArchivedModelVersionID: modelFileID,
				FileCategory:           fileCategoryForChunks + "_chunk",
				FileIdentifier:         pomfURL,
				OriginalFilename:       chunkFilename,
				MimeType:               "application/octet-stream",
				OrderIndex:             i + 1,
			}
			err = LogAssociatedFile(db, assocInfo)
			if err != nil {
				fmt.Printf("ERROR: Failed to log associated file for chunk %s (URL: %s): %v\n", chunkFilename, pomfURL, err)
				return "", fmt.Errorf("failed to log database record for chunk %s (URL: %s) for %s: %w", chunkFilename, pomfURL, fileName, err)
			}
			fmt.Printf("Successfully uploaded and logged chunk %s (URL: %s)\n", chunkFilename, pomfURL)
		}

		if len(uploadedChunkURLs) > 0 {
			fmt.Printf("All %d chunks for %s uploaded successfully. First chunk URL: %s\n", numChunks, fileName, firstChunkURL)
			return firstChunkURL + " (multipart_pomf)", nil
		}
		return "", fmt.Errorf("no chunks were successfully uploaded for %s despite splitting logic", fileName)

	} else {
		fmt.Printf("File %s is small enough (%d bytes), uploading as single part.\n", fileName, len(content))
		pomfURL, err := psb.uploadBytesToPomf(content, fileName)
		if err != nil {
			fmt.Printf("ERROR: Failed to upload single file %s: %v\n", fileName, err)
			return "", fmt.Errorf("failed to upload single file %s: %w", fileName, err)
		}
		fmt.Printf("Successfully uploaded single file %s (URL: %s)\n", fileName, pomfURL)
		return pomfURL, nil
	}
}
