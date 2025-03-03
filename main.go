package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: ", os.Args[0], " <model_type> <model_url|AIR>")
		return
	}

	modelType := os.Args[1]
	modelIdentifier := os.Args[2]

	config, err := loadConfig("config.yaml")
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	token := config.Civitai.Token
	if token == "" {
		fmt.Println("Civitai token not found in config.yaml")
		return
	}

	baseModelPath := config.ComfyUI.BaseModelPath
	if baseModelPath == "" {
		baseModelPath = "."
		fmt.Println("Base model path not specified, using current directory")
	}

	outputPath := filepath.Join(baseModelPath, modelType, "add-detail-xl.safetensors")

	var downloadURL string
	if strings.HasPrefix(modelIdentifier, "urn:air:") {
		// Handle AIR identifier
		parts := strings.Split(modelIdentifier, ":")
		modelID := parts[len(parts)-2]
		version := parts[len(parts)-1]
		downloadURL = fmt.Sprintf("https://api.civitai.com/v1/air/%s/%s?token=%s", modelID, version, token)
	} else {
		// Handle direct URL
		downloadURL = modelIdentifier + "?token=" + token
	}

	err = downloadFile(outputPath, downloadURL)
	if err != nil {
		fmt.Printf("Error downloading %s: %v\n", modelType, err)
		return
	}

	fmt.Printf("Model downloaded successfully to %s\n", outputPath)
}

func loadConfig(filename string) (*Config, error) {
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

func downloadFile(outputPath string, url string) error {
	dir := filepath.Dir(outputPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error for %s: %v", url, resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", outputPath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
