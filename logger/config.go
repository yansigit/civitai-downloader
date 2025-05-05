package logger

import "strings"

// Config holds the configuration for the logger
type Config struct {
	Level            string `yaml:"level"`             // debug, info, warning, error
	EnableFileLogging bool   `yaml:"enable_file_logging"`
}

// ParseLevel converts a string log level to LogLevel
func ParseLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warning", "warn":
		return LevelWarning
	case "error":
		return LevelError
	default:
		return LevelInfo // Default to info
	}
}

// ApplyConfig applies the logger configuration
func ApplyConfig(cfg Config) {
	level := ParseLevel(cfg.Level)
	Initialize(level, cfg.EnableFileLogging)
}
