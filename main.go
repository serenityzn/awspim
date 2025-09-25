package main

import (
	"fmt"
	"os"
	
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/logger"
	slackpkg "github.com/serenityzn/awspim/pkg/slack"
)

type MyEvent struct {
	UserId              string `json:"userid"`
	AwsAccountId        string `json:"awsaccountid"`
	PermissionsBoundary string `json:"permissionsboundary"`
	RequestedTime       string `json:"requestedtime"`
}

// Note: MyEvent and checkAwsAccountId method appear to be unused legacy code
// TODO: Remove if not needed for Lambda functionality
func (e MyEvent) checkAwsAccountId() bool {
	// This would need to import awspkg if used
	// return awspkg.ValidateAccountId(e.AwsAccountId)
	return false
}

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
	
	log.Info("Starting AWS PIM Slack bot for slash commands")
	
	// Start Slack bot
	if err := slackpkg.StartSlackBot(); err != nil {
		log.WithError(err).Error("Failed to start Slack bot")
		return fmt.Errorf("failed to start Slack bot: %w", err)
	}
	
	return nil
}
