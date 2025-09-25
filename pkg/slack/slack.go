package slack

import (
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	awspkg "github.com/serenityzn/awspim/pkg/aws"
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/errors"
	"github.com/serenityzn/awspim/pkg/logger"
	"github.com/serenityzn/awspim/pkg/utils"
)

type handler struct {
	client *slack.Client
}

type Handler interface {
	HandleCommand(evt *socketmode.Event, client *socketmode.Client) error
	HandleInteraction(evt *socketmode.Event, client *socketmode.Client) error
}

// SlackClient wraps the Slack client for easier use
type SlackClient struct {
	client *slack.Client
}

// NewSlackClient creates a new Slack client
func NewSlackClient() (*SlackClient, error) {
	cfg := config.Get()
	token := cfg.GetSlackBotToken()
	if token == "" {
		return nil, errors.NewConfigurationError("SLACK_BOT_TOKEN not configured", nil)
	}

	client := slack.New(token, slack.OptionDebug(false))
	return &SlackClient{client: client}, nil
}

// SendMessage sends a message to a specific channel
func (sc *SlackClient) SendMessage(channelName, message string) error {
	// Try to send message directly to channel name (with # prefix)
	channelRef := "#" + channelName
	_, _, err := sc.client.PostMessage(channelRef, slack.MsgOptionText(message, false))
	if err != nil {
		return fmt.Errorf("failed to send message to %s: %v", channelRef, err)
	}

	return nil
}

// SendMessageToUser sends a direct message to a user
func (sc *SlackClient) SendMessageToUser(userID, message string) error {
	_, _, err := sc.client.PostMessage(userID, slack.MsgOptionText(message, false))
	if err != nil {
		return fmt.Errorf("failed to send DM: %v", err)
	}
	return nil
}

// StartSlackBot starts the Socket Mode bot for slash commands (optional)
func StartSlackBot() error {
	log := logger.GetDefaultLogger()
	cfg := config.Get()

	log.LogSlackOperation("start_bot", logger.Fields{"action": "initializing"}).Info("Starting Slack bot initialization")

	// Start rate limiter periodic cleanup
	rateLimiter := GetRateLimiter()
	rateLimiter.StartPeriodicCleanup()

	token := cfg.GetSlackBotToken()
	appToken := cfg.GetSlackAppToken()

	if token == "" || appToken == "" {
		log.LogSlackOperation("start_bot", logger.Fields{"error": "missing_tokens"}).Error("SLACK_BOT_TOKEN and SLACK_APP_TOKEN not configured")
		return errors.NewConfigurationError("SLACK_BOT_TOKEN and SLACK_APP_TOKEN not configured", nil)
	}

	client := slack.New(token, slack.OptionDebug(false), slack.OptionAppLevelToken(appToken))

	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(false),
	)

	// Create handler
	handler := NewHandler(client)

	// Handle events
	go func() {
		for evt := range socketClient.Events {
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				log.LogSlackOperation("socket_connection", logger.Fields{"status": "connecting"}).Info("Connecting to Slack with Socket Mode")
			case socketmode.EventTypeConnectionError:
				log.LogSlackOperation("socket_connection", logger.Fields{"status": "error"}).Warn("Connection failed. Retrying later")
			case socketmode.EventTypeConnected:
				log.LogSlackOperation("socket_connection", logger.Fields{"status": "connected"}).Info("Connected to Slack with Socket Mode")
			case socketmode.EventTypeSlashCommand:
				if err := handler.HandleCommand(&evt, socketClient); err != nil {
					log.LogSlackOperation("handle_command", logger.Fields{"event_type": "slash_command"}).WithError(err).Error("Error handling slash command")
				}
			case socketmode.EventTypeInteractive:
				if err := handler.HandleInteraction(&evt, socketClient); err != nil {
					log.LogSlackOperation("handle_interaction", logger.Fields{"event_type": "interactive"}).WithError(err).Error("Error handling interaction")
				}
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					log.LogSlackOperation("events_api", logger.Fields{"error": "type_cast_failed"}).Warn("Could not type cast the event to the EventsAPIEvent")
					continue
				}
				socketClient.Ack(*evt.Request, eventsAPIEvent)
			}
		}
	}()

	log.LogSlackOperation("start_bot", logger.Fields{"status": "running"}).Info("Slack bot started successfully")
	socketClient.Run()
	return nil
}

