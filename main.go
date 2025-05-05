package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/yansigit/civitai-downloader/archiver"
	"github.com/yansigit/civitai-downloader/config"
	"github.com/yansigit/civitai-downloader/logger"
)

func main() {
	// Load configuration
	var configPath string
	if home := os.Getenv("HOME"); home != "" && (os.PathSeparator == '/') {
		configPath = fmt.Sprintf("%s/.civitai-downloader/config.yaml", home)
	} else {
		configPath = "C:/Users/User/.civitai-downloader/config.yaml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		logger.Error("Error loading config: <red>%v</red>", err)
		os.Exit(1)
	}

	// Initialize logger
	logger.Initialize(logger.LevelInfo, cfg.Logger.EnableFileLogging)
	if cfg.Logger.Level != "" {
		loggerCfg := logger.Config{
			Level:            cfg.Logger.Level,
			EnableFileLogging: cfg.Logger.EnableFileLogging,
		}
		logger.ApplyConfig(loggerCfg)
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
		logger.Error("Usage: %s <query> [--baseModels <model1,model2,...>] [--types <type1,type2,...>] [--dryrun]", os.Args[0])
		os.Exit(1)
	}
	// Parse query argument, ensuring it excludes any "--query" prefix
	query := os.Args[1]

	// Parse base models filter if provided
	var baseModels []string
	for i, arg := range os.Args {
		if arg == "--baseModels" && i+1 < len(os.Args) {
			baseModels = strings.Split(os.Args[i+1], ",")
		}
	}

	// Parse types filter if provided
	var types []string
	for i, arg := range os.Args {
		if arg == "--types" && i+1 < len(os.Args) {
			types = strings.Split(os.Args[i+1], ",")
		}
	}

	if len(types) == 0 {
		types = []string{"Checkpoint", "TextualInversion", "Hypernetwork", "AestheticGradient", "LORA", "LoCon", "VAE", "Poses", "Controlnet", "Upscaler", "MotionModule", "Wildcards"}
	}

	if query == "-" {
		query = ""
	}

	arch, err := archiver.NewArchiver(cfg, query)
	if err != nil {
		logger.Error("Error initializing archiver: <red>%v</red>", err)
		os.Exit(1)
	}

	if dryrun {
		logger.Info("<yellow>Running in dry-run mode - no files will be downloaded or modified</yellow>")
	}

	if err := arch.Run(query, types, baseModels, dryrun); err != nil {
		logger.Error("Archiving process failed: <red>%v</red>", err)
		os.Exit(1)
	}

	logger.Info("<green>Archiving process completed successfully.</green>")
}
