package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/librescoot/dbc-backlight-service/internal/config"
	"github.com/librescoot/dbc-backlight-service/internal/service"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	cfg := config.New()
	cfg.Parse()

	if *showVersion {
		fmt.Printf("dbc-backlight %s\n", version)
		return
	}

	var logger *log.Logger
	if os.Getenv("JOURNAL_STREAM") != "" {
		logger = log.New(os.Stdout, "", 0)
	} else {
		logger = log.New(os.Stdout, "dbc-backlight: ", log.LstdFlags|log.Lmsgprefix)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := service.New(cfg, logger, version)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("Service failed: %v", err)
	}
}
