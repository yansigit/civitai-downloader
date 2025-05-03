package main

import (
	"fmt"
	"log"
	"os"

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
	query := os.Args[1]

	arch, err := archiver.NewArchiver(cfg, query)
	if err != nil {
		log.Fatalf("Error initializing archiver: %v", err)
	}

	// Run archiver
	if err := arch.Run(query); err != nil {
		log.Fatalf("Archiving process failed: %v", err)
	}

	fmt.Println("Archiving process completed successfully.")
}
