package archiver

import (
	"database/sql"
	"fmt"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/storage"
)

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