func NewHandler(client *slack.Client) Handler {
	return &handler{
		client: client,
	}
}

func (h *handler) HandleCommand(evt *socketmode.Event, client *socketmode.Client) error {
	log := logger.GetDefaultLogger()
	cfg := config.Get()

	switch evt.Type {
	case socketmode.EventTypeSlashCommand:
		cmd, ok := evt.Data.(slack.SlashCommand)
		if !ok {
			log.LogSlackOperation("handle_command", logger.Fields{"error": "type_cast_failed"}).Error("Could not type cast the event to a SlashCommand")
			return errors.NewSlackError("could not type cast event to SlashCommand", nil)
		}

		// Check rate limiting
		rateLimiter := GetRateLimiter()
		if !rateLimiter.IsAllowed(cmd.UserID) {
			log.LogSecurityEvent("rate_limited_command", logger.Fields{
				"user_id": cmd.UserID,
				"command": cmd.Command,
				"channel_id": cmd.ChannelID,
			}).Warn("Command blocked due to rate limiting")

			// Acknowledge the command but send rate limit message
			client.Ack(*evt.Request)
			_, err := h.client.PostEphemeral(cmd.ChannelID, cmd.UserID, 
				slack.MsgOptionText("⚠️ **Rate Limited**\n\nYou're sending commands too quickly. Please wait before trying again.", false))
			if err != nil {
				log.LogSlackOperation("send_rate_limit_message", logger.Fields{
					"channel_id": cmd.ChannelID,
					"user_id": cmd.UserID,
				}).WithError(err).Error("Failed to send rate limit message")
			}
			return nil
		}

		// Handle /pim command
		if cmd.Command == "/pim" {
			log.LogUserAction(cmd.UserID, "pim_command", logger.Fields{
				"command": cmd.Command,
				"text": cmd.Text,
				"channel_id": cmd.ChannelID,
			}).Info("Processing /pim command")
			
			// Acknowledge the command immediately
			client.Ack(*evt.Request)

			// Check if command is used in the correct channel
			channelInfo, err := h.client.GetConversationInfo(&slack.GetConversationInfoInput{
				ChannelID: cmd.ChannelID,
			})
			if err != nil {
				log.LogSlackOperation("get_channel_info", logger.Fields{
					"channel_id": cmd.ChannelID,
					"user_id": cmd.UserID,
				}).WithError(err).Error("Failed to get channel info")
				h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText("Error: Could not verify channel information.", false))
				return err
			}

			if channelInfo.Name != cfg.GetAllowedChannel() {
				log.LogSecurityEvent("unauthorized_channel_usage", logger.Fields{
					"user_id": cmd.UserID,
					"channel_name": channelInfo.Name,
					"channel_id": cmd.ChannelID,
					"command": cmd.Command,
				}).Warn("User attempted to use /pim command in unauthorized channel")
				
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(fmt.Sprintf("This command can only be used in the #%s channel.", cfg.GetAllowedChannel()), false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send error message")
					return err
				}
				return nil
			}

			// Get the parameter provided after /pim command
			parameter := cmd.Text
			var message string
			
			if parameter != "" {
				// Check if the parameter is a valid AWS account ID
				if awspkg.ValidateAccountId(parameter) {
					log.LogUserAction(cmd.UserID, "valid_account_request", logger.Fields{
						"account_id": parameter,
						"channel_id": cmd.ChannelID,
					}).Info("User requested access to valid AWS account")
					
					accountName := awspkg.GetAccountName(parameter)
					// Generate admin access request message with approval button
					userName := cmd.UserName
					if cmd.UserName == "" {
						userName = fmt.Sprintf("<@%s>", cmd.UserID)
					}
					
					// Create message with interactive button
					requestText := fmt.Sprintf("🔐 *Admin Access Request*\n\nUser: *%s* requested Admin access to account *%s* (%s).\n\n✅ Account validated successfully.", 
						userName, parameter, accountName)
					
					// Create approval button with requestor username included
					requestorUsername := cmd.UserName
					if requestorUsername == "" {
						requestorUsername = fmt.Sprintf("<@%s>", cmd.UserID)
					}
					
					approvalButton := slack.NewButtonBlockElement("approve_access", fmt.Sprintf("%s:%s:%s:%s", cmd.UserID, requestorUsername, parameter, accountName), slack.NewTextBlockObject("plain_text", "✅ Approve Access", false, false))
					approvalButton.Style = slack.StylePrimary
					
					denyButton := slack.NewButtonBlockElement("deny_access", fmt.Sprintf("%s:%s:%s:%s", cmd.UserID, requestorUsername, parameter, accountName), slack.NewTextBlockObject("plain_text", "❌ Deny Access", false, false))
					denyButton.Style = slack.StyleDanger
					
					actionBlock := slack.NewActionBlock("approval_actions", approvalButton, denyButton)
					
					// Send message with blocks instead of plain text
					_, _, err = h.client.PostMessage(cmd.ChannelID, 
						slack.MsgOptionText(requestText, false),
						slack.MsgOptionBlocks(
							slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", requestText, false, false), nil, nil),
							actionBlock,
						),
					)
					if err != nil {
						log.LogSlackOperation("send_approval_request", logger.Fields{
							"channel_id": cmd.ChannelID,
							"user_id": cmd.UserID,
							"account_id": parameter,
						}).WithError(err).Error("Failed to send message with buttons")
						return err
					}
					return nil // Return early since we've already sent the message
				} else {
					log.LogSecurityEvent("invalid_account_request", logger.Fields{
						"user_id": cmd.UserID,
						"invalid_account_id": utils.SanitizeUserInput(parameter),
						"channel_id": cmd.ChannelID,
					}).Warn("User requested access to invalid AWS account")
					
					message = fmt.Sprintf("❌ **Invalid Account ID**\n\nAccount ID `%s` is not found in our system.\n\nUse `/acc` to see available accounts.", utils.SanitizeUserInput(parameter))
				}
			} else {
				message = "❌ **Missing Account ID**\n\nPlease provide an AWS Account ID.\n\nUsage: `/pim [account-id]`\nExample: `/pim 904924507160`\n\nUse `/acc` to see available accounts."
			}

			_, _, err = h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
			if err != nil {
				log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send message")
				return err
			}
		}

		// Handle /acc command
		if cmd.Command == "/acc" {
			log.LogUserAction(cmd.UserID, "acc_command", logger.Fields{
				"command": cmd.Command,
				"channel_id": cmd.ChannelID,
			}).Info("Processing /acc command")
			
			// Acknowledge the command immediately
			client.Ack(*evt.Request)

		// Get all AWS accounts
		accounts, err := awspkg.GetAccounts()
		var message string
		
		if err != nil {
			log.LogSlackOperation("get_accounts", logger.Fields{
				"user_id": cmd.UserID,
				"channel_id": cmd.ChannelID,
			}).WithError(err).Error("Failed to fetch AWS accounts")
			message = "❌ Failed to load AWS accounts. Please try again later."
		} else if accounts != nil && len(accounts.Accounts) > 0 {
			message = fmt.Sprintf("📋 Available AWS Accounts (%d total):\n\n", len(accounts.Accounts))
			for i, account := range accounts.Accounts {
				message += fmt.Sprintf("%d. **%s**\n   ID: `%s`\n\n", i+1, account.AccountName, account.AccountId)
			}
		} else {
			message = "❌ No AWS accounts configured."
		}

			_, _, err = h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
			if err != nil {
				log.LogSlackOperation("send_accounts_list", logger.Fields{
					"channel_id": cmd.ChannelID,
					"user_id": cmd.UserID,
				}).WithError(err).Error("Failed to send accounts list message")
				return err
			}
		}
	}
	return nil
}

