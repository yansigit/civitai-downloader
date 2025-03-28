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
	ID          int64   `json:"id"`
	ModelID     int64   `json:"modelId"`
	Name        string  `json:"name"`
	Files       []File  `json:"files"`
	Images      []Image `json:"images"`
	Description string  `json:"description"`
	DownloadURL string  `json:"downloadUrl"`
}

// File represents a file associated with the model version
type File struct {
	ID          int64   `json:"id"`
	SizeKB      float64 `json:"sizeKB"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	DownloadURL string  `json:"downloadUrl"`
}

// Image represents an image associated with the model version
type Image struct {
	URL    string `json:"url"`
	Type   string `json:"type"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

const (
	APIModelVersions = "https://civitai.com/api/v1/model-versions/"
)

// DownloadFile downloads a single file from the given URL to the specified path and returns the saved filename
func DownloadFile(outputPath, url, modelVersionId string) (string, error) {
	outputDir := filepath.Dir(outputPath)
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
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
func DownloadAll(modelType, baseModelPath, modelID string, config *config.Config) error {
	modelURL := fmt.Sprintf("%s%s", APIModelVersions, modelID)
	resp, err := http.Get(modelURL)
	if err != nil {
		return fmt.Errorf("failed to fetch model version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch model version. Status code: %d", resp.StatusCode)
	}

	var modelVersion ModelVersion
	err = json.NewDecoder(resp.Body).Decode(&modelVersion)
	if err != nil {
		return fmt.Errorf("failed to decode model version response: %w", err)
	}

	// Create a subdirectory based on modelType
	dir := filepath.Join(baseModelPath, modelType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create model type directory: %w", err)
	}

	modelVersion.DownloadURL = modelVersion.DownloadURL + "?token=" + config.Civitai.Token

	outputPath, err := DownloadFile(filepath.Join(dir, filepath.Base(baseModelPath)), modelVersion.DownloadURL, modelID)
	if err != nil {
		fmt.Println("Download failed from URL:", modelVersion.DownloadURL)
		return fmt.Errorf("failed to download model file: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))

	for _, image := range modelVersion.Images {
		if image.Type == "image" {
			imgPath := fmt.Sprintf("%s.preview.png", filepath.Join(filepath.Dir(outputPath), baseName))
			if _, err := DownloadFile(imgPath, image.URL, modelID); err != nil {
				return fmt.Errorf("failed to download image: %w", err)
			}
		} else if image.Type == "video" {
			imgPath := fmt.Sprintf("%s.preview.mp4", filepath.Join(filepath.Dir(outputPath), baseName))
			if _, err := DownloadFile(imgPath, image.URL, modelID); err != nil {
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

	if modelVersion.Description != "" {
		descPath := fmt.Sprintf("%s.description.txt", filepath.Join(filepath.Dir(outputPath), baseName))
		if err := os.WriteFile(descPath, []byte(modelVersion.Description), 0644); err != nil {
			return fmt.Errorf("failed to save description: %w", err)
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
