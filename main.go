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
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <query>", os.Args[0])
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
	if query == "--query" && len(os.Args) > 2 {
		query = os.Args[2]
	}

	arch, err := archiver.NewArchiver(cfg, query)
	if err != nil {
		log.Fatalf("Error initializing archiver: %v", err)
	}

	// Run archiver
	if err := arch.Run(query, types); err != nil {
		log.Fatalf("Archiving process failed: %v", err)
	}

	fmt.Println("Archiving process completed successfully.")
}
