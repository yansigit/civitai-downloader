package archiver

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/yansigit/civitai-downloader/downloader"
	"github.com/yansigit/civitai-downloader/logger"
	"github.com/yansigit/civitai-downloader/storage"
)

// processModelVersion handles the archiving logic for a single model version.
func (a *Archiver) processModelVersion(model downloader.Model, version downloader.ModelVersion, baseStoragePath string, token string, dryrun bool) error {
	logger.Info("Processing version ID: <yellow>%d</yellow>, Name: <cyan>%s</cyan>, BaseModel: <yellow>%s</yellow>", version.ID, version.Name, version.BaseModel)

	// Check if this version is already archived using the correct function
	exists, err := storage.CheckModelVersionExists(a.DB, version.ID)
	if err != nil {
		// Log unexpected DB error but continue (maybe DB connection issue?)
		logger.Warning("DB error checking archived status for version <yellow>%d</yellow>: <red>%v</red>", version.ID, err)
		// Allow processing to continue despite check error, maybe it doesn't exist
	}
	if exists {
		logger.Info("Version <yellow>%d</yellow> (<yellow>%s</yellow>) already archived according to DB. Skipping.", version.ID, version.Name)
		return nil // Already archived, success (or skip)
	}

	// Find the primary file from the version's file list
	var primaryFile *downloader.File = nil
	for _, file := range version.Files {
		if file.Primary {
			primaryFile = &file
			break
		}
	}

	// If no primary file is explicitly marked, try to find one (e.g., the first file)
	if primaryFile == nil {
		if len(version.Files) > 0 {
			logger.Warning("No primary file marked for version <yellow>%d</yellow>. Using first file '<yellow>%s</yellow>' as primary.", version.ID, version.Files[0].Name)
			primaryFile = &version.Files[0] // Use first file from the slice
		}
	}

	if primaryFile == nil {
		return fmt.Errorf("no files found for version %d, cannot determine primary file", version.ID)
	}
	logger.Debug("Identified primary file: <cyan>%s</cyan> (ID: <yellow>%d</yellow>)", primaryFile.Name, primaryFile.ID)

	// Call DownloadAll - it handles main file download & saving via backend,
	// and returns identifier + raw metadata/preview bytes if enabled.
	filePathOrID, metadataContent, previewContents, err := downloader.DownloadAll(
		*primaryFile, baseStoragePath, model, version, a.Config, a.DB, a.StorageBackend, dryrun,
	)
	if err != nil {
		// DownloadAll already logs errors internally, just return the error.
		return fmt.Errorf("failed to download/save primary file for version %d: %w", version.ID, err)
	}
	// If DownloadAll returns nil error but empty filePathOrID, it means it was skipped (e.g., already exists check inside DownloadAll) or dryrun.
	if filePathOrID == "" && !dryrun {
		logger.Info("DownloadAll completed for version <yellow>%d</yellow> but returned empty identifier (likely skipped).", version.ID)
		return nil // Consider skipped as success
	}
	if dryrun {
		logger.Info("[<yellow>DRY RUN</yellow>] Skipping actual DB logging and associated file processing for version <yellow>%d</yellow>.", version.ID)
		return nil
	}

	// --- Log the archived model version to DB --- //
	// Use the correct struct and logging function
	if a.DB != nil && filePathOrID != "" { // Only log if save was successful (filePathOrID is not empty)
		archivedModelInfo := storage.ArchivedModelInfo{ // Use ArchivedModelInfo
			ModelID:      model.ID,
			ModelName:    model.Name,
			VersionID:    version.ID,
			VersionName:  version.Name,
			BaseModel:    version.BaseModel,
			FileType:     primaryFile.Type,            // Get from primary file info
			FileFormat:   primaryFile.Metadata.Format, // Get from primary file info
			FilePath:     filePathOrID,                // Store the URL or path returned by DownloadAll
			DownloadedAt: time.Now(),
		}
		// Use LogModel
		err = storage.LogModel(a.DB, archivedModelInfo)
		if err != nil {
			// Log error but don't necessarily fail the whole process, file is saved.
			logger.Error("Error logging archived version <yellow>%d</yellow> to DB: <red>%v</red>", version.ID, err)
		} else {
			logger.Debug("Successfully logged primary file info for version <yellow>%d</yellow> to DB.", version.ID)
		}
	}

	// --- Process and save/log associated files (metadata, previews) --- //
	// These were already downloaded by DownloadAll if enabled in config.
	// _processAssociatedFiles now just needs to save/log them.
	// Determine base filename for associated files (without extension)
	baseFileName := strings.TrimSuffix(downloader.SanitizeFilename(primaryFile.Name), filepath.Ext(primaryFile.Name))
	// Determine relative path structure (needed for local storage)
	modelNameSanitized := downloader.SanitizeFilename(model.Name)
	creatorNameSanitized := downloader.SanitizeFilename(model.Creator.Username)
	modelTypeSanitized := downloader.SanitizeFilename(model.Type)
	versionNameSanitized := downloader.SanitizeFilename(version.Name)                                                 // Added for consistency
	relativePath := filepath.Join(modelTypeSanitized, creatorNameSanitized, modelNameSanitized, versionNameSanitized) // Path structure for organization

	err = a._processAssociatedFiles(version, modelNameSanitized, metadataContent, previewContents, baseFileName, relativePath, token, filePathOrID)
	if err != nil {
		// Log warning, but don't stop the process as the main file was successful
		logger.Warning("Error processing associated files for version <yellow>%d</yellow>: <red>%v</red>", version.ID, err)
	}

	return nil // Success
}

