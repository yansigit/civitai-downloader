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
	CloudSettings map[string]string `yaml:"cloud_settings,omitempty"` // Optional cloud settings
}

type Config struct {
	Civitai struct {
		Token string `yaml:"token"`
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
