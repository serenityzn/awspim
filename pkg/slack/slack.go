package slack

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	awspkg "github.com/serenityzn/awspim/pkg/aws"
	"github.com/serenityzn/awspim/pkg/auth"
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/errors"
	"github.com/serenityzn/awspim/pkg/logger"
	"github.com/serenityzn/awspim/pkg/utils"
)

type handler struct {
	client        *slack.Client
	authenticator *auth.TOTPEmailAuthenticator
	processedApprovals map[string]bool // Track processed approval requests
	approvalMutex      sync.RWMutex    // Protect the processed approvals map
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

	// Initialize TOTP Email authenticator if multi-factor auth is enabled
	var authenticator *auth.TOTPEmailAuthenticator
	if cfg.IsRequireMultiFactorAuth() {
		log.LogSlackOperation("mfa_initialization", logger.Fields{"action": "creating_totp_authenticator"}).Info("Initializing TOTP + Email authenticator")
		
		// Get SES client for email sending
		sessionManager, err := awspkg.GetSessionManager()
		if err != nil {
			log.LogSlackOperation("mfa_initialization", logger.Fields{"error": "failed_to_get_session_manager"}).WithError(err).Error("Failed to get AWS session manager")
			return errors.NewConfigurationError("failed to get AWS session manager", err)
		}
		
		sesClient := sessionManager.GetSESClient()
		authenticator = auth.NewTOTPEmailAuthenticator(sesClient)
		
		log.LogSlackOperation("mfa_initialization", logger.Fields{"action": "totp_authenticator_created"}).Info("TOTP + Email authenticator initialized successfully")
	} else {
		log.LogSlackOperation("mfa_initialization", logger.Fields{"action": "mfa_disabled"}).Info("Multi-factor authentication not enabled - using legacy approval flow")
	}

	// Create handler
	handler := NewHandler(client, authenticator)

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

func NewHandler(client *slack.Client, authenticator *auth.TOTPEmailAuthenticator) Handler {
	h := &handler{
		client:        client,
		authenticator: authenticator,
		processedApprovals: make(map[string]bool),
	}
	
	// Start cleanup routine for processed approvals
	go h.startApprovalCleanup()
	
	return h
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

		// Handle /register-totp command
		if cmd.Command == "/register-totp" {
			log.LogUserAction(cmd.UserID, "register_totp_command", logger.Fields{
				"command": cmd.Command,
				"text": cmd.Text,
				"channel_id": cmd.ChannelID,
			}).Info("Processing /register-totp command")

			// Acknowledge the command immediately
			client.Ack(*evt.Request)

			if h.authenticator == nil {
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText("❌ Multi-factor authentication is not enabled on this system.", false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send MFA disabled message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			// Get user's email from Slack profile
			userInfo, err := h.client.GetUserInfo(cmd.UserID)
			if err != nil {
				log.LogSlackOperation("get_user_info", logger.Fields{
					"user_id": cmd.UserID,
				}).WithError(err).Error("Failed to get user info from Slack")
				
				message := "❌ **Profile Access Error**\n\nUnable to retrieve your Slack profile information. Please ensure the bot has permission to access user profiles."
				_, _, msgErr := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if msgErr != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(msgErr).Error("Failed to send profile error message")
				}
				return errors.NewSlackError("failed to get user info", err).WithContext("user_id", cmd.UserID)
			}

			// Extract email from user profile
			email := userInfo.Profile.Email
			if email == "" {
				log.LogSecurityEvent("missing_email_in_profile", logger.Fields{
					"user_id": cmd.UserID,
					"user_name": utils.SanitizeUserInput(userInfo.Name),
				}).Warn("User has no email in Slack profile")
				
				message := "❌ **No Email Found**\n\nYour Slack profile doesn't have an email address configured. Please contact your Slack administrator to add an email to your profile."
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send no email message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			// Validate email format (just in case)
			if err := utils.ValidateEmail(email); err != nil {
				log.LogSecurityEvent("invalid_email_in_profile", logger.Fields{
					"user_id": cmd.UserID,
					"invalid_email": utils.SanitizeUserInput(email),
					"channel_id": cmd.ChannelID,
				}).WithError(err).Warn("User has invalid email format in Slack profile")
				
				message := fmt.Sprintf("❌ **Invalid Email in Profile**\n\nYour Slack profile email `%s` is not a valid format. Please contact your Slack administrator to update your email address.", utils.SanitizeUserInput(email))
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send invalid email message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			log.LogUserAction(cmd.UserID, "email_retrieved_from_profile", logger.Fields{
				"email": utils.SanitizeUserInput(email),
				"user_name": utils.SanitizeUserInput(userInfo.Name),
			}).Info("Successfully retrieved email from Slack profile")

			// Check if user is already registered
			if h.authenticator.IsUserRegistered(cmd.UserID) {
				message := "⚠️ **Already Registered**\n\nYou are already registered for multi-factor authentication.\n\nIf you need to re-register, please contact your administrator."
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send already registered message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			// Start TOTP registration
			key, err := h.authenticator.StartRegistration(cmd.UserID, email)
			if err != nil {
				log.LogSlackOperation("start_totp_registration", logger.Fields{
					"user_id": cmd.UserID,
					"email": utils.SanitizeUserInput(email),
				}).WithError(err).Error("Failed to start TOTP registration")
				
				message := fmt.Sprintf("❌ **Registration Failed**\n\nUnable to start TOTP registration for email `%s`. Please check your email domain is authorized and try again.", utils.SanitizeUserInput(email))
				_, _, msgErr := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if msgErr != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(msgErr).Error("Failed to send registration error message")
				}
				return nil
			}

			// Create registration completion modal
			modalRequest := slack.ModalViewRequest{
				Type: slack.VTModal,
				Title: &slack.TextBlockObject{
					Type: slack.PlainTextType,
					Text: "Complete TOTP Setup",
				},
				Blocks: slack.Blocks{
					BlockSet: []slack.Block{
						&slack.SectionBlock{
							Type: slack.MBTSection,
							Text: &slack.TextBlockObject{
								Type: slack.MarkdownType,
								Text: fmt.Sprintf("📧 *Email:* %s\n\n📱 *Step 1:* Scan this QR code with your authenticator app:\n\n`%s`\n\n🔐 *Step 2:* Enter your first TOTP code below to verify setup:", utils.SanitizeUserInput(email), key.URL()),
							},
						},
						&slack.InputBlock{
							Type:    slack.MBTInput,
							BlockID: "totp_code",
							Label: &slack.TextBlockObject{
								Type: slack.PlainTextType,
								Text: "TOTP Code (6 digits)",
							},
							Element: &slack.PlainTextInputBlockElement{
								Type:        slack.METPlainTextInput,
								ActionID:    "totp_code_input",
								Placeholder: &slack.TextBlockObject{
									Type: slack.PlainTextType,
									Text: "123456",
								},
								MaxLength: 6,
								MinLength: 6,
							},
						},
					},
				},
				Submit: &slack.TextBlockObject{
					Type: slack.PlainTextType,
					Text: "Complete Setup",
				},
				Close: &slack.TextBlockObject{
					Type: slack.PlainTextType,
					Text: "Cancel",
				},
				CallbackID:     "complete_totp_registration",
				ClearOnClose:   true,
				NotifyOnClose:  false,
			}

			_, err = h.client.OpenView(cmd.TriggerID, modalRequest)
			if err != nil {
				log.LogSlackOperation("open_registration_modal", logger.Fields{
					"user_id": cmd.UserID,
					"trigger_id": cmd.TriggerID,
				}).WithError(err).Error("Failed to open TOTP registration modal")
				
				message := "❌ **Modal Error**\n\nUnable to open registration modal. Please try the command again."
				_, _, msgErr := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if msgErr != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(msgErr).Error("Failed to send modal error message")
				}
				return nil
			}

			log.LogUserAction(cmd.UserID, "totp_registration_modal_opened", logger.Fields{
				"email": utils.SanitizeUserInput(email),
			}).Info("TOTP registration modal opened successfully")
		}

		// Handle /verify-approval command for MFA
		if cmd.Command == "/verify-approval" {
			log.LogUserAction(cmd.UserID, "verify_approval_command", logger.Fields{
				"command": cmd.Command,
				"channel_id": cmd.ChannelID,
			}).Info("Processing /verify-approval command")
			
			// Acknowledge the command immediately
			client.Ack(*evt.Request)

			// Check if MFA is enabled
			if h.authenticator == nil || !cfg.IsRequireMultiFactorAuth() {
				message := "❌ **MFA Not Enabled**\n\nMulti-factor authentication is not enabled on this system."
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send MFA disabled message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			// Parse parameters: /verify-approval <email-code> <totp-code>
			params := strings.Fields(cmd.Text)
			if len(params) != 2 {
				message := "❌ **Invalid Usage**\n\nUsage: `/verify-approval <email-code> <totp-code>`\n\nExample: `/verify-approval 123456 654321`"
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send usage message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			emailCode := strings.TrimSpace(params[0])
			totpCode := strings.TrimSpace(params[1])

			// Validate codes format
			if len(emailCode) != 6 || len(totpCode) != 6 {
				message := "❌ **Invalid Code Format**\n\nBoth email and TOTP codes must be exactly 6 digits."
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send invalid format message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			// Check if user has pending verification
			if !h.authenticator.IsVerificationPending(cmd.UserID) {
				message := "❌ **No Pending Verification**\n\nYou don't have any pending approval verification. Please click an approval button first."
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if err != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send no pending message")
					return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
				}
				return nil
			}

			// Verify the codes and get the approval data
			verification, err := h.authenticator.VerifyApproval(cmd.UserID, emailCode, totpCode)
			if err != nil {
				log.LogSecurityEvent("approval_verification_failed", logger.Fields{
					"user_id": cmd.UserID,
					"error": err.Error(),
				}).WithError(err).Error("Approval verification failed via command")

				message := "❌ **Verification Failed**\n\nInvalid email or TOTP code. Please check your codes and try again."
				_, _, msgErr := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if msgErr != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(msgErr).Error("Failed to send verification failure message")
				}
				return nil
			}

			// Extract approval data from verification
			approvalData := verification.ApprovalData
			requestedUserID, _ := approvalData["requestor_user_id"].(string)
			requestorUsername, _ := approvalData["requestor"].(string)
			accountID, _ := approvalData["account_id"].(string)
			accountName, _ := approvalData["account_name"].(string)
			approverUsername, _ := approvalData["approver_username"].(string)

			// Process the approval
			err = h.processApproval(requestedUserID, requestorUsername, accountID, accountName, cmd.UserID, approverUsername)
			if err != nil {
				log.LogSlackOperation("process_approval_after_mfa", logger.Fields{
					"account_id": accountID,
					"approver_user_id": cmd.UserID,
				}).WithError(err).Error("Failed to process approval after MFA verification")
				
				message := "❌ **Processing Failed**\n\nVerification successful but approval processing failed. Please contact administrator."
				_, _, msgErr := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
				if msgErr != nil {
					log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(msgErr).Error("Failed to send processing error message")
				}
				return nil
			}

			// Success message with details
			message := fmt.Sprintf("✅ **Approval Verified & Processed Successfully!**\n\n**Account:** %s (%s)\n**Requestor:** %s\n**Approved by:** %s", 
				utils.SanitizeUserInput(accountName), 
				utils.SanitizeUserInput(accountID), 
				utils.SanitizeUserInput(requestorUsername), 
				utils.SanitizeUserInput(approverUsername))
			
			_, _, err = h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
			if err != nil {
				log.LogSlackOperation("send_message", logger.Fields{"channel_id": cmd.ChannelID}).WithError(err).Error("Failed to send verification success message")
				return errors.NewSlackError("failed to send message", err).WithContext("channel_id", cmd.ChannelID)
			}

			log.LogUserAction(cmd.UserID, "approval_verification_success", logger.Fields{
				"channel_id": cmd.ChannelID,
				"account_id": accountID,
				"requestor": requestorUsername,
			}).Info("Approval verification completed successfully via command")

			return nil
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

	// Handle modal submissions (for TOTP registration)
	if interaction.Type == slack.InteractionTypeViewSubmission {
		// Acknowledge the interaction
		client.Ack(*evt.Request)
		return h.handleModalSubmission(interaction)
	}

	// Handle button clicks - DON'T acknowledge yet, we might need to respond with modal
	if interaction.Type == slack.InteractionTypeBlockActions {
		for _, action := range interaction.ActionCallback.BlockActions {
				if action.ActionID == "open_mfa_modal" {
					// Handle the "Open Verification Form" button click
					return h.handleMFAModalTrigger(action, interaction, client, evt)
				}
				
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
						// Create unique approval ID to prevent duplicate processing
						approvalID := fmt.Sprintf("%s:%s:%s:%s", requestedUserID, accountID, approverUserID, interaction.Message.Timestamp)
						
						// Check if this approval has already been processed
						h.approvalMutex.RLock()
						alreadyProcessed := h.processedApprovals[approvalID]
						h.approvalMutex.RUnlock()
						
						if alreadyProcessed {
							log.LogSecurityEvent("duplicate_approval_attempt", logger.Fields{
								"approval_id": approvalID,
								"approver_user_id": approverUserID,
								"account_id": accountID,
								"requestor": requestorUsername,
							}).Warn("Duplicate approval attempt detected")
							
							// Send ephemeral message to the user  
							client.Ack(*evt.Request)
							
							_, err := h.client.PostEphemeral(
								interaction.Channel.ID,
								approverUserID,
								slack.MsgOptionText("⚠️ **Already Processed**\n\nThis approval request has already been processed. No further action is needed.", false),
							)
							if err != nil {
								log.LogSlackOperation("send_duplicate_warning", logger.Fields{"channel_id": interaction.Channel.ID}).WithError(err).Error("Failed to send duplicate approval warning")
							}
							return nil // Skip processing this action
						}
						
						// Mark this approval as being processed (optimistic locking)
						h.approvalMutex.Lock()
						h.processedApprovals[approvalID] = true
						h.approvalMutex.Unlock()
						
						// Check if MFA is enabled and user is registered
						if h.authenticator != nil && cfg.IsRequireMultiFactorAuth() {
							// Check if approver is registered for MFA
							if h.authenticator.IsUserRegistered(approverUserID) {
								// Start MFA verification flow with approval context
								approvalDataMap := map[string]interface{}{
									"requestor_user_id": requestedUserID,
									"requestor": requestorUsername,  // Used in email template
									"account_id": accountID,         // Used in email template
									"account_name": accountName,     // Used in email template
									"approver_user_id": approverUserID,
									"approver_username": approverUser,
								}
								
								_, err := h.authenticator.InitiateApproval(approverUserID, approvalDataMap)
								if err != nil {
									log.LogSecurityEvent("mfa_verification_start_failed", logger.Fields{
										"approver_user_id": approverUserID,
										"account_id": accountID,
										"error": err.Error(),
									}).WithError(err).Error("Failed to start MFA verification")
									
									// Acknowledge with error message
									client.Ack(*evt.Request)
									return nil
								}
								
								// Store approval data for the modal button
								approvalData := fmt.Sprintf("%s:%s:%s:%s", requestedUserID, requestorUsername, accountID, accountName)
								
								// For button interactions, we need to use a different approach
								// The most reliable method is to acknowledge with a response that triggers a modal
								
								// First, acknowledge the interaction
								client.Ack(*evt.Request)
								
								// Since direct modal opening from button clicks has limitations,
								// let's use an immediate follow-up approach:
								// Send an ephemeral message with a button that opens the modal
								
								message := fmt.Sprintf("🔐 **Multi-Factor Authentication Required**\n\n**Account:** %s (%s)\n**Requestor:** %s\n\n📧 Email verification sent! Click below to open verification form:", 
									utils.SanitizeUserInput(accountName), 
									utils.SanitizeUserInput(accountID), 
									utils.SanitizeUserInput(requestorUsername))
								
								// Create a button that will trigger the modal
								modalButton := slack.NewButtonBlockElement("open_mfa_modal", approvalData, slack.NewTextBlockObject("plain_text", "🔐 Open Verification Form", false, false))
								modalButton.Style = slack.StylePrimary
								
								actionBlock := slack.NewActionBlock("mfa_modal_trigger", modalButton)
								
								fallbackText := fmt.Sprintf("Use command: /verify-approval <email-code> <totp-code>")
								
								blocks := []slack.Block{
									slack.NewSectionBlock(
										slack.NewTextBlockObject("mrkdwn", message, false, false),
										nil, nil,
									),
									actionBlock,
									slack.NewSectionBlock(
										slack.NewTextBlockObject("mrkdwn", "💡 *Alternative:* Use `/verify-approval <email-code> <totp-code>` command", false, false),
										nil, nil,
									),
								}
								
								_, err = h.client.PostEphemeral(
									interaction.Channel.ID,
									approverUserID,
									slack.MsgOptionText(fallbackText, false),
									slack.MsgOptionBlocks(blocks...),
								)
								if err != nil {
									log.LogSlackOperation("send_mfa_trigger", logger.Fields{
										"channel_id": interaction.Channel.ID,
										"approver_user_id": approverUserID,
									}).WithError(err).Error("Failed to send MFA trigger message")
								}
								
								log.LogSecurityEvent("mfa_modal_opened_from_button", logger.Fields{
									"approver_user_id": approverUserID,
									"account_id": accountID,
									"requestor": requestorUsername,
								}).Info("MFA verification modal opened from button click")
								
								return nil // Don't continue processing - modal will handle verification
							} else {
								// Approver not registered for MFA
								client.Ack(*evt.Request)
								
								_, err := h.client.PostEphemeral(
									interaction.Channel.ID,
									approverUserID,
									slack.MsgOptionText("❌ **MFA Registration Required**\n\nYou must register for multi-factor authentication before approving requests.\n\nUse `/register-totp` to get started.", false),
								)
								if err != nil {
									log.LogSlackOperation("send_mfa_required_message", logger.Fields{"channel_id": interaction.Channel.ID}).WithError(err).Error("Failed to send MFA required message")
								}
								return nil
							}
						}
						
						// Legacy approval flow (when MFA is disabled)
						err := h.processApproval(requestedUserID, requestorUsername, accountID, accountName, approverUserID, approverUser)
						if err != nil {
							log.LogSlackOperation("process_legacy_approval", logger.Fields{
								"account_id": accountID,
								"approver_user_id": approverUserID,
							}).WithError(err).Error("Failed to process legacy approval")
							client.Ack(*evt.Request)
							return nil
						}
					
					responseText = fmt.Sprintf("✅ *Access Approved*\n\nAdmin access to account *%s* (%s) has been *APPROVED* by *%s*.\n\n🎯 Access granted to <@%s>.\n\n📨 Notification sent to management system.", 
						accountID, accountName, approverUser, requestedUserID)
					responseColor = "good"
					} else if action.ActionID == "deny_access" {
						// Create unique approval ID to prevent duplicate processing
						approvalID := fmt.Sprintf("%s:%s:%s:%s", requestedUserID, accountID, approverUserID, interaction.Message.Timestamp)
						
						// Check if this approval has already been processed
						h.approvalMutex.RLock()
						alreadyProcessed := h.processedApprovals[approvalID]
						h.approvalMutex.RUnlock()
						
						if alreadyProcessed {
							log.LogSecurityEvent("duplicate_denial_attempt", logger.Fields{
								"approval_id": approvalID,
								"approver_user_id": approverUserID,
								"account_id": accountID,
								"requestor": requestorUsername,
							}).Warn("Duplicate denial attempt detected")
							
							// Send ephemeral message to the user
							client.Ack(*evt.Request)
							
							_, err := h.client.PostEphemeral(
								interaction.Channel.ID,
								approverUserID,
								slack.MsgOptionText("⚠️ **Already Processed**\n\nThis request has already been processed. No further action is needed.", false),
							)
							if err != nil {
								log.LogSlackOperation("send_duplicate_warning", logger.Fields{"channel_id": interaction.Channel.ID}).WithError(err).Error("Failed to send duplicate denial warning")
							}
							return nil // Skip processing this action
						}
						
						// Mark this approval as being processed
						h.approvalMutex.Lock()
						h.processedApprovals[approvalID] = true
						h.approvalMutex.Unlock()
						
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

				// Acknowledge the interaction
				client.Ack(*evt.Request)
				
				// Update the original message to show the approval/denial with disabled buttons
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

// handleModalSubmission handles modal form submissions for TOTP registration
func (h *handler) handleModalSubmission(interaction slack.InteractionCallback) error {
	log := logger.GetDefaultLogger()

	// Handle TOTP registration completion
	if interaction.View.CallbackID == "complete_totp_registration" {
		return h.handleTOTPRegistrationCompletion(interaction)
	}

	// Handle approval verification (when MFA is enabled)
	if interaction.View.CallbackID == "verify_approval" {
		return h.handleApprovalVerification(interaction)
	}

	log.LogSlackOperation("unknown_modal", logger.Fields{
		"callback_id": interaction.View.CallbackID,
		"user_id": interaction.User.ID,
	}).Warn("Unknown modal submission callback ID")

	return nil
}

// handleTOTPRegistrationCompletion processes the TOTP registration modal submission
func (h *handler) handleTOTPRegistrationCompletion(interaction slack.InteractionCallback) error {
	log := logger.GetDefaultLogger()

	// Extract TOTP code from modal
	totpCode := ""
	if inputBlock, exists := interaction.View.State.Values["totp_code"]; exists {
		if inputElement, exists := inputBlock["totp_code_input"]; exists {
			totpCode = strings.TrimSpace(inputElement.Value)
		}
	}

	if totpCode == "" {
		log.LogSecurityEvent("empty_totp_code", logger.Fields{
			"user_id": interaction.User.ID,
		}).Warn("User submitted empty TOTP code")

		// Since this is a modal submission, we can't send a regular message
		// The user will need to try again
		return nil
	}

	// Validate TOTP code format
	if len(totpCode) != 6 {
		log.LogSecurityEvent("invalid_totp_code_length", logger.Fields{
			"user_id": interaction.User.ID,
			"code_length": len(totpCode),
		}).Warn("User submitted TOTP code with invalid length")
		return nil
	}

	// Complete TOTP registration - since we verified the email by getting it from Slack profile,
	// we can complete registration with just the TOTP code
	err := h.authenticator.CompleteRegistration(interaction.User.ID, "", totpCode)
	if err != nil {
		log.LogSecurityEvent("totp_registration_failed", logger.Fields{
			"user_id": interaction.User.ID,
			"error": err.Error(),
		}).WithError(err).Error("TOTP registration completion failed")

		// Send failure message to user
		message := "❌ **Registration Failed**\n\nInvalid TOTP code. Please try `/register-totp` again and ensure you're using the correct authenticator app."
		_, _, msgErr := h.client.PostMessage(interaction.User.ID, slack.MsgOptionText(message, false))
		if msgErr != nil {
			log.LogSlackOperation("send_dm", logger.Fields{
				"user_id": interaction.User.ID,
			}).WithError(msgErr).Error("Failed to send registration failure DM")
		}
		return nil
	}

	// Get the registered user to show backup codes
	user := h.authenticator.GetRegisteredUser(interaction.User.ID)
	if user == nil {
		log.LogSlackOperation("get_registered_user", logger.Fields{
			"user_id": interaction.User.ID,
		}).Error("Unable to retrieve registered user after successful registration")
		return errors.NewInternalError("failed to retrieve user registration", nil)
	}

	log.LogUserAction(interaction.User.ID, "totp_registration_completed", logger.Fields{
		"email": utils.SanitizeUserInput(user.Email),
	}).Info("TOTP registration completed successfully")

	// Send success message with backup codes
	backupCodesText := ""
	for i, code := range user.BackupCodes {
		backupCodesText += fmt.Sprintf("%d. `%s`\n", i+1, code)
	}

	successMessage := fmt.Sprintf(`✅ **TOTP Registration Complete!**

🔐 Your multi-factor authentication is now active for email: **%s**

📱 **Next Steps:**
• Your authenticator app is now configured
• Use it for all future approval requests
• Save these backup codes in a secure location

🔑 **Backup Codes** (Single-use only):
%s

⚠️ **Important:** Store these backup codes securely - they can only be used once each.`, 
		utils.SanitizeUserInput(user.Email), 
		backupCodesText)

	_, _, err = h.client.PostMessage(interaction.User.ID, slack.MsgOptionText(successMessage, false))
	if err != nil {
		log.LogSlackOperation("send_success_dm", logger.Fields{
			"user_id": interaction.User.ID,
		}).WithError(err).Error("Failed to send registration success DM")
		return errors.NewSlackError("failed to send success message", err)
	}

	return nil
}

// handleApprovalVerification processes approval verification when MFA is required
func (h *handler) handleApprovalVerification(interaction slack.InteractionCallback) error {
	log := logger.GetDefaultLogger()

	// Extract email code and TOTP code from modal
	emailCode := ""
	totpCode := ""
	
	if inputBlock, exists := interaction.View.State.Values["email_code"]; exists {
		if inputElement, exists := inputBlock["email_code_input"]; exists {
			emailCode = strings.TrimSpace(inputElement.Value)
		}
	}
	
	if inputBlock, exists := interaction.View.State.Values["totp_code"]; exists {
		if inputElement, exists := inputBlock["totp_code_input"]; exists {
			totpCode = strings.TrimSpace(inputElement.Value)
		}
	}

	// Get approval context from private metadata (we'll set this when creating the modal)
	approvalData := interaction.View.PrivateMetadata
	parts := strings.Split(approvalData, ":")
	if len(parts) != 4 {
		log.LogSlackOperation("parse_approval_data", logger.Fields{
			"approval_data": approvalData,
			"expected_parts": 4,
			"actual_parts": len(parts),
		}).Error("Invalid approval data format")
		return nil
	}

	requestedUserID := parts[0]
	requestorUsername := parts[1]
	accountID := parts[2]
	accountName := parts[3]
	approverUserID := interaction.User.ID

	// Verify the approval using MFA
	_, err := h.authenticator.VerifyApproval(approverUserID, emailCode, totpCode)
	if err != nil {
		log.LogSecurityEvent("approval_verification_failed", logger.Fields{
			"approver_user_id": approverUserID,
			"account_id": accountID,
			"error": err.Error(),
		}).WithError(err).Error("Approval verification failed")

		// Send failure message
		message := "❌ **Verification Failed**\n\nInvalid email or TOTP code. Please try the approval again."
		_, _, msgErr := h.client.PostMessage(interaction.User.ID, slack.MsgOptionText(message, false))
		if msgErr != nil {
			log.LogSlackOperation("send_verification_failure_dm", logger.Fields{
				"user_id": interaction.User.ID,
			}).WithError(msgErr).Error("Failed to send verification failure DM")
		}
		return nil
	}

	// Process the approval (similar to existing approval logic)
	return h.processApproval(requestedUserID, requestorUsername, accountID, accountName, approverUserID, interaction.User.Name)
}

// processApproval handles the actual approval logic (extracted from existing code)
func (h *handler) processApproval(requestedUserID, requestorUsername, accountID, accountName, approverUserID, approverUsername string) error {
	log := logger.GetDefaultLogger()

	log.LogUserAction(approverUserID, "approve_access", logger.Fields{
		"account_id": accountID,
		"account_name": accountName,
		"requestor": requestorUsername,
		"approver": approverUsername,
	}).Info("Access request approved")

	// Send approval notification
	if err := awspkg.SendApprovalNotification(requestorUsername, approverUsername, accountID); err != nil {
		log.LogSlackOperation("send_sqs_notification", logger.Fields{
			"account_id": accountID,
			"requestor": requestorUsername,
			"approver": approverUsername,
		}).WithError(err).Error("Failed to send SQS approval notification")
	}

	approvalMessage := fmt.Sprintf("✅ **Access Approved**\n\n**Account:** %s (%s)\n**Requestor:** %s\n**Approved by:** %s",
		utils.SanitizeUserInput(accountName),
		utils.SanitizeUserInput(accountID),
		utils.SanitizeUserInput(requestorUsername),
		utils.SanitizeUserInput(approverUsername))

	_, _, err := h.client.PostMessage(requestedUserID, slack.MsgOptionText(approvalMessage, false))
	if err != nil {
		log.LogSlackOperation("send_approval_dm", logger.Fields{
			"user_id": requestedUserID,
		}).WithError(err).Error("Failed to send approval DM")
		return errors.NewSlackError("failed to send approval message", err)
	}

	return nil
}

// startApprovalCleanup periodically cleans up old processed approvals to prevent memory growth
func (h *handler) startApprovalCleanup() {
	log := logger.GetDefaultLogger()
	
	// Clean up processed approvals every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		h.approvalMutex.Lock()
		// For simplicity, clear all processed approvals every hour
		// In production, you might want to implement TTL-based cleanup
		approvalCount := len(h.processedApprovals)
		h.processedApprovals = make(map[string]bool)
		h.approvalMutex.Unlock()
		
		if approvalCount > 0 {
			log.LogSlackOperation("cleanup_processed_approvals", logger.Fields{
				"cleared_count": approvalCount,
			}).Info("Cleaned up processed approvals cache")
		}
	}
}

// handleMFAModalTrigger handles the "Open Verification Form" button click to show modal  
func (h *handler) handleMFAModalTrigger(action *slack.BlockAction, interaction slack.InteractionCallback, client *socketmode.Client, evt *socketmode.Event) error {
	log := logger.GetDefaultLogger()
	
	// Parse approval data from button value
	approvalData := action.Value
	parts := strings.Split(approvalData, ":")
	if len(parts) != 4 {
		client.Ack(*evt.Request)
		log.LogSlackOperation("parse_mfa_modal_data", logger.Fields{
			"approval_data": approvalData,
			"expected_parts": 4,
			"actual_parts": len(parts),
		}).Error("Invalid MFA modal approval data format")
		return nil
	}
	
	_ = parts[0] // requestedUserID - not used in modal creation
	requestorUsername := parts[1]
	accountID := parts[2]
	accountName := parts[3]
	
	// Create the modal
	modalView := slack.ModalViewRequest{
		Type: slack.VTModal,
		Title: &slack.TextBlockObject{
			Type: slack.PlainTextType,
			Text: "Verify Approval",
		},
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				&slack.SectionBlock{
					Type: slack.MBTSection,
					Text: &slack.TextBlockObject{
						Type: slack.MarkdownType,
						Text: fmt.Sprintf("🔐 **Multi-Factor Authentication Required**\n\n**Account:** %s (%s)\n**Requestor:** %s\n\n📧 Check your email for verification code\n🔑 Enter codes below to approve access:", utils.SanitizeUserInput(accountName), utils.SanitizeUserInput(accountID), utils.SanitizeUserInput(requestorUsername)),
					},
				},
				&slack.InputBlock{
					Type:    slack.MBTInput,
					BlockID: "email_code",
					Label: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "Email Verification Code",
					},
					Element: &slack.PlainTextInputBlockElement{
						Type:        slack.METPlainTextInput,
						ActionID:    "email_code_input",
						Placeholder: &slack.TextBlockObject{
							Type: slack.PlainTextType,
							Text: "123456",
						},
						MaxLength: 6,
						MinLength: 6,
					},
				},
				&slack.InputBlock{
					Type:    slack.MBTInput,
					BlockID: "totp_code",
					Label: &slack.TextBlockObject{
						Type: slack.PlainTextType,
						Text: "TOTP Code (from authenticator app)",
					},
					Element: &slack.PlainTextInputBlockElement{
						Type:        slack.METPlainTextInput,
						ActionID:    "totp_code_input",
						Placeholder: &slack.TextBlockObject{
							Type: slack.PlainTextType,
							Text: "123456",
						},
						MaxLength: 6,
						MinLength: 6,
					},
				},
			},
		},
		Submit: &slack.TextBlockObject{
			Type: slack.PlainTextType,
			Text: "Approve Access",
		},
		Close: &slack.TextBlockObject{
			Type: slack.PlainTextType,
			Text: "Cancel",
		},
		CallbackID:      "verify_approval",
		PrivateMetadata: approvalData,
		ClearOnClose:    true,
		NotifyOnClose:   false,
	}
	
	// Open the modal using the trigger ID from the button interaction
	_, err := h.client.OpenView(interaction.TriggerID, modalView)
	if err != nil {
		client.Ack(*evt.Request)
		log.LogSlackOperation("open_mfa_modal_from_button", logger.Fields{
			"user_id": interaction.User.ID,
			"trigger_id": interaction.TriggerID,
		}).WithError(err).Error("Failed to open MFA modal from button")
		
		// Send fallback message
		fallbackMessage := fmt.Sprintf("❌ **Modal Failed**\n\nUnable to open verification form. Use `/verify-approval <email-code> <totp-code>` command instead.")
		_, msgErr := h.client.PostEphemeral(
			interaction.Channel.ID,
			interaction.User.ID,
			slack.MsgOptionText(fallbackMessage, false),
		)
		if msgErr != nil {
			log.LogSlackOperation("send_modal_fallback", logger.Fields{
				"channel_id": interaction.Channel.ID,
				"user_id": interaction.User.ID,
			}).WithError(msgErr).Error("Failed to send modal fallback message")
		}
		return nil
	}
	
	// Acknowledge the successful interaction
	client.Ack(*evt.Request)
	
	log.LogSecurityEvent("mfa_modal_opened_successfully", logger.Fields{
		"user_id": interaction.User.ID,
		"account_id": accountID,
		"requestor": requestorUsername,
	}).Info("MFA verification modal opened successfully from button")
	
	return nil
}
