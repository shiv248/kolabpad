package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/shiv248/kolabpad/pkg/database"
	"github.com/shiv248/kolabpad/pkg/logger"
	"github.com/shiv248/kolabpad/pkg/server"
)

// Config holds all server configuration
type Config struct {
	Port                  string
	ExpiryDays            int
	SQLiteURI             string
	CleanupInterval       time.Duration
	MaxDocumentSize       int
	WSReadTimeout         time.Duration
	WSWriteTimeout        time.Duration
	BroadcastBufferSize   int
}

func main() {
	// Initialize logger
	logger.Init()

	// Load configuration from environment
	config := Config{
		Port:                getEnv("PORT", "3030"),
		ExpiryDays:          getEnvInt("EXPIRY_DAYS", 7),
		SQLiteURI:           os.Getenv("SQLITE_URI"),
		CleanupInterval:     time.Duration(getEnvInt("CLEANUP_INTERVAL_HOURS", 1)) * time.Hour,
		MaxDocumentSize:     getEnvInt("MAX_DOCUMENT_SIZE_KB", 256) * 1024, // Convert KB to bytes
		WSReadTimeout:       time.Duration(getEnvInt("WS_READ_TIMEOUT_MINUTES", 30)) * time.Minute,
		WSWriteTimeout:      time.Duration(getEnvInt("WS_WRITE_TIMEOUT_SECONDS", 10)) * time.Second,
		BroadcastBufferSize: getEnvInt("BROADCAST_BUFFER_SIZE", 16),
	}

	logger.Info("Starting Kolabpad server...")
	logger.Info("Port: %s", config.Port)
	logger.Info("Document expiry: %d days", config.ExpiryDays)

	// Initialize database if configured
	var db *database.Database
	if config.SQLiteURI != "" {
		logger.Info("Database: %s", config.SQLiteURI)
		var err error
		db, err = database.New(config.SQLiteURI)
		if err != nil {
			logger.Error("Failed to initialize database: %v", err)
			log.Fatalf("Failed to initialize database: %v", err)
		}
		defer db.Close()
	} else {
		logger.Info("Database: disabled (in-memory only)")
	}

	// Create server with config
	srv := server.NewServer(db, config.MaxDocumentSize, config.BroadcastBufferSize, config.WSReadTimeout, config.WSWriteTimeout)

	// Start cleanup task
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.StartCleaner(ctx, config.ExpiryDays, config.CleanupInterval)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutting down...")
		cancel()
		srv.Shutdown(ctx)
		os.Exit(0)
	}()

	// Start server
	addr := fmt.Sprintf(":%s", config.Port)
	log.Fatal(srv.ListenAndServe(addr))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
