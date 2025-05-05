package downloader

// Model represents a model from the Civitai API
type Model struct {
	ID            int64          `json:"id"`
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	ModelVersions []ModelVersion `json:"modelVersions"`
	Creator       Creator        `json:"creator"`
	Nsfw          bool           `json:"nsfw"`
}

// Metadata represents pagination metadata from the Civitai API
type Metadata struct {
	Total      int     `json:"total"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	Page       int     `json:"page"`
	NextCursor *string `json:"nextCursor"`
	NextPage   *string `json:"nextPage"`
}

// Creator represents the model creator information
type Creator struct {
	Username string `json:"username"`
	Image    string `json:"image"` // Optional: might exist
}

// ModelDetail represents the detailed information for a single model from the Civitai API
// Used by the /api/v1/models/{modelID} endpoint
type ModelDetail struct {
	ID            int64          `json:"id"`
	Name          string         `json:"name"`
	Description   *string        `json:"description"` // Pointer for optional field
	Type          string         `json:"type"`
	Nsfw          bool           `json:"nsfw"`
	Tags          []string       `json:"tags"`
	Creator       Creator        `json:"creator"`
	ModelVersions []ModelVersion `json:"modelVersions"`
	// Add other relevant fields if needed based on API response
}

// CivitModelsRequest defines the parameters for fetching models from the Civitai API.
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

// NOTE: ModelVersion struct seems to be defined elsewhere (likely downloader.go or another types file)
// Ensure it's accessible within this package.
