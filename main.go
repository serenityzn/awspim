package main

import (
	awspkg "github.com/serenityzn/awspim/pkg/aws"
	"github.com/serenityzn/awspim/pkg/logger"
	slackpkg "github.com/serenityzn/awspim/pkg/slack"
)

type MyEvent struct {
	UserId              string `json:"userid"`
	AwsAccountId        string `json:"awsaccountid"`
	PermissionsBoundary string `json:"permissionsboundary"`
	RequestedTime       string `json:"requestedtime"`
}

func (e MyEvent) checkAwsAccountId() bool {
	return awspkg.ValidateAccountId(e.AwsAccountId)
}

func init() {
	log := logger.GetDefaultLogger()
	
	err := awspkg.Initialize()
	if err != nil {
		log.WithError(err).Error("Failed to initialize AWS package")
		// Don't exit for Lambda, just log the error
	} else {
		log.Info("AWS package initialized successfully")
	}
}

func main() {
	log := logger.GetDefaultLogger()
	
	log.Info("Starting AWS PIM Slack bot for slash commands")
	
	err := slackpkg.StartSlackBot()
	if err != nil {
		log.WithError(err).Fatal("Failed to start Slack bot")
	}
}
