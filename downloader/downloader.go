package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/schollz/progressbar/v3"
	"github.com/yansigit/civitai-downloader/config"
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
	FP     string `json:"fp"`     // e.g., "fp16", "fp32"
}

const (
	APIModelVersions = "https://civitai.com/api/v1/model-versions/"
	APIModels        = "https://civitai.com/api/v1/models/" // Added base models endpoint
)

// DownloadFile downloads a single file from the given URL to the specified path and returns the saved filename
func DownloadFile(outputPath, url, modelVersionId, token string) (string, error) {
	outputDir := filepath.Dir(outputPath)
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	if strings.Contains(url, "civitai.com") {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to download file: %w, response body: %s", err, string(body))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, response body: %s", resp.StatusCode, string(body))
	}

	header := resp.Header.Get("content-disposition")
	if header != "" {
		parts := strings.Split(header, "filename=")
		if len(parts) > 1 {
			outputPath = filepath.Join(filepath.Dir(outputPath), strings.Trim(parts[1], "\""))
		}
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	bar := progressbar.NewOptions(
		int(resp.ContentLength),
		progressbar.OptionSetWidth(15),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetDescription("[Downloading] "),
		progressbar.OptionSetTheme(
			progressbar.Theme{
				Saucer:        "[green]=[reset]",
				SaucerHead:    "[green]>[reset]",
				SaucerPadding: " ",
				BarStart:      "|",
				BarEnd:        "|",
			}),
	)

	_, err = io.Copy(io.MultiWriter(file, bar), resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write to file: %w", err)
	}

	return outputPath, nil
}

// DownloadAll downloads all available files for a given model ID
func DownloadAll(file File, baseModelPath string, model Model, modelVersion ModelVersion, config *config.Config) error {
	modelID := fmt.Sprintf("%d", modelVersion.ID)
	// modelURL := fmt.Sprintf("%s%s", APIModelVersions, modelID)
	// resp, err := http.Get(modelURL)
	// if err != nil {
	// 	return fmt.Errorf("failed to fetch model version: %w", err)
	// }
	// defer resp.Body.Close()

	// if resp.StatusCode != http.StatusOK {
	// 	return fmt.Errorf("failed to fetch model version. Status code: %d", resp.StatusCode)
	// }

	// var modelVersion ModelVersion
	// err = json.NewDecoder(resp.Body).Decode(&modelVersion)
	// if err != nil {
	// 	return fmt.Errorf("failed to decode model version response: %w", err)
	// }

	// Create a subdirectory based on modelType
	dir := filepath.Join(baseModelPath, model.Type)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create model type directory: %w", err)
	}

	// modelVersion.DownloadURL = modelVersion.DownloadURL + "?token=" + config.Civitai.Token
	modelVersion.DownloadURL = modelVersion.DownloadURL + "?type=" + file.Type + "&format=" + file.Metadata.Format + "&token=" + config.Civitai.Token

	outputPath, err := DownloadFile(filepath.Join(dir, filepath.Base(baseModelPath)), modelVersion.DownloadURL, modelID, config.Civitai.Token)
	if err != nil {
		fmt.Println("Download failed from URL:", modelVersion.DownloadURL)
		return fmt.Errorf("failed to download model file: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))

	for _, image := range modelVersion.Images {
		if image.Type == "image" {
			imgPath := fmt.Sprintf("%s.preview.png", filepath.Join(filepath.Dir(outputPath), baseName))
			if _, err := DownloadFile(imgPath, image.URL, modelID, config.Civitai.Token); err != nil {
				return fmt.Errorf("failed to download image: %w", err)
			}
		} else if image.Type == "video" {
			imgPath := fmt.Sprintf("%s.preview.mp4", filepath.Join(filepath.Dir(outputPath), baseName))
			if _, err := DownloadFile(imgPath, image.URL, modelID, config.Civitai.Token); err != nil {
				return fmt.Errorf("failed to download image: %w", err)
			}
		}
	}

	metadataPath := fmt.Sprintf("%s.civitai.info", filepath.Join(filepath.Dir(outputPath), baseName))
	metadata, err := json.MarshalIndent(modelVersion, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metadataPath, metadata, 0644); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	// Handle optional description pointer
	if modelVersion.Description != nil && *modelVersion.Description != "" {
		descPath := fmt.Sprintf("%s.description.txt", filepath.Join(filepath.Dir(outputPath), baseName))
		// Dereference the pointer to get the string value
		if err := os.WriteFile(descPath, []byte(*modelVersion.Description), 0644); err != nil {
			// Log warning instead of failing the whole download?
			fmt.Printf("Warning: failed to save description: %v\n", err)
			// return fmt.Errorf("failed to save description: %w", err)
		}
	}

	fmt.Printf("Successfully downloaded model files to %s\n", filepath.Dir(outputPath))
	return nil
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
