package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Model represents a model from the Civitai API
type Model struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	BaseModel string `json:"baseModel"`
}

// Metadata represents pagination metadata from the Civitai API
type Metadata struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	Page    int  `json:"page"`
	HasNext bool `json:"hasNext"`
}

type CivitModelsRequest struct {
	Limit                  int      `json:"limit,omitempty"`
	Page                   int      `json:"page,omitempty"`
	Query                  string   `json:"query,omitempty"`
	Tag                    string   `json:"tag,omitempty"`
	Username               string   `json:"username,omitempty"`
	Types                  []string `json:"types,omitempty"` // Enum: Checkpoint, TextualInversion, Hypernetwork, etc.
	Sort                   string   `json:"sort,omitempty"`
	Period                 string   `json:"period,omitempty"`
	Rating                 int      `json:"rating,omitempty"`
	Favorites              bool     `json:"favorites,omitempty"`
	Hidden                 bool     `json:"hidden,omitempty"`
	PrimaryFileOnly        bool     `json:"primaryFileOnly,omitempty"`
	AllowDerivatives       bool     `json:"allowDerivatives,omitempty"`
	AllowDifferentLicenses bool     `json:"allowDifferentLicenses,omitempty"`
	AllowCommercialUse     string   `json:"allowCommercialUse,omitempty"`
	Nsfw                   string   `json:"nsfw,omitempty"`
	BaseModels             []string `json:"baseModels,omitempty"`
	Ids                    string   `json:"ids,omitempty"`
	Cursor                 string   `json:"cursor,omitempty"`
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
		log.Printf("Post-fetch filtering enabled for BaseModels: %v", request.BaseModels)
	}

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "StabilityMatrix") // Set a descriptive User-Agent
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	log.Printf("Requesting models with URL: %s", fullURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("failed to make GET request: %w", err)
	}
	defer resp.Body.Close()

	// Log the HTTP status code and headers
	log.Printf("HTTP Status Code: %d", resp.StatusCode)
	// log.Printf("Response Headers: %v", resp.Header) // Can be verbose, uncomment if needed

	// Read the response body ONCE
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("failed to read response body: %w", err)
	}
	log.Printf("Response body length: %d", len(body)) // Log length immediately after reading

	// Check for non-200 status codes AFTER reading the body
	if resp.StatusCode != http.StatusOK {
		log.Printf("Full API Request URL: %s", fullURL) // Log the URL that failed

		// Check for empty body
		if len(body) == 0 {
			log.Printf("Warning: Received empty response body from API with status %d.", resp.StatusCode)
			return nil, Metadata{}, fmt.Errorf("received empty response body from API (status: %d)", resp.StatusCode)
		}

		// Log the API response body for debugging error responses
		log.Printf("API Error Response Body: %s", string(body))
		return nil, Metadata{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Check for empty body even on success (shouldn't happen with valid JSON, but good practice)
	if len(body) == 0 {
		log.Printf("Warning: Received empty response body from API despite 200 OK status.")
		// Decide if this is an error or just needs an empty result
		return []Model{}, Metadata{}, nil // Or return an error if empty body is unexpected
		// return nil, Metadata{}, fmt.Errorf("received empty response body from API")
	}

	// Save the response body to a file (optional, useful for debugging)
	// Consider making this conditional based on a debug flag
	// fileName := "response.json"
	// err = os.WriteFile(fileName, body, 0644)
	// if err != nil {
	// 	log.Printf("Warning: failed to save response to file '%s': %v", fileName, err)
	// } else {
	// 	log.Printf("Response saved to file: %s", fileName)
	// }

	// Log the API response body for debugging (use the body we already read)
	// Limit logging length if necessary to avoid flooding logs
	// logBody := string(body)
	// if len(logBody) > 1000 { // Log first 1000 chars
	// 	logBody = logBody[:1000] + "..."
	// }
	// log.Printf("API Response Body (preview): %s", logBody)

	// Validate JSON response (use the body we already read)
	if !json.Valid(body) {
		log.Printf("Invalid JSON response received from URL: %s", fullURL)
		// Log more of the body if possible, carefully handling potential large sizes
		log.Printf("Invalid JSON content: %s", string(body)) // Be cautious with large responses
		return nil, Metadata{}, fmt.Errorf("invalid JSON response")
	}

	// Parse the JSON content (use the body we already read)
	var result struct {
		Items    []Model  `json:"items"` // Corrected field name based on observed response.json
		Metadata Metadata `json:"metadata"`
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Printf("Failed to decode JSON response from URL: %s", fullURL)
		log.Printf("JSON decode error: %v. Body: %s", err, string(body)) // Log error and body
		return nil, Metadata{}, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Printf("Fetched %d models. Metadata: %+v", len(result.Items), result.Metadata)

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
			if _, exists := baseModelSet[model.BaseModel]; exists {
				filteredModels = append(filteredModels, model)
			}
		}
		log.Printf("Filtered models count: %d (based on BaseModels: %v)", len(filteredModels), request.BaseModels)
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

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for model %d: %w", modelID, err)
	}
	req.Header.Set("User-Agent", "StabilityMatrix") // Consistent User-Agent
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	log.Printf("Requesting model details from URL: %s", apiURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch model %d details: %w", modelID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body) // Read body once
	if err != nil {
		return nil, fmt.Errorf("failed to read response body for model %d: %w", modelID, err)
	}
	log.Printf("Model %d details response length: %d", modelID, len(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("GetModelVersions failed for model %d with status %d. URL: %s", modelID, resp.StatusCode, apiURL)
		log.Printf("Error Response Body: %s", string(body))
		return nil, fmt.Errorf("unexpected status code %d fetching model %d details", resp.StatusCode, modelID)
	}

	if len(body) == 0 {
		log.Printf("GetModelVersions received empty body for model ID %d despite 200 OK", modelID)
		return nil, fmt.Errorf("received empty response body fetching model %d versions", modelID)
	}

	// Unmarshal the full model detail response
	var modelDetail ModelDetail
	err = json.Unmarshal(body, &modelDetail)
	if err != nil {
		log.Printf("Failed to decode model %d details response. URL: %s", modelID, apiURL)
		log.Printf("Decode error: %v. Body: %s", err, string(body))
		return nil, fmt.Errorf("failed to decode model %d details response: %w", modelID, err)
	}

	log.Printf("Successfully fetched %d versions for model %d (%s)", len(modelDetail.ModelVersions), modelID, modelDetail.Name)
	return modelDetail.ModelVersions, nil
}
