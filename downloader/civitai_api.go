package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yansigit/civitai-downloader/logger"
)

// _makeCivitaiRequest handles the common logic for making a GET request to a Civitai API endpoint.
func _makeCivitaiRequest(apiURL string, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", apiURL, err)
	}
	req.Header.Set("User-Agent", "StabilityMatrix") // Consistent User-Agent
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	logger.Debug("Requesting URL: <cyan>%s</cyan>", apiURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make GET request to %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body) // Read body once
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", apiURL, err)
	}
	logger.Debug("Response length: <yellow>%d</yellow>, Status: <yellow>%d</yellow>", len(body), resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		logger.Error("Request failed for URL: <red>%s</red>", apiURL)
		logger.Debug("Status Code: <red>%d</red>, Response Body: %s", resp.StatusCode, string(body))
		// Return an error that includes the status code, but not the body to avoid massive log spam if body is huge
		return nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, apiURL)
	}

	if len(body) == 0 {
		logger.Warning("Received empty response body from <yellow>%s</yellow> despite 200 OK", apiURL)
		// Consider if this is an error or expected. For now, return empty bytes.
		return []byte{}, nil
	}

	return body, nil
}

func GetModels(request CivitModelsRequest, token string) ([]Model, Metadata, error) {
	apiURL := "https://civitai.com/api/v1/models"
	params := url.Values{}

	// Determine if additional filtering logic is needed
	usePostFetchFiltering := len(request.BaseModels) > 0

	if request.Limit > 0 {
		params.Add("limit", strconv.Itoa(request.Limit))
	}
	if request.Page > 0 {
		params.Add("page", strconv.Itoa(request.Page))
	}
	if request.Sort != "" {
		params.Add("sort", request.Sort)
	}
	if len(request.Types) > 0 {
		params.Add("types", strings.Join(request.Types, ","))
	}
	if request.Query != "" {
		params.Add("query", request.Query)
	}
	if request.Cursor != "" {
		params.Add("cursor", request.Cursor)
	}
	if request.Tag != "" {
		params.Add("tag", request.Tag)
	}
	if request.Username != "" {
		params.Add("username", request.Username)
	}
	if request.Period != "" {
		params.Add("period", request.Period)
	}
	if request.Rating > 0 {
		params.Add("rating", strconv.Itoa(request.Rating))
	}
	if request.Favorites {
		params.Add("favorites", "true")
	}
	if request.Hidden {
		params.Add("hidden", "true")
	}
	if request.PrimaryFileOnly {
		params.Add("primaryFileOnly", "true")
	}
	if request.AllowDerivatives {
		params.Add("allowDerivatives", "true")
	}
	if request.AllowDifferentLicenses {
		params.Add("allowDifferentLicenses", "true")
	}
	if request.AllowCommercialUse != "" {
		params.Add("allowCommercialUse", request.AllowCommercialUse)
	}
	if request.Nsfw != "" {
		params.Add("nsfw", request.Nsfw)
	}
	if len(request.BaseModels) > 0 {
		params.Add("baseModels", strings.Join(request.BaseModels, ","))
	}
	if request.Ids != "" {
		params.Add("ids", request.Ids)
	}

	if usePostFetchFiltering {
		logger.Debug("Post-fetch filtering enabled for BaseModels: <cyan>%v</cyan>", request.BaseModels)
	}

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	body, err := _makeCivitaiRequest(fullURL, token)
	if err != nil {
		// Error includes URL and status code from helper
		return nil, Metadata{}, fmt.Errorf("failed to fetch models: %w", err)
	}

	if len(body) == 0 {
		logger.Warning("GetModels received empty body, returning empty result.")
		return []Model{}, Metadata{}, nil
	}

	// Save the response body to a file (optional, useful for debugging)
	// Consider making this conditional based on a debug flag
	// fileName := "response.json"
	// err = os.WriteFile(fileName, body, 0644)
	// if err != nil {
	// logger.Warning("Failed to save response to file '%s': %v", fileName, err)
	// } else {
	// logger.Debug("Response saved to file: %s", fileName)
	// }

	// Log the API response body for debugging (use the body we already read)
	// Limit logging length if necessary to avoid flooding logs
	// logBody := string(body)
	// if len(logBody) > 1000 { // Log first 1000 chars
	// logBody = logBody[:1000] + "..."
	// }
	// logger.Debug("API Response Body (preview): %s", logBody)

	// Validate JSON response (use the body we already read)
	if !json.Valid(body) {
		logger.Error("Invalid JSON response received from URL: <red>%s</red>", fullURL)
		// Log more of the body if possible, carefully handling potential large sizes
		logger.Debug("Invalid JSON content: %s", string(body)) // Be cautious with large responses
		return nil, Metadata{}, fmt.Errorf("invalid JSON response")
	}

	// Parse the JSON content (use the body we already read)
	var result struct {
		Items    []Model  `json:"items"` // Corrected field name based on observed response.json
		Metadata Metadata `json:"metadata"`
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		logger.Error("Failed to decode JSON response from URL: <red>%s</red>", fullURL)
		logger.Debug("JSON decode error: <red>%v</red>. Body: %s", err, string(body)) // Log error and body
		return nil, Metadata{}, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Info("Fetched <yellow>%d</yellow> models. Metadata: %+v", len(result.Items), result.Metadata)

	// Print a summary of the fetched models
	for _, model := range result.Items {
		for _, version := range model.ModelVersions {
			logger.Debug("Model Name: <cyan>%s</cyan>, Version: <yellow>%s</yellow>, Type: <yellow>%s</yellow>, BaseModel: <yellow>%s</yellow>", model.Name, version.Name, model.Type, version.BaseModel)
		}
	}

	// Apply post-fetch filtering if needed
	if usePostFetchFiltering && len(request.BaseModels) > 0 {
		filteredModels := []Model{}
		baseModelSet := make(map[string]struct{}) // Use a set for efficient lookup
		for _, bm := range request.BaseModels {
			baseModelSet[bm] = struct{}{}
		}

		for _, model := range result.Items {
			// Check if the model's baseModel exists in the requested set
			// Ensure `model.BaseModel` field exists and is populated correctly in the Model struct
			for _, version := range model.ModelVersions {
				if _, exists := baseModelSet[version.BaseModel]; exists {
					filteredModels = append(filteredModels, model)
					break // No need to check other versions if one matches
				}
			}
		}
		logger.Debug("Filtered models count: <yellow>%d</yellow> (based on BaseModels: <cyan>%v</cyan>)", len(filteredModels), request.BaseModels)
		// Update metadata? The API metadata reflects the unfiltered result.
		// Decide if the returned metadata should reflect the filtered count or original.
		// For simplicity, returning original metadata.
		return filteredModels, result.Metadata, nil
	}

	return result.Items, result.Metadata, nil
}

// GetModelVersions fetches the details of a specific model, including all its versions.
// It uses the ModelDetail, ModelVersion, File, etc. structs defined likely in downloader.go or types.go
func GetModelVersions(modelID int64, token string) ([]ModelVersion, error) {
	// The endpoint usually returns the full model details, including versions
	apiURL := fmt.Sprintf("https://civitai.com/api/v1/models/%d", modelID)

	body, err := _makeCivitaiRequest(apiURL, token)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch model %d details: %w", modelID, err)
	}

	if len(body) == 0 {
		logger.Warning("GetModelVersions received empty body for model ID <yellow>%d</yellow>", modelID)
		return nil, fmt.Errorf("received empty response body fetching model %d versions", modelID)
	}

	// Unmarshal the full model detail response
	var modelDetail ModelDetail
	err = json.Unmarshal(body, &modelDetail)
	if err != nil {
		logger.Error("Failed to decode model <yellow>%d</yellow> details response. URL: <red>%s</red>", modelID, apiURL)
		logger.Debug("Decode error: <red>%v</red>. Body: %s", err, string(body))
		return nil, fmt.Errorf("failed to decode model %d details response: %w", modelID, err)
	}

	logger.Info("Successfully fetched <yellow>%d</yellow> versions for model <cyan>%d</cyan> (<yellow>%s</yellow>)", len(modelDetail.ModelVersions), modelID, modelDetail.Name)
	return modelDetail.ModelVersions, nil
}
