package archiver

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/downloader"
	"github.com/yansigit/civitai-downloader/storage"
)

// Archiver orchestrates the model archiving process
type Archiver struct {
	Config         *config.Config
	StorageBackend storage.StorageBackend
	DB             *sql.DB // SQLite database handle
}

// NewArchiver creates a new Archiver instance
func NewArchiver(cfg *config.Config, query string) (*Archiver, error) {
	var backend storage.StorageBackend

	// Initialize the appropriate storage backend
	if cfg.Storage.Type == "fuckingfast" {
		// Pass AuthToken, BasePath will be handled by SaveFile using config path
		backend = &storage.RemoteStorageBackend{AuthToken: cfg.Storage.AuthToken}
	} else if cfg.Storage.Type == "local" {
		backend = &storage.LocalStorageBackend{}
		// Ensure the base local directory exists
		if err := backend.EnsureDirectory(cfg.Storage.Path); err != nil {
			return nil, fmt.Errorf("failed to ensure base local storage directory '%s': %w", cfg.Storage.Path, err)
		}
	} else {
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}

	// Initialize SQLite database
	db, err := storage.InitDB(cfg.Storage.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return &Archiver{
		Config:         cfg,
		StorageBackend: backend,
		DB:             db,
	}, nil
}

// Run executes the archiving process
func (a *Archiver) Run(query string, types []string, baseModels []string, dryrun bool) error {
	page := 1
	limit := 10 // Adjust limit as needed
	token := a.Config.Civitai.Token
	baseStoragePath := a.Config.Storage.Path // Get base path from config

	for _, modelType := range types {
		page = 1 // Reset page for each model type
		for {
			// Use the correct request struct from the downloader (or ideally api) package
			request := downloader.CivitModelsRequest{
				Limit: limit,
				Page:  page,
				Query: query,
				Nsfw:  "true",
				Types: []string{modelType}, // Send a single type per request
			}
			log.Printf("Requesting models with parameters: %+v", request)

			// Use the correct GetModels function
			models, metadata, err := downloader.GetModels(request, token)
			if err != nil {
				// Check for specific error indicating end of pagination if API uses that
				log.Printf("Failed to fetch models page %d for type %s: %v. Stopping pagination for this type.", page, modelType, err)
				break // Stop paginating for this type on error
			}

			// --- Filtering logic remains the same ---
			var filteredModels []downloader.Model
			if len(baseModels) == 0 {
				filteredModels = models
				log.Printf("No BaseModels filtering applied, processing all models: %d models", len(filteredModels))
			} else {
				for _, model := range models {
					for _, version := range model.ModelVersions {
						if sliceContains(baseModels, version.BaseModel) {
							filteredModels = append(filteredModels, model)
							break // Avoid adding the same model multiple times
						}
					}
				}
				log.Printf("Filtered models based on BaseModels: %d models", len(filteredModels))
				for _, model := range filteredModels {
					log.Printf("Model Name: %s", model.Name)
				}
			}

			for _, model := range filteredModels {
				log.Printf("Processing model: %s (ID: %d)", model.Name, model.ID)

				// Fetch model versions
				versions, err := downloader.GetModelVersions(model.ID, token)
				if err != nil {
					log.Printf("Failed to fetch versions for model %s: %v", model.Name, err)
					continue
				}

				// Process each version
				for _, version := range versions {
					// EnsureDirectory is now handled within LocalStorageBackend.SaveFile
					// No need to calculate destinationPath or call EnsureDirectory explicitly here.

					// Save main file and associated files
					if len(version.Files) > 0 {
						file := version.Files[0] // Process the first file

						// Updated DownloadAll call: pass baseStoragePath
						filePathOrID, err := downloader.DownloadAll(file, baseStoragePath, model, version, a.Config, a.DB, a.StorageBackend, dryrun)
						if err != nil {
							if err == nil && filePathOrID == "" {
								// Skipped, already logged
								continue
							}
							log.Printf("Failed to download/save file for model %s version %s: %v", model.Name, version.Name, err)
							continue
						}

						// Log to DB if successful and not dryrun
						if !dryrun && filePathOrID != "" {
							archivedInfo := storage.ArchivedModelInfo{
								ModelID:      model.ID,
								ModelName:    model.Name,
								VersionID:    version.ID,
								VersionName:  version.Name,
								BaseModel:    version.BaseModel,
								FileType:     file.Type,
								FileFormat:   file.Metadata.Format,
								FilePath:     filePathOrID, // Use the full path/key returned
								DownloadedAt: time.Now(),
							}
							if err := storage.LogModel(a.DB, archivedInfo); err != nil {
								log.Printf("Failed to log model %s version %s in database: %v", model.Name, version.Name, err)
							} else {
								// Log the full path/key for clarity
								log.Printf("Successfully logged model %s version %s to DB (Path: %s)", model.Name, version.Name, filePathOrID)
							}
						}
					} else {
						log.Printf("Skipping version %s for model %s as it has no files listed.", version.Name, model.Name)
					}
				} // End version loop
			} // End model loop

			// Pagination logic
			// Use NextCursor if available, otherwise check NextPage (though cursor is preferred)
			if metadata.NextCursor != nil && *metadata.NextCursor != "" {
				log.Printf("Moving to next page using cursor: %s", *metadata.NextCursor)
				request.Cursor = *metadata.NextCursor
				// Reset Page if using Cursor? API dependent, assume cursor handles position.
				page = 0 // Or remove page parameter entirely if cursor is used
			} else if metadata.NextPage != nil && *metadata.NextPage != "" {
				// Fallback to page number if cursor is not present but NextPage is
				page++
				log.Printf("Moving to next page number: %d", page)
				request.Cursor = "" // Ensure cursor is not sent when using page
			} else {
				log.Printf("No next page or cursor found for type %s. Finished.", modelType)
				break // No more pages/cursors
			}

			// Add a small delay to avoid rate limiting?
			// time.Sleep(500 * time.Millisecond)

		} // End pagination loop
	} // End type loop

	return nil
}

// sliceContains checks if a slice contains a specific item.
func sliceContains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
