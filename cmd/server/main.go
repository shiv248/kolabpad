package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/shiv/kolabpad/pkg/server"
)

func main() {
	// Load configuration from environment
	port := getEnv("PORT", "3030")
	expiryDays := getEnvInt("EXPIRY_DAYS", 1)

	log.Printf("Starting Kolabpad server...")
	log.Printf("Port: %s", port)
	log.Printf("Document expiry: %d days", expiryDays)

	// Create server
	srv := server.NewServer()

	// Start cleanup task
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.StartCleaner(ctx, expiryDays)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
		srv.Shutdown(ctx)
		os.Exit(0)
	}()

	// Start server
	addr := fmt.Sprintf(":%s", port)
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
