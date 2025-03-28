package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/downloader"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: ", os.Args[0], " <model_type> <model_url|AIR>")
		return
	}

	modelType := os.Args[1]
	modelIdentifier := os.Args[2]

	if strings.HasPrefix(os.Args[1], "urn:air:") {
		modelIdentifier = os.Args[1]
		modelType = ""
	}

	configPath := os.Getenv("HOME") + "/.civitai-downloader/config.yaml"
	config, err := config.LoadConfig(configPath)
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

	var modelVersionId string
	if strings.HasPrefix(modelIdentifier, "urn:air:") {
		parts := strings.Split(modelIdentifier, ":")
		if modelType == "" {
			modelType = parts[3]
		}
		modelInfo := strings.Split(parts[len(parts)-1], "@")
		modelVersionId = modelInfo[1]
	} else {
		re := regexp.MustCompile(`https://civitai.com/models/(\d+)`)
		matches := re.FindStringSubmatch(modelIdentifier)
		if len(matches) == 2 {
			modelVersionId = matches[1]
		}
	}
	fmt.Println("Model version ID:", modelVersionId)
	outputPath := filepath.Join(baseModelPath, modelType, fmt.Sprintf("temp-%d.safetensors", time.Now().UnixNano()))

	err = downloader.DownloadAll(modelType, baseModelPath, modelVersionId, config)
	if err != nil {
		fmt.Printf("Error downloading %s: %v\n", modelType, err)
		return
	}

	fmt.Printf("Model downloaded successfully to %s\n", filepath.Dir(outputPath))
}
