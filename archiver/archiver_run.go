package archiver

import (
	"time"

	"github.com/yansigit/civitai-downloader/downloader"
	"github.com/yansigit/civitai-downloader/logger"
)

// Run executes the archiving process
func (a *Archiver) Run(query string, types []string, baseModels []string, dryrun bool) error {
	page := 0
	limit := 40 // Adjust limit as needed
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
			logger.Debug("Requesting models with parameters: %+v", request)

			models, metadata, err := downloader.GetModels(request, token)
			if err != nil {
				logger.Error("Failed to fetch models page <yellow>%d</yellow> for type <yellow>%s</yellow>: <red>%v</red>. Stopping pagination for this type.", page, modelType, err)
				break
			}

			// --- Filtering logic remains the same ---
			var filteredModels []downloader.Model
			if len(baseModels) == 0 {
				filteredModels = models
				logger.Debug("No BaseModels filtering applied, processing <yellow>%d</yellow> models", len(filteredModels))
			} else {
				for _, model := range models {
					for _, version := range model.ModelVersions {
						if sliceContains(baseModels, version.BaseModel) { // Use helper from utils
							filteredModels = append(filteredModels, model)
							break // Avoid adding the same model multiple times
						}
					}
				}
				logger.Debug("Filtered models based on BaseModels (<yellow>%v</yellow>): <yellow>%d</yellow> models", baseModels, len(filteredModels))
				// Optional: Log filtered model names for verification
				// for _, model := range filteredModels {
				// log.Printf("  - %s", model.Name)
				// }
			}
			// --- End Filtering ---

			if len(filteredModels) == 0 && len(models) > 0 {
				logger.Info("No models matched the base model filter on this page.")
			}

			for _, model := range filteredModels {
				for _, version := range model.ModelVersions {
					if len(baseModels) == 0 || sliceContains(baseModels, version.BaseModel) { // Process only if no filter or matches filter
						err := a.processModelVersion(model, version, baseStoragePath, token, dryrun)
						if err != nil {
							logger.Error("Error processing version <yellow>%d</yellow> for model <yellow>%d</yellow> (<yellow>%s</yellow>): <red>%v</red>", version.ID, model.ID, model.Name, err)
							// Decide whether to continue with other versions/models or stop
							// For now, log and continue
						}
						// Optional: Add a small delay between processing versions if needed
						time.Sleep(100 * time.Millisecond)
					}
				}
			}

			// Handle pagination using cursor
			if metadata.NextCursor != nil && *metadata.NextCursor != "" { // Check pointer then dereference
				cursor = *metadata.NextCursor // Dereference pointer for assignment
				logger.Debug("Moving to next page with cursor: <cyan>%s</cyan>", cursor)
				// Reset page if using cursor primarily, or maybe remove page logic?
				// Civitai API seems to favor cursor over page when both are present.
				page = 0 // Indicate we are using cursor
			} else if metadata.NextPage != nil && *metadata.NextPage != "" { // Check pointer then dereference (assuming NextPage is also a string URL part)
				// The original API likely returns NextPage as a full URL or path, treat as string
				// If it was meant to be an int, the downloader.Metadata struct needs correction.
				// For now, assume it's a string like cursor and handle potential int conversion elsewhere if needed.
				// Let's assume we just need to know *if* there's a next page via URL/string, not the page number itself.
				// Reverting to simple page increment for now, as NextPage might be deprecated or complex.
				page++
				logger.Debug("Moving to next page number: <yellow>%d</yellow>", page)
				cursor = "" // Ensure cursor is empty if using page increment
			} else {
				logger.Debug("No more pages for type <yellow>%s</yellow>.", modelType)
				break // Exit the inner loop for this type
			}

			// Optional: Add a delay between fetching pages
			time.Sleep(500 * time.Millisecond)
		}
	}

	logger.Info("<green>Archiver run completed.</green>")
	return nil
}
