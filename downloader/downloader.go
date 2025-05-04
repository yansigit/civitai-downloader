package downloader

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/schollz/progressbar/v3"
	"github.com/yansigit/civitai-downloader/config"
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

// SanitizeFilename sanitizes filenames by replacing illegal characters
func SanitizeFilename(name string) string {
	illegalChars := regexp.MustCompile(`[<>:"/\\|?*-]`)
	trailingChars := regexp.MustCompile(`[ .]+$`)
	sanitized := illegalChars.ReplaceAllString(name, "_")
	sanitized = trailingChars.ReplaceAllString(sanitized, "")
	if sanitized == "" || sanitized == "." || sanitized == ".." {
		return "downloaded_file"
	}
	return sanitized
}

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
		log.Printf("Model version %d (%s) is already downloaded. Skipping.", modelVersion.ID, modelVersion.Name)
		// Correct return for skipped: path/ID, metadata bytes, preview bytes, error
		return "", nil, nil, nil
	}

	if dryrun {
		log.Printf("Dryrun: Would download files for model %s version %s", model.Name, modelVersion.Name)
		// Correct return for dryrun: path/ID, metadata bytes, preview bytes, error
		return SanitizeFilename(file.Name), nil, nil, nil
	}

	downloadURL := modelVersion.DownloadURL + "?type=" + file.Type + "&format=" + file.Metadata.Format + "&token=" + config.Civitai.Token
	log.Printf("Attempting download from: %s", downloadURL)

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
		log.Printf("Error response body from %s: %s", downloadURL, string(bodyBytes))
		return "", nil, nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, downloadURL)
	}

	finalFileName := SanitizeFilename(file.Name)
	relativePath := path.Join(model.Type, model.Name, modelVersion.Name)

	bar := progressbar.NewOptions(
		int(resp.ContentLength),
		progressbar.OptionSetDescription(fmt.Sprintf("[Downloading %s] ", finalFileName)),
		progressbar.OptionSetWidth(15),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "|",
			BarEnd:        "|",
		}),
	)
	progressReader := progressbar.NewReader(resp.Body, bar)

	// Save ONLY the main file using the storage backend
	savedFilePathOrID, err := storageBackend.SaveFile(&progressReader, baseStoragePath, relativePath, finalFileName)
	if err != nil {
		fmt.Println() // Ensure progress bar newline
		log.Printf("Failed to save file %s using storage backend: %v", finalFileName, err)
		return "", nil, nil, fmt.Errorf("failed to save model file via backend: %w", err)
	}
	fmt.Println() // Ensure progress bar newline

	log.Printf("Successfully saved main file: %s (Identifier: %s)", finalFileName, savedFilePathOrID)

	var metadataContent []byte
	var previewContents [][]byte

	// Prepare Previews if enabled
	if config.Storage.SavePreviews {
		baseName := strings.TrimSuffix(finalFileName, filepath.Ext(finalFileName))
		previewContents = make([][]byte, 0, len(modelVersion.Images)) // Pre-allocate slice
		for i, image := range modelVersion.Images {
			imgExt := ".png"
			if urlExt := filepath.Ext(image.URL); urlExt != "" && len(urlExt) <= 5 {
				imgExt = urlExt
			}
			imgFileName := SanitizeFilename(fmt.Sprintf("%s.%d.preview%s", baseName, i, imgExt)) // Used for logging only now
			log.Printf("Attempting download for preview image: %s", image.URL)

			imgResp, err := http.Get(image.URL)
			if err != nil {
				log.Printf("Warning: failed to download preview image %s: %v", image.URL, err)
				continue
			}
			defer imgResp.Body.Close()

			if imgResp.StatusCode != http.StatusOK {
				log.Printf("Warning: failed to download preview image %s, status: %s", image.URL, imgResp.Status)
				continue
			}

			// Read image bytes
			imgBytes, err := io.ReadAll(imgResp.Body)
			if err != nil {
				log.Printf("Warning: failed to read preview image bytes for %s: %v", image.URL, err)
				continue
			}
			previewContents = append(previewContents, imgBytes)
			log.Printf("Successfully read preview image %d bytes for %s", i, imgFileName)

			// Local saving is removed, archiver will handle based on storage type
		}
	}

	// Prepare Metadata if enabled
	if config.Storage.SaveMetadata {
		metadataFileName := fmt.Sprintf("%s.civitai.info", strings.TrimSuffix(finalFileName, filepath.Ext(finalFileName))) // Used for logging only now
		metadataContent, err = json.MarshalIndent(modelVersion, "", "  ")                                                  // Assign to the return variable
		if err != nil {
			log.Printf("Warning: failed to marshal metadata for %s: %v", finalFileName, err)
			metadataContent = nil // Ensure it's nil on error
		} else {
			log.Printf("Successfully marshalled metadata for: %s", metadataFileName)
			// Local saving is removed, archiver will handle based on storage type
		}
	}

	// Return the main file path/ID, metadata bytes, preview bytes, and nil error
	return savedFilePathOrID, metadataContent, previewContents, nil
}
