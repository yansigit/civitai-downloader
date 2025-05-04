package downloader

import (
	"bytes" // Added
	"database/sql"
	"encoding/json" // Added
	"fmt"
	"io"
	"log" // Added
	"net/http"
	"path"
	"path/filepath"
	"regexp" // Added
	"strings"

	"github.com/schollz/progressbar/v3"
	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/storage" // Added
)

// ModelVersion represents the model version information from the Civitai API
type ModelVersion struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	BaseModel     string   `json:"baseModel"`     // Added
	BaseModelType string   `json:"baseModelType"` // Added
	PublishedAt   string   `json:"publishedAt"`   // Added
	Files         []File   `json:"files"`
	Images        []Image  `json:"images"`      // Assuming Image struct is defined below or elsewhere
	Description   *string  `json:"description"` // Changed to pointer
	DownloadURL   string   `json:"downloadUrl"`
	TrainedWords  []string `json:"trainedWords"` // Added
}

// File represents a file associated with the model version
type File struct {
	ID          int64        `json:"id"`
	SizeKB      float64      `json:"sizeKB"`
	Name        string       `json:"name"`
	Type        string       `json:"type"`
	DownloadURL string       `json:"downloadUrl"`
	Metadata    FileMetadata `json:"metadata"` // Added metadata field
}

