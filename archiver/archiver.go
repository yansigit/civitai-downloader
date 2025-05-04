package archiver

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/downloader"
	"github.com/yansigit/civitai-downloader/storage"
	"golang.org/x/exp/slices"
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
	var db *sql.DB // Declare db handle here
	var err error  // Declare err here

	// Initialize SQLite database first, as it might be needed by backends or is needed regardless
	db, err = storage.InitDB(cfg.Storage.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize the appropriate storage backend
	switch cfg.Storage.Type {
	case "fuckingfast":
		// Pass AuthToken, BasePath will be handled by SaveFile using config path
		backend = &storage.RemoteStorageBackend{AuthToken: cfg.Storage.AuthToken}
	case "local":
		backend = &storage.LocalStorageBackend{}
		// Ensure the base local directory exists
		if err := backend.EnsureDirectory(cfg.Storage.Path); err != nil {
			return nil, fmt.Errorf("failed to ensure base local storage directory '%s': %w", cfg.Storage.Path, err)
		}
	case "pomf":
		// Pomf backend itself doesn't need the DB handle during initialization
		backend = &storage.PomfStorageBackend{}
	case "fileditch": // Add fileditch case
		backend = &storage.FileditchStorageBackend{}
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Storage.Type)
	}

	// DB handle is already initialized above and stored in 'db' variable

	return &Archiver{
		Config:         cfg,
		StorageBackend: backend,
		DB:             db, // Assign the initialized DB handle to the Archiver struct
	}, nil
}

