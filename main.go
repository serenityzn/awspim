package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/logger"
	slackpkg "github.com/serenityzn/awspim/pkg/slack"
)


func main() {
	if err := run(); err != nil {
		// Use fmt.Fprintf to stderr since logger might not be initialized
		fmt.Fprintf(os.Stderr, "Application failed to start: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration first
	_, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	
	log := logger.GetDefaultLogger()
	log.Info("Configuration loaded successfully")
	
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	
	// Channel to signal when bot has stopped
	done := make(chan error, 1)
	
	log.Info("Starting AWS PIM Slack bot for slash commands")
	
	// Start Slack bot in a goroutine
	go func() {
		done <- slackpkg.StartSlackBot()
	}()
	
	// Wait for either:
	// 1. Bot to exit with error
	// 2. Interrupt signal (SIGTERM/SIGINT)
	select {
	case err := <-done:
		if err != nil {
			log.WithError(err).Error("Slack bot stopped with error")
			return fmt.Errorf("slack bot error: %w", err)
		}
		log.Info("Slack bot stopped normally")
		return nil
		
	case sig := <-sigChan:
		log.WithField("signal", sig.String()).Info("Received shutdown signal, gracefully exiting")
		log.Info("Slack WebSocket connections will be closed automatically")
		
		// The Slack socketmode client will automatically disconnect when process exits
		// We don't need to explicitly close it
		return nil
	}
}
