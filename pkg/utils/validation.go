package utils

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

// AWS Account ID must be exactly 12 digits
var awsAccountIDRegex = regexp.MustCompile(`^\d{12}$`)

// ValidateAWSAccountID checks if the provided string is a valid AWS account ID format
func ValidateAWSAccountID(accountID string) bool {
	// Remove whitespace and backticks (users often copy-paste from Slack code formatting)
	accountID = strings.TrimSpace(accountID)
	accountID = strings.Trim(accountID, "`")
	
	// Check if it matches the AWS account ID pattern (12 digits)
	return awsAccountIDRegex.MatchString(accountID)
}

// ValidateEmail checks if the provided string is a valid email address format
func ValidateEmail(email string) error {
	// Use Go's built-in email validation
	_, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("invalid email format: %w", err)
	}
	return nil
}

// SanitizeUserInput removes potentially dangerous characters from user input for logging
func SanitizeUserInput(input string) string {
	// Remove newlines, carriage returns, and other control characters
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	input = strings.ReplaceAll(input, "\t", "")
	
	// Limit length to prevent log flooding
	if len(input) > 100 {
		input = input[:100] + "..."
	}
	
	return input
}

// ValidateSlackUserID checks if the provided string looks like a valid Slack user ID
func ValidateSlackUserID(userID string) bool {
	// Slack user IDs typically start with 'U' and are followed by alphanumeric characters
	matched, _ := regexp.MatchString(`^U[A-Z0-9]{8,}$`, userID)
	return matched
}

// ValidateSlackChannelID checks if the provided string looks like a valid Slack channel ID
func ValidateSlackChannelID(channelID string) bool {
	// Slack channel IDs typically start with 'C' and are followed by alphanumeric characters
	matched, _ := regexp.MatchString(`^C[A-Z0-9]{8,}$`, channelID)
	return matched
}
