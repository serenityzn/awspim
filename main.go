package main

import (
	"fmt"
	awspkg "github.com/serenityzn/awspim/pkg/aws"
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
	err := awspkg.Initialize()
	if err != nil {
		fmt.Printf("Error initializing AWS package: %v\n", err)
		// Don't exit for Lambda, just log the error
	}
}

func main() {
	fmt.Println("Starting Slack bot for slash commands...")
	err := slackpkg.StartSlackBot()
	if err != nil {
		fmt.Printf("Failed to start Slack bot: %v\n", err)
	}
}
