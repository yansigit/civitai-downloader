package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Filters defines model filtering criteria
type Filters struct {
	Types      []string `yaml:"types,omitempty"`
	BaseModels []string `yaml:"baseModels,omitempty"`
}

// StorageTarget defines where models should be saved
type StorageTarget struct {
	Type          string            `yaml:"type"`
	Path          string            `yaml:"path"`
	DatabasePath  string            `yaml:"database_path"`            // Path to SQLite database
	CloudSettings map[string]string `yaml:"cloud_settings,omitempty"` // Optional cloud settings
	AuthToken     string            `yaml:"auth_token,omitempty"`     // Authentication token for remote storage
	SaveMetadata  bool              `yaml:"save_metadata,omitempty"`  // Whether to save metadata files
	SavePreviews  bool              `yaml:"save_previews,omitempty"`  // Whether to save preview images
}

type Config struct {
	Logger struct {
		Level             string `yaml:"level"`
		EnableFileLogging bool   `yaml:"enable_file_logging"`
	} `yaml:"logger"`
	Civitai struct {
		Token         string `yaml:"token"`
		NSFWOnly      bool   `yaml:"nsfw_only"`
		MaxFileSizeMB int    `yaml:"max_file_size_mb"`
	} `yaml:"civitai"`
	ComfyUI struct {
		BaseModelPath string `yaml:"base_model_path"`
	} `yaml:"comfyui"`
	Filters Filters       `yaml:"filters"`
	Storage StorageTarget `yaml:"storage"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	config := &Config{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
