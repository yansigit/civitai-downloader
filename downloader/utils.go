package downloader

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// SanitizeFilename removes or replaces characters that are potentially problematic in filenames.
func SanitizeFilename(filename string) string {
	// Replace known problematic characters with underscores
	re := regexp.MustCompile(`[<>:"/\|?*]`) // Basic set, can be expanded
	sanitized := re.ReplaceAllString(filename, "_")
	// Replace multiple spaces with a single space
	sanitized = regexp.MustCompile(`\s+`).ReplaceAllString(sanitized, " ")
	// Trim leading/trailing spaces and underscores
	sanitized = strings.Trim(sanitized, " _")
	return sanitized
}

// DownloadFile fetches the content of a given URL, adding authorization if it's a Civitai URL.
func DownloadFile(url string, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}
	if token != "" && strings.Contains(url, "civitai.com") {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "StabilityMatrix") // Add User-Agent
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("DownloadFile error response body from %s: %s", url, string(bodyBytes))
		return nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, url)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", url, err)
	}
	return content, nil
}

// DownloadPreviews downloads all images specified in the Image slice.
func DownloadPreviews(images []Image, token string) ([][]byte, error) {
	var contents [][]byte
	for i, image := range images {
		log.Printf("Attempting download for preview image %d: %s", i, image.URL)
		imgBytes, err := DownloadFile(image.URL, token) // Reuse DownloadFile
		if err != nil {
			log.Printf("Warning: failed to download preview image %s: %v", image.URL, err)
			// Continue trying other images even if one fails
			continue
		}
		contents = append(contents, imgBytes)
		log.Printf("Successfully downloaded preview image %d (%d bytes)", i, len(imgBytes))
	}
	// It's not an error if some previews failed, so return nil error if loop finishes.
	return contents, nil
}

// GetMimeTypeFromExtension returns a best-guess MIME type based on a file extension.
func GetMimeTypeFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	// Add more common types as needed
	default:
		return "application/octet-stream" // Default binary type
	}
}