// Image represents an image associated with the model version
type Image struct {
	URL    string `json:"url"`
	Type   string `json:"type"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// --- Structs needed for GetModelVersions response ---

// ModelDetail represents the full response when fetching a single model by ID.
type ModelDetail struct {
	ID            int64          `json:"id"`
	Name          string         `json:"name"`
	Description   *string        `json:"description"`
	Type          string         `json:"type"`
	Tags          []string       `json:"tags"`
	Creator       Creator        `json:"creator"`
	ModelVersions []ModelVersion `json:"modelVersions"`
	// Add other top-level fields as needed
}

// Creator represents the model author.
type Creator struct {
	Username string  `json:"username"`
	Image    *string `json:"image"` // URL to avatar, make optional
}

// FileMetadata contains format, size, and precision info.
// Added here as it was part of the removed structs in civitai_api.go
// and might be needed if File struct is expanded later.
type FileMetadata struct {
	Format string `json:"format"` // e.g., "SafeTensor", "PickleTensor"
	Size   string `json:"size"`   // e.g., "full", "pruned"
	FP     string `json:"fp"`
}

const (
	APIModelVersions = "https://civitai.com/api/v1/model-versions/"
	APIModels        = "https://civitai.com/api/v1/models/"
)

// Helper function to sanitize filenames
var illegalChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
var trailingChars = regexp.MustCompile(`[ .]+$`)

func SanitizeFilename(name string) string {
	// Replace illegal characters with underscore
	sanitized := illegalChars.ReplaceAllString(name, "_")
	// Remove trailing dots and spaces
	sanitized = trailingChars.ReplaceAllString(sanitized, "")
	// Limit length if necessary (optional)
	// const maxLength = 200
	// if len(sanitized) > maxLength {
	//     sanitized = sanitized[:maxLength]
	// }
	// Ensure filename is not empty or just "." or ".."
	if sanitized == "" || sanitized == "." || sanitized == ".." {
		return "downloaded_file" // Provide a default name
	}
	return sanitized
}

// DownloadAll downloads all available files for a given model ID using the provided storage backend
// DownloadAll downloads all available files for a given model ID using the provided storage backend
func DownloadAll(file File, baseStoragePath string, model Model, modelVersion ModelVersion, config *config.Config, db *sql.DB, storageBackend storage.StorageBackend, dryrun bool) (string, error) {
	exists, err := storage.CheckModelVersionExists(db, modelVersion.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check database for model version: %w", err)
	}
	if exists {
		log.Printf("Model version %d (%s) is already downloaded. Skipping.", modelVersion.ID, modelVersion.Name)
		return "", nil
	}

	if dryrun {
		log.Printf("Dryrun: Would download files for model %s version %s", model.Name, modelVersion.Name)
		return SanitizeFilename(file.Name), nil
	}

	// Removed unused modelID variable

	downloadURL := modelVersion.DownloadURL + "?type=" + file.Type + "&format=" + file.Metadata.Format + "&token=" + config.Civitai.Token
	log.Printf("Attempting download from: %s", downloadURL)

	// Perform HTTP GET request
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s: %w", downloadURL, err)
	}
	if strings.Contains(downloadURL, "civitai.com") {
		req.Header.Set("Authorization", "Bearer "+config.Civitai.Token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download from %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Error response body from %s: %s", downloadURL, string(bodyBytes))
		return "", fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, downloadURL)
	}

	// Determine filename (handle content-disposition)
	finalFileName := file.Name // Default filename from API response
	header := resp.Header.Get("content-disposition")
	if header != "" {
		parts := strings.Split(header, "filename=")
		if len(parts) > 1 {
			// More robust parsing might be needed for complex headers
			potentialName := strings.Trim(parts[1], "\" ")
			if potentialName != "" {
				finalFileName = potentialName
			}
		}
	}
	finalFileName = SanitizeFilename(finalFileName) // Sanitize the final chosen name

	// Determine destination directory
	// This path is primarily for LocalStorageBackend. RemoteStorageBackend might ignore it or use it differently internally.
	// Construct raw relative path using forward slashes
	relativePath := path.Join(model.Type, model.Name, modelVersion.Name)

	// Use storage backend to save the file with progress bar
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

	// Save using the backend - pass baseStoragePath, relativePath, finalFileName
	// CORRECTED CALL
	savedFilePathOrID, err := storageBackend.SaveFile(&progressReader, baseStoragePath, relativePath, finalFileName)
	if err != nil {
		fmt.Println()
		log.Printf("Failed to save file %s using storage backend: %v", finalFileName, err)
		return "", fmt.Errorf("failed to save model file via backend: %w", err)
	}
	fmt.Println()

	log.Printf("Successfully saved main file: %s (Identifier: %s)", finalFileName, savedFilePathOrID)

	// --- Save Images ---
	baseName := strings.TrimSuffix(finalFileName, filepath.Ext(finalFileName)) // Use sanitized name
	for i, image := range modelVersion.Images {
		var imgExt string
		if image.Type == "image" {
			// Try to guess extension from URL, default to .png
			imgExt = ".png"
			if urlExt := filepath.Ext(image.URL); urlExt != "" && len(urlExt) <= 5 { // Basic check for valid extension
				imgExt = urlExt
			}
		} else if image.Type == "video" {
			imgExt = ".mp4" // Assume mp4 for video previews
		} else {
			log.Printf("Skipping unknown image type: %s for URL: %s", image.Type, image.URL)
			continue // Skip unknown types
		}

		imgFileName := SanitizeFilename(fmt.Sprintf("%s.%d.preview%s", baseName, i, imgExt))
		log.Printf("Attempting download for image: %s", image.URL)

		// Use http.Get for simplicity, add auth header if required for image URLs
		imgResp, err := http.Get(image.URL)
		if err != nil {
			log.Printf("Warning: failed to download image %s: %v", image.URL, err)
			continue
		}
		defer imgResp.Body.Close() // Close body inside the loop

		if imgResp.StatusCode != http.StatusOK {
			log.Printf("Warning: failed to download image %s, status: %s", image.URL, imgResp.Status)
			continue
		}

		// Save image using the storage backend - pass same paths
		// CORRECTED CALL
		_, err = storageBackend.SaveFile(imgResp.Body, baseStoragePath, relativePath, imgFileName)
		if err != nil {
			log.Printf("Warning: failed to save image %s: %v", imgFileName, err)
		} else {
			log.Printf("Successfully saved image: %s", imgFileName)
		}
	}

	// --- Save Metadata ---
	metadataFileName := fmt.Sprintf("%s.civitai.info", baseName)
	metadataBytes, err := json.MarshalIndent(modelVersion, "", "  ")
	if err != nil {
		log.Printf("Warning: failed to marshal metadata for %s: %v", baseName, err)
	} else {
		metadataReader := bytes.NewReader(metadataBytes)
		// Save metadata using the storage backend - pass same paths
		// CORRECTED CALL
		_, err = storageBackend.SaveFile(metadataReader, baseStoragePath, relativePath, metadataFileName)
		if err != nil {
			log.Printf("Warning: failed to save metadata file %s: %v", metadataFileName, err)
		} else {
			log.Printf("Successfully saved metadata: %s", metadataFileName)
		}
	}

	// --- Save Description ---
	if modelVersion.Description != nil && *modelVersion.Description != "" {
		descFileName := fmt.Sprintf("%s.description.txt", baseName)
		descriptionReader := strings.NewReader(*modelVersion.Description)
		// Save description using the storage backend - pass same paths
		// CORRECTED CALL
		_, err = storageBackend.SaveFile(descriptionReader, baseStoragePath, relativePath, descFileName)
		if err != nil {
			log.Printf("Warning: failed to save description file %s: %v", descFileName, err)
		} else {
			log.Printf("Successfully saved description: %s", descFileName)
		}
	}

	// Return the path/ID of the main file (which is now the full path/key)
	return savedFilePathOrID, nil
}

// GetModelID retrieves the model ID from the URL
func GetModelID(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch model page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	parts := url[len("https://civitai.com/models/"):]
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid URL format")
	}
	modelID := parts
	return modelID, nil
}

// Removed duplicated API structs and function definitions.
// These should be defined solely in downloader/civitai_api.go
