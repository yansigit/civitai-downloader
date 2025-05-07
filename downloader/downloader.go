package downloader

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/logger"
	"github.com/yansigit/civitai-downloader/progress"
	"github.com/yansigit/civitai-downloader/storage"
)

// ModelVersion represents the model version information from the Civitai API
type ModelVersion struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	BaseModel     string   `json:"baseModel"`
	BaseModelType string   `json:"baseModelType"`
	PublishedAt   string   `json:"publishedAt"`
	Files         []File   `json:"files"`
	Images        []Image  `json:"images"`
	Description   *string  `json:"description"`
	DownloadURL   string   `json:"downloadUrl"`
	TrainedWords  []string `json:"trainedWords"`
}

// File represents a file associated with the model version
type File struct {
	ID          int64        `json:"id"`
	SizeKB      float64      `json:"sizeKB"`
	Name        string       `json:"name"`
	Type        string       `json:"type"`
	DownloadURL string       `json:"downloadUrl"`
	Metadata    FileMetadata `json:"metadata"`
	Primary     bool         `json:"primary"`
}

// Image represents an image associated with the model version
type Image struct {
	URL    string `json:"url"`
	Type   string `json:"type"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// FileMetadata contains format, size, and precision info
type FileMetadata struct {
	Format string `json:"format"`
	Size   string `json:"size"`
	FP     string `json:"fp"`
}

const (
	APIModelVersions = "https://civitai.com/api/v1/model-versions/"
	APIModels        = "https://civitai.com/api/v1/models/"
)

// DownloadAll downloads the main file for a model version and returns its path/ID,
// along with raw metadata and preview content if enabled in config.
// It uses the provided storage backend ONLY for the main file.
// Metadata/preview saving is handled by the caller (archiver) based on storage type.
func DownloadAll(file File, baseStoragePath string, model Model, modelVersion ModelVersion, config *config.Config, db *sql.DB, storageBackend storage.StorageBackend, dryrun bool) (string, []byte, [][]byte, error) {
	exists, err := storage.CheckModelVersionExists(db, modelVersion.ID)
	if err != nil {
		// Correct return for error: path/ID, metadata bytes, preview bytes, error
		return "", nil, nil, fmt.Errorf("failed to check database for model version: %w", err)
	}
	if exists {
		logger.Info("Model version <yellow>%d</yellow> (<cyan>%s</cyan>) is already downloaded. Skipping.", modelVersion.ID, modelVersion.Name)
		// Correct return for skipped: path/ID, metadata bytes, preview bytes, error
		return "", nil, nil, nil
	}
	if !model.Nsfw && config.Civitai.NSFWOnly {
		logger.Info("<pink>Skipping non-NSFW model</pink>: <yellow>%s</yellow> (ID: <cyan>%d</cyan>)", model.Name, model.ID)
		return "", nil, nil, nil
	}
	// Skip large files (over configured limit)
	// if file.SizeKB > float64(config.Civitai.MaxFileSizeMB*1024) {
	// 	logger.Warning("File <yellow>%s</yellow> is too large (<red>%.2f MB</red>). <pink>Skipping files over %dMB.</pink>", file.Name, file.SizeKB/1024, config.Civitai.MaxFileSizeMB)
	// 	return "", nil, nil, nil
	// }
	// Skip models that are not SafeTensor or PickleTensor format
	if file.Metadata.Format != "SafeTensor" && file.Metadata.Format != "PickleTensor" {
		logger.Warning("File <yellow>%s</yellow> has unsupported format: <red>%s</red>. Skipping non-SafeTensor/PickleTensor files.", file.Name, file.Metadata.Format)
		return "", nil, nil, nil
	}

	if dryrun {
		logger.Info("Dryrun: Would download files for model <cyan>%s</cyan> version <yellow>%s</yellow>", model.Name, modelVersion.Name)
		// Correct return for dryrun: path/ID, metadata bytes, preview bytes, error
		return SanitizeFilename(file.Name), nil, nil, nil
	}

	downloadURL := modelVersion.DownloadURL + "?type=" + file.Type + "&format=" + file.Metadata.Format + "&token=" + config.Civitai.Token
	logger.Info("Attempting download from: <cyan>%s</cyan>", downloadURL)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to create request for %s: %w", downloadURL, err)
	}
	if strings.Contains(downloadURL, "civitai.com") {
		req.Header.Set("Authorization", "Bearer "+config.Civitai.Token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to download from %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.Error("Error response from <yellow>%s</yellow>: <red>%s</red>", downloadURL, string(bodyBytes))
		return "", nil, nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, downloadURL)
	}

	finalFileName := SanitizeFilename(file.Name)
	relativePath := filepath.Join(model.Type, model.Name, modelVersion.Name)

	// Create a progress bar for the download
	var progressReader io.Reader = resp.Body
	if progress.IsTerminal() {
		bar := progress.NewProgressBar(resp.ContentLength, "[Downloading] ", "green")
		progressReader = progress.NewProgressReader(resp.Body, bar)
	}

	// Read all content from progressReader into a byte slice
	// This is necessary because the new StorageBackend.SaveFile interface expects []byte
	fileContent, err := io.ReadAll(progressReader)
	if err != nil {
		fmt.Println() // Ensure progress bar newline if it was active
		logger.Error("Failed to read downloaded content for <yellow>%s</yellow>: %v", finalFileName, err)
		return "", nil, nil, fmt.Errorf("failed to read downloaded content for %s: %w", finalFileName, err)
	}
	// The progress bar, if active, will complete upon io.ReadAll. A newline is good.
	if progress.IsTerminal() {
		fmt.Println()
	}

	// Save the file using the storage backend
	// savedFilePathOrID, err := storageBackend.SaveFile(progressReader, baseStoragePath, relativePath, finalFileName)
	// Updated call to match new StorageBackend.SaveFile signature:
	savedFilePathOrID, err := storageBackend.SaveFile(db, modelVersion.ID, "model", fileContent, baseStoragePath, relativePath, finalFileName)
	if err != nil {
		// fmt.Println() // Newline handled above after ReadAll or if no progress bar
		logger.Error("Failed to save file <yellow>%s</yellow> using storage backend: %v", finalFileName, err)
		return "", nil, nil, fmt.Errorf("failed to save model file via backend: %w", err)
	}
	// fmt.Println() // Newline handled above

	logger.Info("Successfully saved main file: <green>%s</green> (Identifier: <yellow>%s</yellow>)", finalFileName, savedFilePathOrID)

	var metadataContent []byte
	var previewContents [][]byte

	// Prepare Previews if enabled
	if config.Storage.SavePreviews && len(modelVersion.Images) > 0 {
		previewContents = make([][]byte, 0, len(modelVersion.Images)) // Pre-allocate slice
		logger.Debug("Processing preview images...")

		// Process preview images
		for i, image := range modelVersion.Images {
			if !strings.HasPrefix(image.URL, "http") {
				image.URL = "https://civitai.com" + image.URL
			}

			logger.Info("Downloading preview image <yellow>%d/%d</yellow>: <cyan>%s</cyan>", i+1, len(modelVersion.Images), image.URL)

			// Download the image
			resp, err := http.Get(image.URL)
			if err != nil {
				logger.Error("Failed to download preview image: <red>%v</red>", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				logger.Warning("Failed to download preview image: status code <red>%d</red>", resp.StatusCode)
				continue
			}

			// Read the image data
			imageData, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Error("Failed to read preview image data: <red>%v</red>", err)
				continue
			}

			previewContents = append(previewContents, imageData)
		}
	}

	// Save metadata as JSON
	if config.Storage.SaveMetadata {
		metadataJSON, err := json.MarshalIndent(modelVersion, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal metadata: <red>%v</red>", err)
		} else {
			metadataContent = metadataJSON
			logger.Debug("Successfully marshalled metadata for: <cyan>%s</cyan>", finalFileName)
		}
	}

	// Return the main file path/ID, metadata bytes, preview bytes, and nil error
	return savedFilePathOrID, metadataContent, previewContents, nil
}
