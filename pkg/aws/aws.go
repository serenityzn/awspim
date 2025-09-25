package aws

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-secretsmanager-caching-go/secretcache"
	"github.com/serenityzn/awspim/pkg/logger"
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

var globalAwsAccounts *AwsAccounts

// Initialize loads AWS accounts from Secrets Manager and stores them globally
func Initialize() error {
	log := logger.GetDefaultLogger()

	log.LogAWSOperation("initialize", logger.Fields{"action": "loading_accounts"}).Info("Initializing AWS accounts")

	result, err := getAwsAccounts()
	if err != nil {
		log.LogAWSOperation("initialize", logger.Fields{"action": "loading_accounts"}).WithError(err).Error("Failed to retrieve AWS accounts")
		return fmt.Errorf("error retrieving AWS accounts: %v", err)
	}

	globalAwsAccounts = result
	log.LogAWSOperation("initialize", logger.Fields{
		"action":         "loading_accounts",
		"accounts_count": len(result.Accounts),
	}).Info("AWS accounts loaded successfully")

	return nil
}

// GetAccounts returns the globally loaded AWS accounts
func GetAccounts() *AwsAccounts {
	return globalAwsAccounts
}

// ValidateAccountId checks if the given account ID exists in the loaded accounts
func ValidateAccountId(accountId string) bool {
	if globalAwsAccounts == nil {
		return false
	}

	for _, account := range globalAwsAccounts.Accounts {
		if account.AccountId == accountId {
			return true
		}
	}
	return false
}

// GetAccountName returns the account name for a given account ID
func GetAccountName(accountId string) string {
	if globalAwsAccounts == nil {
		return ""
	}

	for _, account := range globalAwsAccounts.Accounts {
		if account.AccountId == accountId {
			return account.AccountName
		}
	}
	return ""
}

// SendApprovalNotification sends an approval message to the configured SQS queue
func SendApprovalNotification(requestor, approver, accountID string) error {
	log := logger.GetDefaultLogger()

	log.LogAWSOperation("send_approval_notification", logger.Fields{
		"requestor":  requestor,
		"approver":   approver,
		"account_id": accountID,
	}).Info("Sending approval notification")

	sqsARN := os.Getenv("MANAGER_SQS_ARN")
	if sqsARN == "" {
		log.LogAWSOperation("send_approval_notification", logger.Fields{"error": "missing_sqs_arn"}).Error("MANAGER_SQS_ARN environment variable is not set")
		return fmt.Errorf("MANAGER_SQS_ARN environment variable is not set")
	}

	// Get AWS region from environment variable, default to us-east-2
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}

	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		log.LogAWSOperation("send_approval_notification", logger.Fields{"region": region}).WithError(err).Error("Failed to create AWS session")
		return fmt.Errorf("failed to create AWS session: %v", err)
	}

	// Create SQS service client
	sqsClient := sqs.New(sess)

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
		return fmt.Errorf("failed to marshal approval message: %v", err)
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
		return fmt.Errorf("failed to send SQS message: %v", err)
	}

	log.LogAWSOperation("send_approval_notification", logger.Fields{
		"requestor":  requestor,
		"approver":   approver,
		"account_id": accountID,
		"queue_url":  sqsARN,
	}).Info("Approval notification sent successfully")

	return nil
}

func getAwsAccounts() (*AwsAccounts, error) {
	log := logger.GetDefaultLogger()

	log.LogAWSOperation("get_aws_accounts", logger.Fields{"action": "retrieving_from_secrets_manager"}).Debug("Retrieving AWS accounts from Secrets Manager")

	awsAccountsSecret := os.Getenv("AWS_ACCOUNTS_SECRET")
	if awsAccountsSecret == "" {
		log.LogAWSOperation("get_aws_accounts", logger.Fields{"error": "missing_secret_name"}).Error("AWS_ACCOUNTS_SECRET environment variable is not set")
		return nil, fmt.Errorf("AWS_ACCOUNTS_SECRET environment variable is not set")
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		log.LogAWSOperation("get_aws_accounts", logger.Fields{"region": region}).WithError(err).Error("Failed to create AWS session")
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	svc := secretsmanager.New(sess)

	secret, err := secretcache.New(func(cache *secretcache.Cache) {
		cache.Client = svc
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create secret cache: %v", err)
	}

	secretValue, err := secret.GetSecretString(awsAccountsSecret)
	if err != nil {
		log.LogAWSOperation("get_aws_accounts", logger.Fields{"secret_name": awsAccountsSecret}).WithError(err).Error("Failed to retrieve secret")
		return nil, fmt.Errorf("failed to retrieve secret: %v", err)
	}

	var awsAccounts AwsAccounts
	if err := json.Unmarshal([]byte(secretValue), &awsAccounts); err != nil {
		log.LogAWSOperation("get_aws_accounts", logger.Fields{"secret_name": awsAccountsSecret}).WithError(err).Error("Failed to parse secret JSON")
		return nil, fmt.Errorf("failed to parse secret JSON: %v", err)
	}

	log.LogAWSOperation("get_aws_accounts", logger.Fields{
		"secret_name":    awsAccountsSecret,
		"accounts_count": len(awsAccounts.Accounts),
	}).Info("AWS accounts retrieved successfully")

	return &awsAccounts, nil
}
