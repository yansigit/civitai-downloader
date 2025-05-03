package archiver

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/downloader"
	"github.com/yansigit/civitai-downloader/storage"
)

// Archiver orchestrates the model archiving process
type Archiver struct {
	Config         *config.Config
	StorageBackend storage.StorageBackend
}

// NewArchiver creates a new Archiver instance
func NewArchiver(cfg *config.Config, query string) (*Archiver, error) {
	var backend storage.StorageBackend

	// Initialize the appropriate storage backend
	if cfg.Storage.Type == "local" {
		backend = &storage.LocalStorageBackend{}
	} else {
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}

	return &Archiver{
		Config:         cfg,
		StorageBackend: backend,
	}, nil
}

// Run executes the archiving process
func (a *Archiver) Run(query string) error {
	page := 1
	limit := 1
	token := a.Config.Civitai.Token

	for {
		request := downloader.CivitModelsRequest{
			Limit: limit,
			Page:  page,
			Query: query,
		}
		log.Printf("Requesting models with parameters: %+v", request)
		models, metadata, err := downloader.GetModels(request, token)
		if err != nil {
			return fmt.Errorf("failed to fetch models: %w", err)
		}

		filteredModels := []downloader.Model{}
		for _, model := range models {
			if sliceContains(a.Config.Filters.BaseModels, model.BaseModel) {
				filteredModels = append(filteredModels, model)
			}
		}
		log.Printf("Filtered models based on BaseModels: %+v", filteredModels)
		models = filteredModels

		for _, model := range models {
			log.Printf("Processing model: %s (ID: %d)", model.Name, model.ID)

			// Fetch model versions
			versions, err := downloader.GetModelVersions(model.ID, token)
			if err != nil {
				log.Printf("Failed to fetch versions for model %s: %v", model.Name, err)
				continue
			}

			// Process each version
			for _, version := range versions {
				destinationPath := filepath.Join(a.Config.Storage.Path, model.Type, model.BaseModel)
				if err := a.StorageBackend.EnsureDirectory(destinationPath); err != nil {
					log.Printf("Failed to create directory for model %s: %v", model.Name, err)
					continue
				}

				// Save main file
				if len(version.Files) > 0 {
					file := version.Files[0]
					if err := a.StorageBackend.SaveFile(file.DownloadURL, destinationPath, version.Name, file.Type); err != nil {
						log.Printf("Failed to save file for model %s: %v", model.Name, err)
						continue
					}
				}

				// Save metadata
				metadata, err := json.Marshal(version)
				if err == nil {
					if err := a.StorageBackend.SaveMetadata(metadata, destinationPath, version.Name); err != nil {
						log.Printf("Failed to save metadata for model %s: %v", model.Name, err)
					}
				}

				// Save description
				// Save description (handle pointer)
				if version.Description != nil && *version.Description != "" {
					// Dereference the pointer to pass the string value
					if err := a.StorageBackend.SaveDescription(*version.Description, destinationPath, version.Name); err != nil {
						log.Printf("Failed to save description for model %s: %v", model.Name, err)
					}
				}
			}
		}

		if !metadata.HasNext {
			break
		}
		page++
	}

	return nil
}

// sliceContains checks if a slice contains a specific item
func sliceContains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
