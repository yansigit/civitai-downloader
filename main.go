package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/yansigit/civitai-downloader/archiver"
	"github.com/yansigit/civitai-downloader/config"
)

func main() {
	// Load configuration
	configPath := "C:/Users/User/.civitai-downloader/config.yaml"
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// Initialize archiver
	dryrun := false
	for _, arg := range os.Args {
		if arg == "--dryrun" {
			dryrun = true
			break
		}
	}

	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <query> [--baseModels <model1,model2,...>] [--types <type1,type2,...>] [--dryrun]", os.Args[0])
	}
	// Parse query argument, ensuring it excludes any "--query" prefix
	query := os.Args[1]

	// Parse types argument if provided
	var types []string
	for i, arg := range os.Args {
		if arg == "--types" && i+1 < len(os.Args) {
			types = strings.Split(os.Args[i+1], ",")
		}
	}

	if query == "-" {
		query = ""
	}

	// Parse baseModels argument if provided
	var baseModels []string
	for i, arg := range os.Args {
		if arg == "--baseModels" && i+1 < len(os.Args) {
			baseModels = strings.Split(os.Args[i+1], ",")
		}
	}

	arch, err := archiver.NewArchiver(cfg, query)
	if err != nil {
		log.Fatalf("Error initializing archiver: %v", err)
	}

	// Run archiver with baseModels
	if err := arch.Run(query, types, baseModels, dryrun); err != nil {
		log.Fatalf("Archiving process failed: %v", err)
	}

	fmt.Println("Archiving process completed successfully.")
}
