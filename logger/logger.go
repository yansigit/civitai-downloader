package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarning
	LevelError
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorPink   = "\033[38;5;213m"
	colorWhite  = "\033[37m"
	colorGray   = "\033[90m"
	colorOrange = "\033[38;5;208m"
	colorTeal   = "\033[38;5;6m"
	colorBrown  = "\033[38;5;130m"
	colorLime   = "\033[38;5;118m"
)

type Logger struct {
	fileLogger        *log.Logger
	console           *log.Logger
	minLevel          LogLevel
	enableFileLogging bool
}

var defaultLogger *Logger

func init() {
	defaultLogger = NewLogger(LevelInfo, false) // Default to info level, no file logging
}

// NewLogger creates a new logger instance
func NewLogger(level LogLevel, enableFileLogging bool) *Logger {
	logger := &Logger{
		console:           log.New(os.Stdout, "", 0),
		minLevel:          level,
		enableFileLogging: enableFileLogging,
	}

	if enableFileLogging {
		// Create logs directory if it doesn't exist
		logDir := "logs"
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Printf("Failed to create logs directory: %v", err)
		} else {
			// Create log file with timestamp
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			logPath := filepath.Join(logDir, fmt.Sprintf("civitai-downloader_%s.log", timestamp))
			file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Printf("Failed to open log file: %v", err)
			} else {
				// Only write fileLogger to the file, not stdout as well.
				logger.fileLogger = log.New(file, "", 0)
			}
		}
	}

	return logger
}

// SetDefaultLogger sets the default logger instance
func SetDefaultLogger(logger *Logger) {
	defaultLogger = logger
}

// processColorTags replaces color tags with ANSI color codes
func processColorTags(message string) string {
	replacements := map[string]string{
		"<red>":     colorRed,
		"</red>":    colorReset,
		"<green>":   colorGreen,
		"</green>":  colorReset,
		"<yellow>":  colorYellow,
		"</yellow>": colorReset,
		"<blue>":    colorBlue,
		"</blue>":   colorReset,
		"<purple>":  colorPurple,
		"</purple>": colorReset,
		"<cyan>":    colorCyan,
		"</cyan>":   colorReset,
		"<pink>":    colorPink,
		"</pink>":   colorReset,
		"<white>":   colorWhite,
		"</white>":  colorReset,
		"<gray>":    colorGray,
		"</gray>":   colorReset,
		"<orange>":  colorOrange,
		"</orange>": colorReset,
		"<teal>":    colorTeal,
		"</teal>":   colorReset,
		"<brown>":   colorBrown,
		"</brown>":  colorReset,
		"<lime>":    colorLime,
		"</lime>":   colorReset,
	}

	for tag, code := range replacements {
		message = strings.ReplaceAll(message, tag, code)
	}
	return message
}

// log writes a log message with the specified level
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.minLevel {
		return
	}

	var levelStr string
	switch level {
	case LevelDebug:
		levelStr = "[DEBUG]"
	case LevelInfo:
		levelStr = "[INFO] "
	case LevelWarning:
		levelStr = "[WARN] "
	case LevelError:
		levelStr = "[ERROR]"
	}

	timestamp := time.Now().Format("2006/01/02 15:04:05")
	message := fmt.Sprintf(format, args...)
	formattedMessage := fmt.Sprintf("%s %s %s", timestamp, levelStr, processColorTags(message))

	if l.fileLogger != nil {
		// Remove color codes for file logging
		noColorMessage := strings.ReplaceAll(formattedMessage, "\033[0m", "")
		noColorMessage = strings.ReplaceAll(noColorMessage, "\033[31m", "")
		noColorMessage = strings.ReplaceAll(noColorMessage, "\033[32m", "")
		noColorMessage = strings.ReplaceAll(noColorMessage, "\033[33m", "")
		noColorMessage = strings.ReplaceAll(noColorMessage, "\033[34m", "")
		noColorMessage = strings.ReplaceAll(noColorMessage, "\033[35m", "")
		noColorMessage = strings.ReplaceAll(noColorMessage, "\033[36m", "")
		l.fileLogger.Println(noColorMessage)
	}

	l.console.Println(formattedMessage)
}

// Package-level logging functions

func Debug(format string, v ...interface{}) {
	defaultLogger.log(LevelDebug, format, v...)
}

func Info(format string, v ...interface{}) {
	defaultLogger.log(LevelInfo, format, v...)
}

func Warning(format string, v ...interface{}) {
	defaultLogger.log(LevelWarning, format, v...)
}

func Error(format string, v ...interface{}) {
	defaultLogger.log(LevelError, format, v...)
}

// Initialize initializes the default logger with the specified level and file logging
func Initialize(level LogLevel, enableFileLogging bool) {
	defaultLogger = NewLogger(level, enableFileLogging)
}
