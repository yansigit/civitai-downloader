package archiver

import (
	"database/sql"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/storage"
)

// Archiver orchestrates the model archiving process
type Archiver struct {
	Config         *config.Config
	StorageBackend storage.StorageBackend
	DB             *sql.DB // SQLite database handle
}
