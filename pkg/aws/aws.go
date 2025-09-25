package aws

import (
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/errors"
	"github.com/serenityzn/awspim/pkg/logger"
	"github.com/serenityzn/awspim/pkg/utils"
)

type AwsAccount struct {
	AccountId   string `json:"accountid"`
	AccountName string `json:"accountname"`
}

type AwsAccounts struct {
	Accounts []AwsAccount `json:"accounts"`
}

// ApprovalMessage represents the structure of approval notification sent to SQS
type ApprovalMessage struct {
	Requestor string `json:"requestor"`
	Approver  string `json:"approver"`
	Account   string `json:"account"`
	DateTime  string `json:"datetime"`
}

// GetAccounts fetches AWS accounts using cached data for better performance
func GetAccounts() (*AwsAccounts, error) {
	cache := GetAccountsCache()
	return cache.GetAccounts()
}

// ValidateAccountId checks if the given account ID exists and has valid format
func ValidateAccountId(accountId string) bool {
	log := logger.GetDefaultLogger()
	
	// First check format validation
	if !utils.ValidateAWSAccountID(accountId) {
		log.LogAWSOperation("validate_account_id", logger.Fields{
			"account_id": utils.SanitizeUserInput(accountId),
			"error": "invalid_format",
		}).Warn("Account ID format validation failed")
		return false
	}
	
	// Then check if it exists in our system using cached data
	cache := GetAccountsCache()
	accounts, err := cache.GetAccounts()
	if err != nil {
		log.LogAWSOperation("validate_account_id", logger.Fields{
			"account_id": accountId,
		}).WithError(err).Error("Failed to fetch accounts for validation")
		return false
	}

	for _, account := range accounts.Accounts {
		if account.AccountId == accountId {
			return true
		}
	}
	return false
}

// GetAccountName returns the account name for a given account ID using cached data
func GetAccountName(accountId string) string {
	log := logger.GetDefaultLogger()
	
	// Use cached data for better performance
	cache := GetAccountsCache()
	accounts, err := cache.GetAccounts()
	if err != nil {
		log.LogAWSOperation("get_account_name", logger.Fields{
			"account_id": accountId,
		}).WithError(err).Error("Failed to fetch accounts for name lookup")
		return ""
	}

	for _, account := range accounts.Accounts {
		if account.AccountId == accountId {
			return account.AccountName
		}
	}
	return ""
}

// SendApprovalNotification sends an approval message to the configured SQS queue using optimized connections
func SendApprovalNotification(requestor, approver, accountID string) error {
	log := logger.GetDefaultLogger()
	cfg := config.Get()

	log.LogAWSOperation("send_approval_notification", logger.Fields{
		"requestor":  utils.SanitizeUserInput(requestor),
		"approver":   utils.SanitizeUserInput(approver),
		"account_id": accountID,
	}).Info("Sending approval notification")

	sqsARN := cfg.GetManagerSQSARN()
	if sqsARN == "" {
		log.LogAWSOperation("send_approval_notification", logger.Fields{"error": "missing_sqs_arn"}).Error("MANAGER_SQS_ARN not configured")
		return errors.NewConfigurationError("MANAGER_SQS_ARN not configured", nil).WithContext("operation", "send_approval_notification")
	}

	// Use session manager for optimized connections
	sessionManager, err := GetSessionManager()
	if err != nil {
		return errors.NewAWSError("failed to get session manager", err)
	}

	// Refresh session if needed
	if err := sessionManager.RefreshIfNeeded(); err != nil {
		log.LogAWSOperation("send_approval_notification", logger.Fields{
			"warning": "session_refresh_failed",
		}).WithError(err).Warn("Failed to refresh AWS session, continuing with existing session")
	}

	// Get reused SQS client
	sqsClient := sessionManager.GetSQSClient()

	// Create approval message
	approvalMsg := ApprovalMessage{
		Requestor: requestor,
		Approver:  approver,
		Account:   accountID,
		DateTime:  time.Now().Format("2006-01-02 15:04"), // YYYY-MM-DD HH:MM format
	}

	// Marshal message to JSON
	messageBody, err := json.Marshal(approvalMsg)
	if err != nil {
		return errors.NewInternalError("failed to marshal approval message", err).WithContext("message", approvalMsg)
	}

	// Send message to SQS
	_, err = sqsClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    aws.String(sqsARN),
		MessageBody: aws.String(string(messageBody)),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"MessageType": {
				DataType:    aws.String("String"),
				StringValue: aws.String("PIM_APPROVAL"),
			},
			"Account": {
				DataType:    aws.String("String"),
				StringValue: aws.String(accountID),
			},
		},
	})
	if err != nil {
		log.LogAWSOperation("send_approval_notification", logger.Fields{
			"queue_url":  sqsARN,
			"account_id": accountID,
		}).WithError(err).Error("Failed to send SQS message")
		return errors.NewAWSError("failed to send SQS message", err).WithContext("queue_url", sqsARN).WithContext("account_id", accountID)
	}

	log.LogAWSOperation("send_approval_notification", logger.Fields{
		"requestor":  utils.SanitizeUserInput(requestor),
		"approver":   utils.SanitizeUserInput(approver),
		"account_id": accountID,
		"queue_url":  sqsARN,
	}).Info("Approval notification sent successfully")

	return nil
}