func (h *handler) HandleInteraction(evt *socketmode.Event, client *socketmode.Client) error {
	log := logger.GetDefaultLogger()
	cfg := config.Get()

	interaction, ok := evt.Data.(slack.InteractionCallback)
	if !ok {
		log.LogSlackOperation("handle_interaction", logger.Fields{"error": "type_cast_failed"}).Error("Could not type cast the event to InteractionCallback")
		return errors.NewSlackError("could not type cast event to InteractionCallback", nil)
	}

	// Acknowledge the interaction
	client.Ack(*evt.Request)

	// Handle button clicks
	if interaction.Type == slack.InteractionTypeBlockActions {
		for _, action := range interaction.ActionCallback.BlockActions {
				if action.ActionID == "approve_access" || action.ActionID == "deny_access" {
					log.LogUserAction(interaction.User.ID, action.ActionID, logger.Fields{
						"action_id": action.ActionID,
						"button_value": action.Value,
						"channel_id": interaction.Channel.ID,
					}).Info("Processing approval/denial button click")
					
					// Parse the button value: "userID:requestorUsername:accountID:accountName"
					parts := strings.Split(action.Value, ":")
					if len(parts) != 4 {
						log.LogSlackOperation("parse_button_value", logger.Fields{
							"button_value": action.Value,
							"expected_parts": 4,
							"actual_parts": len(parts),
						}).Error("Invalid button value format")
						continue
					}
				
				requestedUserID := parts[0]
				requestorUsername := parts[1]
				accountID := parts[2]
				accountName := parts[3]
				approverUserID := interaction.User.ID
				approverUser := interaction.User.Name
				if approverUser == "" {
					approverUser = fmt.Sprintf("<@%s>", interaction.User.ID)
				}

				// Check if the user is trying to approve their own request
				// Exception: admin users are allowed to self-approve
				if requestedUserID == approverUserID && !cfg.IsAdminUser(approverUser) {
						log.LogSecurityEvent("self_approval_attempt", logger.Fields{
							"user_id": approverUserID,
							"approver_user": approverUser,
							"requested_user_id": requestedUserID,
							"account_id": accountID,
							"action": action.ActionID,
						}).Warn("User attempted to self-approve access request")
						
						// Send an ephemeral message (only visible to the user who clicked)
						_, err := h.client.PostEphemeral(
							interaction.Channel.ID,
							approverUserID,
							slack.MsgOptionText("❌ *Self-Approval Not Allowed*\n\nYou cannot approve or deny your own access request. Another user must handle this request.", false),
						)
						if err != nil {
							log.LogSlackOperation("send_ephemeral", logger.Fields{"channel_id": interaction.Channel.ID}).WithError(err).Error("Failed to send ephemeral message")
						}
						continue // Skip processing this action
					}

				var responseText string
				var responseColor string
				
					if action.ActionID == "approve_access" {
						log.LogUserAction(approverUserID, "approve_access", logger.Fields{
							"requestor": requestorUsername,
							"approver": approverUser,
							"account_id": accountID,
							"account_name": accountName,
						}).Info("Access request approved")
						
						// Send SQS notification for approval using actual usernames
						err := awspkg.SendApprovalNotification(requestorUsername, approverUser, accountID)
						if err != nil {
							log.LogSlackOperation("send_sqs_notification", logger.Fields{
								"requestor": requestorUsername,
								"approver": approverUser,
								"account_id": accountID,
							}).WithError(err).Error("Failed to send SQS approval notification")
							// Continue with Slack update even if SQS fails
						}
					
					responseText = fmt.Sprintf("✅ *Access Approved*\n\nAdmin access to account *%s* (%s) has been *APPROVED* by *%s*.\n\n🎯 Access granted to <@%s>.\n\n📨 Notification sent to management system.", 
						accountID, accountName, approverUser, requestedUserID)
					responseColor = "good"
					} else {
						log.LogUserAction(approverUserID, "deny_access", logger.Fields{
							"requestor": requestorUsername,
							"approver": approverUser,
							"account_id": accountID,
							"account_name": accountName,
						}).Info("Access request denied")
						
						responseText = fmt.Sprintf("❌ *Access Denied*\n\nAdmin access to account *%s* (%s) has been *DENIED* by *%s*.\n\n🚫 Access not granted to <@%s>.", 
							accountID, accountName, approverUser, requestedUserID)
						responseColor = "danger"
					}

				// Update the original message to show the approval/denial
					_, _, _, err := h.client.UpdateMessage(
						interaction.Channel.ID,
						interaction.Message.Timestamp,
						slack.MsgOptionText(responseText, false),
						slack.MsgOptionAttachments(slack.Attachment{
							Color: responseColor,
							Text:  responseText,
						}),
					)
					if err != nil {
						log.LogSlackOperation("update_message", logger.Fields{
							"channel_id": interaction.Channel.ID,
							"message_timestamp": interaction.Message.Timestamp,
							"action": action.ActionID,
						}).WithError(err).Error("Failed to update message")
						return err
					}
			}
		}
	}

	return nil
}
