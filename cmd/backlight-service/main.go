package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/librescoot/dbc-backlight-service/internal/config"
	"github.com/librescoot/dbc-backlight-service/internal/service"
)

var version = "0.1.0" // Default version, can be overridden during build

func main() {
	// Create logger
	var logger *log.Logger
	if os.Getenv("INVOCATION_ID") != "" {
		logger = log.New(os.Stdout, "", 0)
	} else {
		logger = log.New(os.Stdout, "dbc-backlight: ", log.LstdFlags|log.Lmsgprefix)
	}

	// Create config
	cfg := config.New()
	cfg.Parse()

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create service
	svc, err := service.New(cfg, logger, version)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Run service
	if err := svc.Run(ctx); err != nil {
		log.Fatalf("Service failed: %v", err)
	}
}