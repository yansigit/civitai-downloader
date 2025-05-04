package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

// CheckModelVersionExists checks if a specific model version exists in the database.
func CheckModelVersionExists(db *sql.DB, versionID int64) (bool, error) {
	query := `SELECT COUNT(*) FROM archived_models WHERE version_id = ?`
	var count int
	err := db.QueryRow(query, versionID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to query model version: %w", err)
	}
	return count > 0, nil
}

type ArchivedModelInfo struct {
	ModelID      int64     `json:"modelId"`
	ModelName    string    `json:"modelName"`
	VersionID    int64     `json:"versionId"`
	VersionName  string    `json:"versionName"`
	BaseModel    string    `json:"baseModel"`
	FileType     string    `json:"fileType"`   // e.g., "Model", "Pruned Model"
	FileFormat   string    `json:"fileFormat"` // e.g., "SafeTensor", "PickleTensor"
	FilePath     string    `json:"filePath"`   // The absolute path where the file was saved
	DownloadedAt time.Time `json:"downloadedAt"`
}

// InitDB initializes the SQLite database and creates the necessary table if it doesn't exist.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create table if not exists
	createArchivedModelsTableSQL := `CREATE TABLE IF NOT EXISTS archived_models (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		model_id INTEGER,
		model_name TEXT,
		version_id INTEGER,
		version_name TEXT,
		base_model TEXT,
		file_type TEXT,
		file_format TEXT,
		file_path TEXT UNIQUE, -- Ensure uniqueness based on path
		downloaded_at DATETIME
	);`

	_, err = db.Exec(createArchivedModelsTableSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to create archived_models table: %w", err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	return db, nil
}

// LogModel inserts a new record into the archived_models table.
func LogModel(db *sql.DB, info ArchivedModelInfo) error {
	insertSQL := `INSERT INTO archived_models (
		model_id, model_name, version_id, version_name, base_model,
		file_type, file_format, file_path, downloaded_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := db.Exec(insertSQL,
		info.ModelID, info.ModelName, info.VersionID, info.VersionName, info.BaseModel,
		info.FileType, info.FileFormat, info.FilePath, info.DownloadedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert model log: %w", err)
	}
	return nil
}