// _processAssociatedFiles handles saving/logging metadata and previews based on storage type.
// It now receives the metadata/preview bytes directly and the primary file's path/ID.
func (a *Archiver) _processAssociatedFiles(version downloader.ModelVersion, modelNameSanitized string, metadataContent []byte, previewContents [][]byte, baseFileName string, relativePath string, _ string, _ string) error {
	baseStoragePath := a.Config.Storage.Path // Needed for local storage base

	// --- Pomf.se / Fileditch Specific Logic --- //
	if a.Config.Storage.Type == "pomf" || a.Config.Storage.Type == "fileditch" {
		logger.Info("Processing associated files for <yellow>%s</yellow> storage...", a.Config.Storage.Type)
		// For Pomf/Fileditch, save each associated file individually and log its URL in the DB.

		// Save metadata if fetched
		if metadataContent != nil {
			metadataFileName := fmt.Sprintf("%s_%d.civitai.info", modelNameSanitized, version.ID)
			metadataURL, err := a.StorageBackend.SaveFile(a.DB, version.ID, "metadata", metadataContent, "", "", metadataFileName)
			if err != nil {
				logger.Warning("Failed to save metadata file '<yellow>%s</yellow>' to <yellow>%s</yellow>: <red>%v</red>", metadataFileName, a.Config.Storage.Type, err)
			} else {
				logger.Info("Successfully saved metadata to <yellow>%s</yellow>: <cyan>%s</cyan> (URL: <cyan>%s</cyan>)", a.Config.Storage.Type, metadataFileName, metadataURL)
				if a.DB != nil {
					metadataAssocInfo := storage.AssociatedFileInfo{
						ArchivedModelVersionID: version.ID, // Changed field name based on DB schema
						FileCategory:           "metadata",
						FileIdentifier:         metadataURL,
						OriginalFilename:       metadataFileName,
						MimeType:               "application/json",
						OrderIndex:             0,
					}
					assocErr := storage.LogAssociatedFile(a.DB, metadataAssocInfo) // Use LogAssociatedFile
					if assocErr != nil {
						logger.Error("Error logging associated file (metadata) for version <yellow>%d</yellow>: <red>%v</red>", version.ID, assocErr)
					}
				}
			}
		}

		// Save previews if fetched
		if len(previewContents) > 0 {
			for i, previewBytes := range previewContents {
				// Determine original filename and extension
				originalPreviewFilename := fmt.Sprintf("preview_%d_%d.png", version.ID, i) // Default
				mimeType := "image/png"
				if i < len(version.Images) {
					originalURL := version.Images[i].URL
					if parsedURL, parseErr := url.Parse(originalURL); parseErr == nil {
						base := filepath.Base(parsedURL.Path)
						ext := filepath.Ext(base)
						if base != "" && base != "." {
							originalPreviewFilename = downloader.SanitizeFilename(fmt.Sprintf("%s_%d_%d%s", modelNameSanitized, version.ID, i, ext))
							mimeType = downloader.GetMimeTypeFromExtension(ext) // Helper needed
						}
					}
				}

				// previewReader := bytes.NewReader(previewBytes) // Not needed, previewBytes is []byte
				// Pomf/Fileditch SaveFile ignores baseStoragePath and relativePath
				previewURL, err := a.StorageBackend.SaveFile(a.DB, version.ID, "preview", previewBytes, "", "", originalPreviewFilename)
				if err != nil {
					logger.Warning("Failed to save preview file '<yellow>%s</yellow>' to <yellow>%s</yellow>: <red>%v</red>", originalPreviewFilename, a.Config.Storage.Type, err)
				} else {
					logger.Info("Successfully saved preview <yellow>%d</yellow> to <yellow>%s</yellow>: <cyan>%s</cyan> (URL: <cyan>%s</cyan>)", i, a.Config.Storage.Type, originalPreviewFilename, previewURL)
					if a.DB != nil {
						previewAssocInfo := storage.AssociatedFileInfo{
							ArchivedModelVersionID: version.ID, // Changed field name
							FileCategory:           "preview",
							FileIdentifier:         previewURL,
							OriginalFilename:       originalPreviewFilename,
							MimeType:               mimeType,
							OrderIndex:             i,
						}
						assocErr := storage.LogAssociatedFile(a.DB, previewAssocInfo) // Use LogAssociatedFile
						if assocErr != nil {
							logger.Error("Error logging associated file (preview <yellow>%d</yellow>) for version <yellow>%d</yellow>: <red>%v</red>", i, version.ID, assocErr)
						}
					}
				}
			}
		}
	} // --- End Pomf/Fileditch Logic ---

	// --- Local Storage Specific Logic --- //
	if a.Config.Storage.Type == "local" {
		logger.Info("Processing associated files for local storage (relative path: <cyan>%s</cyan>)...", relativePath)
		// Associated files are saved alongside the main file using the same relativePath.
		// The storage backend handles placing them correctly using baseStoragePath and relativePath.

		// Save metadata locally
		if metadataContent != nil {
			// Use baseFileName derived from primary file for consistency
			metadataFileName := fmt.Sprintf("%s.civitai.info", baseFileName)
			// metadataReader := bytes.NewReader(metadataContent) // Not needed
			// Pass the original relativePath calculated in processModelVersion
			_, err := a.StorageBackend.SaveFile(a.DB, version.ID, "metadata", metadataContent, baseStoragePath, relativePath, metadataFileName)
			if err != nil {
				logger.Warning("Failed to save local metadata file '<yellow>%s</yellow>' in '<yellow>%s</yellow>': <red>%v</red>", metadataFileName, relativePath, err)
			} else {
				logger.Info("Successfully saved local metadata: <cyan>%s</cyan>", filepath.Join(relativePath, metadataFileName))
			}
		}

		// Save previews locally
		if len(previewContents) > 0 {
			for i, previewBytes := range previewContents {
				imgExt := ".png"
				originalURL := ""
				if i < len(version.Images) {
					originalURL = version.Images[i].URL
					if urlExt := filepath.Ext(originalURL); urlExt != "" && len(urlExt) <= 5 {
						imgExt = urlExt
					}
				}
				// Use baseFileName derived from primary file for consistency
				previewFileName := downloader.SanitizeFilename(fmt.Sprintf("%s.%d.preview%s", baseFileName, i, imgExt))
				// previewReader := bytes.NewReader(previewBytes) // Not needed
				// Pass the original relativePath calculated in processModelVersion
				_, err := a.StorageBackend.SaveFile(a.DB, version.ID, "preview", previewBytes, baseStoragePath, relativePath, previewFileName)
				if err != nil {
					logger.Warning("Failed to save local preview file '<yellow>%s</yellow>' in '<yellow>%s</yellow>': <red>%v</red>", previewFileName, relativePath, err)
				} else {
					logger.Info("Successfully saved local preview <yellow>%d</yellow>: <cyan>%s</cyan>", i, filepath.Join(relativePath, previewFileName))
				}
			}
		}
	} // --- End Local Logic ---

	// Add logic for other storage types if needed

	return nil // Return nil as associated file errors are logged but not fatal
}
