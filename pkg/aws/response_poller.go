package aws

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/logger"
)

// NotifyFunc is called for each response received from the assigner.
// slackUserID is the Slack user to notify; message is the human-readable text.
type NotifyFunc func(slackUserID, message string)

// StartResponsePoller starts a long-running goroutine that polls the response
// SQS queue and calls notify for every result the assigner sends back.
// It returns immediately; the goroutine stops when ctx is cancelled.
// If response_sqs_url is not configured the poller is skipped silently.
func StartResponsePoller(ctx context.Context, notify NotifyFunc) {
	cfg := config.Get()
	queueURL := cfg.GetResponseSQSURL()
	if queueURL == "" {
		logger.GetDefaultLogger().
			LogAWSOperation("response_poller", logger.Fields{}).
			Info("response_sqs_url not configured — response poller disabled")
		return
	}

	go func() {
		log := logger.GetDefaultLogger()
		log.LogAWSOperation("response_poller_start", logger.Fields{
			"queue_url": queueURL,
		}).Info("Response poller started")

		for {
			select {
			case <-ctx.Done():
				log.LogAWSOperation("response_poller_stop", logger.Fields{}).Info("Response poller stopped")
				return
			default:
			}

			if err := pollOnce(queueURL, notify); err != nil {
				log.LogAWSOperation("response_poller_error", logger.Fields{
					"queue_url": queueURL,
				}).WithError(err).Warn("Error during response queue poll — retrying")
			}
		}
	}()
}

// pollOnce does a single long-poll (20 s) on the response queue and processes
// any messages received.
func pollOnce(queueURL string, notify NotifyFunc) error {
	log := logger.GetDefaultLogger()

	sessionManager, err := GetSessionManager()
	if err != nil {
		return err
	}
	if err := sessionManager.RefreshIfNeeded(); err != nil {
		log.LogAWSOperation("response_poller_session_refresh", logger.Fields{}).
			WithError(err).Warn("Session refresh failed, continuing")
	}

	sqsClient := sessionManager.GetSQSClient()

	result, err := sqsClient.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: aws.Int64(10),
		WaitTimeSeconds:     aws.Int64(20), // long-poll
		VisibilityTimeout:   aws.Int64(30),
	})
	if err != nil {
		return err
	}

	for _, msg := range result.Messages {
		if err := handleResponseMessage(sqsClient, queueURL, msg, notify); err != nil {
			log.LogAWSOperation("response_message_error", logger.Fields{
				"message_id": aws.StringValue(msg.MessageId),
			}).WithError(err).Warn("Failed to process response message")
		}
	}
	return nil
}

// handleResponseMessage parses a single response message, calls notify, then deletes it.
func handleResponseMessage(sqsClient *sqs.SQS, queueURL string, msg *sqs.Message, notify NotifyFunc) error {
	log := logger.GetDefaultLogger()

	var resp ResponseMessage
	if err := json.Unmarshal([]byte(aws.StringValue(msg.Body)), &resp); err != nil {
		log.LogAWSOperation("parse_response_message", logger.Fields{
			"message_id": aws.StringValue(msg.MessageId),
		}).WithError(err).Error("Failed to parse response message body")
		// Delete malformed message so it doesn't block the queue
		deleteMessage(sqsClient, queueURL, msg)
		return err
	}

	log.LogAWSOperation("response_received", logger.Fields{
		"request_id":    resp.RequestID,
		"slack_user_id": resp.SlackUserID,
		"account_id":    resp.AccountID,
		"status":        resp.Status,
	}).Info("Received assigner response")

	if resp.SlackUserID != "" {
		notify(resp.SlackUserID, formatResponseMessage(resp))
	}

	deleteMessage(sqsClient, queueURL, msg)
	return nil
}

// formatResponseMessage builds the human-readable Slack notification text.
func formatResponseMessage(resp ResponseMessage) string {
	account := resp.AccountID
	if resp.AccountName != "" {
		account = resp.AccountName + " (`" + resp.AccountID + "`)"
	}

	switch resp.Status {
	case "granted":
		msg := "✅ *Access Granted*\n\nYour request for account " + account + " has been processed and access has been granted."
		if resp.Reason != "" {
			msg += "\n\n_Note: " + resp.Reason + "_"
		}
		return msg
	case "revoked":
		msg := "🔄 *Access Revoked*\n\nYour access for account " + account + " has been revoked."
		if resp.Reason != "" {
			msg += "\n\n_Reason: " + resp.Reason + "_"
		}
		return msg
	case "rejected":
		msg := "❌ *Request Rejected*\n\nYour access request for account " + account + " was rejected."
		if resp.Reason != "" {
			msg += "\n\n_Reason: " + resp.Reason + "_"
		}
		return msg
	case "failed":
		msg := "⚠️ *Processing Failed*\n\nYour access request for account " + account + " could not be processed due to a system error."
		if resp.Reason != "" {
			msg += "\n\n_Details: " + resp.Reason + "_"
		}
		return msg
	default:
		msg := "ℹ️ *Update on your request*\n\nAccount: " + account + "\nStatus: " + resp.Status
		if resp.Reason != "" {
			msg += "\nDetails: " + resp.Reason
		}
		return msg
	}
}

func deleteMessage(sqsClient *sqs.SQS, queueURL string, msg *sqs.Message) {
	log := logger.GetDefaultLogger()
	_, err := sqsClient.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	})
	if err != nil {
		log.LogAWSOperation("delete_response_message", logger.Fields{
			"message_id": aws.StringValue(msg.MessageId),
		}).WithError(err).Warn("Failed to delete response message from queue")
	}
}
