package logger

import (
	"log"
	"os"
	"strings"
)

// LogLevel represents the logging level
type LogLevel int

const (
	LevelError LogLevel = iota
	LevelInfo
	LevelDebug
)

var currentLevel LogLevel = LevelInfo

// Init initializes the logger with the specified level from environment
func Init() {
	levelStr := strings.ToLower(os.Getenv("LOG_LEVEL"))
	switch levelStr {
	case "debug":
		currentLevel = LevelDebug
	case "info":
		currentLevel = LevelInfo
	case "error":
		currentLevel = LevelError
	default:
		currentLevel = LevelInfo
	}
}

// Debug logs a debug message (only if LOG_LEVEL=debug)
func Debug(format string, v ...interface{}) {
	if currentLevel >= LevelDebug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// Info logs an info message (if LOG_LEVEL=info or debug)
func Info(format string, v ...interface{}) {
	if currentLevel >= LevelInfo {
		log.Printf("[INFO] "+format, v...)
	}
}

// Error logs an error message (always logged)
func Error(format string, v ...interface{}) {
	log.Printf("[ERROR] "+format, v...)
}
