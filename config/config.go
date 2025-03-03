package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Civitai struct {
		Token string `yaml:"token"`
	} `yaml:"civitai"`
	ComfyUI struct {
		BaseModelPath string `yaml:"base_model_path"`
	} `yaml:"comfyui"`
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