// Run executes the archiving process
func (a *Archiver) Run(query string, types []string, baseModels []string, dryrun bool) error {
	page := 1
	limit := 20 // Adjust limit as needed
	token := a.Config.Civitai.Token
	baseStoragePath := a.Config.Storage.Path // Get base path from config (used by local storage)

	for _, modelType := range types {
		page = 1
		cursor := "" // New: initialize cursor for pagination
		for {
			var request downloader.CivitModelsRequest
			if cursor != "" {
				request = downloader.CivitModelsRequest{
					Limit:  limit,
					Query:  query,
					Nsfw:   "true",
					Sort:   "Most Downloaded",
					Types:  []string{modelType},
					Cursor: cursor,
				}
			} else {
				request = downloader.CivitModelsRequest{
					Limit: limit,
					Page:  page,
					Query: query,
					Nsfw:  "true",
					Sort:  "Most Downloaded",
					Types: []string{modelType},
				}
			}
			log.Printf("Requesting models with parameters: %+v", request)

			models, metadata, err := downloader.GetModels(request, token)
			if err != nil {
				log.Printf("Failed to fetch models page %d for type %s: %v. Stopping pagination for this type.", page, modelType, err)
				break
			}

			// --- Filtering logic remains the same ---
			var filteredModels []downloader.Model
			if len(baseModels) == 0 {
				filteredModels = models
				log.Printf("No BaseModels filtering applied, processing %d models", len(filteredModels))
			} else {
				for _, model := range models {
					for _, version := range model.ModelVersions {
						if sliceContains(baseModels, version.BaseModel) {
							filteredModels = append(filteredModels, model)
							break // Avoid adding the same model multiple times
						}
					}
				}
				log.Printf("Filtered models based on BaseModels (%v): %d models", baseModels, len(filteredModels))
				// Optional: Log filtered model names for verification
				// for _, model := range filteredModels {
				// log.Printf("  - %s", model.Name)
				// }
			}
			// --- End Filtering ---

			if len(filteredModels) == 0 && len(models) > 0 {
				log.Printf("No models matched the base model filter on this page.")
			}

			for _, model := range filteredModels {
				log.Printf("Processing model: %s (ID: %d)", model.Name, model.ID)

				// Fetch model versions
				versions, err := downloader.GetModelVersions(model.ID, token)
				if err != nil {
					log.Printf("Failed to fetch versions for model %s (ID: %d): %v", model.Name, model.ID, err)
					continue // Skip this model if versions can't be fetched
				}

				// Process each version
				for _, version := range versions {
					log.Printf("Processing version: %s (ID: %d) for model %s", version.Name, version.ID, model.Name)

					// Check DB *before* attempting download
					exists, err := storage.CheckModelVersionExists(a.DB, version.ID)
					if err != nil {
						log.Printf("Error checking database for model version %d: %v", version.ID, err)
						continue // Skip this version on DB error
					}
					if exists {
						log.Printf("Model version %d (%s) already exists in DB. Skipping.", version.ID, version.Name)
						continue
					}

					// Select the primary file to download
					if len(version.Files) == 0 {
						log.Printf("Skipping version %s for model %s as it has no files listed.", version.Name, model.Name)
						continue
					}
					file := version.Files[0] // Process the first file

					// Call DownloadAll
					mainFileIDOrPath, metadataContent, previewContents, err := downloader.DownloadAll(file, baseStoragePath, model, version, a.Config, a.DB, a.StorageBackend, dryrun)

					// Handle errors or skips from DownloadAll
					if err != nil {
						log.Printf("Failed to download/save main file for model %s version %s: %v", model.Name, version.Name, err)
						continue // Skip this version if main file failed
					}
					if mainFileIDOrPath == "" && !dryrun {
						log.Printf("Skipping model %s version %s (DownloadAll returned empty path/ID without error).", model.Name, version.Name)
						continue
					}

					// --- Processing successful download ---

					if dryrun {
						log.Printf("Dryrun: Would log model %s version %s to DB (Path/ID: %s)", model.Name, version.Name, mainFileIDOrPath)
						if metadataContent != nil {
							log.Printf("Dryrun: Would save metadata for %s", mainFileIDOrPath)
						}
						if len(previewContents) > 0 {
							log.Printf("Dryrun: Would save %d previews for %s", len(previewContents), mainFileIDOrPath)
						}
						continue // Skip actual saving and DB logging in dryrun
					}

					// --- Actual Saving and DB Logging (Not Dryrun) ---

					// 1. Log the main model entry in archived_models
					archivedInfo := storage.ArchivedModelInfo{
						ModelID:      model.ID,
						ModelName:    model.Name,
						VersionID:    version.ID,
						VersionName:  version.Name,
						BaseModel:    version.BaseModel,
						FileType:     file.Type,
						FileFormat:   file.Metadata.Format,
						FilePath:     mainFileIDOrPath,
						DownloadedAt: time.Now(),
					}
					if err := storage.LogModel(a.DB, archivedInfo); err != nil {
						log.Printf("CRITICAL: Failed to log main model %s version %s in database after successful download: %v", model.Name, version.Name, err)
						// Continue to attempt saving associated files but log the critical failure.
					} else {
						log.Printf("Successfully logged main model %s version %s to DB (Path/ID: %s)", model.Name, version.Name, mainFileIDOrPath)
					}

					// 2. Handle Associated Files (Metadata, Previews) based on Storage Type

					// Common variables needed
					baseFileName := strings.TrimSuffix(downloader.SanitizeFilename(file.Name), filepath.Ext(file.Name))
					relativePath := path.Join(model.Type, model.Name, version.Name) // Used by local storage

					// --- Pomf & Fileditch Storage Specific Logic ---
					if a.Config.Storage.Type == "pomf" || a.Config.Storage.Type == "fileditch" {
						// 2a. Log main model file association
						modelAssocInfo := storage.AssociatedFileInfo{
							ArchivedModelVersionID: version.ID,
							FileCategory:           "model",
							FileIdentifier:         mainFileIDOrPath, // This is the Pomf/Fileditch URL for the main file
							OriginalFilename:       file.Name,
						}
						assocErr := storage.LogAssociatedFile(a.DB, modelAssocInfo)
						if assocErr != nil {
							log.Printf("Error logging associated file (model) for version %d: %v", version.ID, assocErr)
						}

						// 2b. Save and log metadata
						if metadataContent != nil {
							metadataFileName := fmt.Sprintf("%s.civitai.info", baseFileName)
							metadataReader := bytes.NewReader(metadataContent)
							// Pomf/Fileditch SaveFile ignores baseStoragePath and relativePath
							metadataURL, err := a.StorageBackend.SaveFile(metadataReader, "", "", metadataFileName)
							if err != nil {
								log.Printf("Warning: failed to save metadata file '%s' to %s: %v", metadataFileName, a.Config.Storage.Type, err)
							} else {
								log.Printf("Successfully saved metadata to %s: %s (URL: %s)", a.Config.Storage.Type, metadataFileName, metadataURL)
								metadataAssocInfo := storage.AssociatedFileInfo{
									ArchivedModelVersionID: version.ID,
									FileCategory:           "metadata",
									FileIdentifier:         metadataURL,
									OriginalFilename:       metadataFileName,
									MimeType:               "application/json",
								}
								assocErr := storage.LogAssociatedFile(a.DB, metadataAssocInfo)
								if assocErr != nil {
									log.Printf("Error logging associated file (metadata) for version %d: %v", version.ID, assocErr)
								}
							}
						}

						// 2c. Save and log previews
						if len(previewContents) > 0 {
							for i, previewBytes := range previewContents {
								// Determine original filename and extension
								imgExt := ".png"                                                                   // Default extension
								originalPreviewFilename := fmt.Sprintf("%s.%d.preview%s", baseFileName, i, imgExt) // Default filename
								mimeType := "image/png"                                                            // Default mime type

								if i < len(version.Images) { // Ensure index is valid
									originalURL := version.Images[i].URL
									if urlExt := filepath.Ext(originalURL); urlExt != "" && len(urlExt) <= 5 {
										imgExt = urlExt
										// Update filename and mime type based on actual extension
										originalPreviewFilename = downloader.SanitizeFilename(fmt.Sprintf("%s.%d.preview%s", baseFileName, i, imgExt))
										switch strings.ToLower(imgExt) {
										case ".jpg", ".jpeg":
											mimeType = "image/jpeg"
										case ".png":
											mimeType = "image/png"
										case ".gif":
											mimeType = "image/gif"
										case ".webp":
											mimeType = "image/webp"
											// Add other common image types if needed
										}
									}
								}

								previewReader := bytes.NewReader(previewBytes)
								// Pomf/Fileditch SaveFile ignores baseStoragePath and relativePath
								previewURL, err := a.StorageBackend.SaveFile(previewReader, "", "", originalPreviewFilename) // Use original filename for upload
								if err != nil {
									log.Printf("Warning: failed to save preview file '%s' to %s: %v", originalPreviewFilename, a.Config.Storage.Type, err)
								} else {
									log.Printf("Successfully saved preview %d to %s: %s (URL: %s)", i, a.Config.Storage.Type, originalPreviewFilename, previewURL)
									previewAssocInfo := storage.AssociatedFileInfo{
										ArchivedModelVersionID: version.ID,
										FileCategory:           "preview",
										FileIdentifier:         previewURL,
										OriginalFilename:       originalPreviewFilename,
										MimeType:               mimeType,
										OrderIndex:             i,
									}
									assocErr := storage.LogAssociatedFile(a.DB, previewAssocInfo)
									if assocErr != nil {
										log.Printf("Error logging associated file (preview %d) for version %d: %v", i, version.ID, assocErr)
									}
								}
							}
						}
					} // --- End Pomf/Fileditch Logic ---

					// --- Local Storage Specific Logic ---
					if a.Config.Storage.Type == "local" {
						// For local storage, associated files are saved alongside the main file.
						// The storage backend handles placing them correctly using baseStoragePath and relativePath.
						// We don't need to log associated files separately in the DB for local storage,
						// as their paths are implicitly relative to the main model file path.

						// 2b. Save metadata locally
						if metadataContent != nil {
							metadataFileName := fmt.Sprintf("%s.civitai.info", baseFileName)
							metadataReader := bytes.NewReader(metadataContent)
							_, err := a.StorageBackend.SaveFile(metadataReader, baseStoragePath, relativePath, metadataFileName)
							if err != nil {
								log.Printf("Warning: failed to save local metadata file '%s': %v", metadataFileName, err)
							} else {
								log.Printf("Successfully saved local metadata: %s", metadataFileName)
							}
						}

						// 2c. Save previews locally
						if len(previewContents) > 0 {
							for i, previewBytes := range previewContents {
								imgExt := ".png"
								if i < len(version.Images) {
									originalURL := version.Images[i].URL
									if urlExt := filepath.Ext(originalURL); urlExt != "" && len(urlExt) <= 5 {
										imgExt = urlExt
									}
								}
								previewFileName := downloader.SanitizeFilename(fmt.Sprintf("%s.%d.preview%s", baseFileName, i, imgExt))
								previewReader := bytes.NewReader(previewBytes)
								_, err := a.StorageBackend.SaveFile(previewReader, baseStoragePath, relativePath, previewFileName)
								if err != nil {
									log.Printf("Warning: failed to save local preview file '%s': %v", previewFileName, err)
								} else {
									log.Printf("Successfully saved local preview %d: %s", i, previewFileName)
								}
							}
						}
					} // --- End Local Logic ---

					// Add logic for other storage types like "fuckingfast" if they need special handling for metadata/previews

				} // End version loop
			} // End model loop

			// Pagination logic
			if metadata.NextCursor != nil && *metadata.NextCursor != "" {
				log.Printf("Moving to next page using cursor: %s", *metadata.NextCursor)
				cursor = *metadata.NextCursor
			} else if metadata.NextPage != nil && *metadata.NextPage != "" {
				page++
				log.Printf("Moving to next page number: %d", page)
				cursor = ""
			} else {
				log.Printf("No next page or cursor found for type %s. Finished.", modelType)
				break // No more pages/cursors
			}

			// Optional delay to avoid rate limiting
			// time.Sleep(500 * time.Millisecond)

		} // End pagination loop (while true)
	} // End type loop

	return nil
}

// sliceContains checks if a slice contains a specific item.
func sliceContains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}
