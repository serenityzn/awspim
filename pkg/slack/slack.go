package slack

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	awspkg "github.com/serenityzn/awspim/pkg/aws"
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
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN environment variable is not set")
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
	token := os.Getenv("SLACK_BOT_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")

	if token == "" || appToken == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN and SLACK_APP_TOKEN environment variables must be set")
	}

	client := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))

	socketClient := socketmode.New(
		client,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	// Create handler
	handler := NewHandler(client)

	// Handle events
	go func() {
		for evt := range socketClient.Events {
			switch evt.Type {
			case socketmode.EventTypeConnecting:
				log.Println("Connecting to Slack with Socket Mode...")
			case socketmode.EventTypeConnectionError:
				log.Println("Connection failed. Retrying later...")
			case socketmode.EventTypeConnected:
				log.Println("Connected to Slack with Socket Mode.")
			case socketmode.EventTypeSlashCommand:
				if err := handler.HandleCommand(&evt, socketClient); err != nil {
					log.Printf("Error handling command: %v", err)
				}
			case socketmode.EventTypeInteractive:
				if err := handler.HandleInteraction(&evt, socketClient); err != nil {
					log.Printf("Error handling interaction: %v", err)
				}
			case socketmode.EventTypeEventsAPI:
				eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					log.Printf("Could not type cast the event to the EventsAPIEvent: %v\n", evt)
					continue
				}
				socketClient.Ack(*evt.Request, eventsAPIEvent)
			}
		}
	}()

	log.Println("Starting Slack bot...")
	socketClient.Run()
	return nil
}

func NewHandler(client *slack.Client) Handler {
	return &handler{
		client: client,
	}
}

func (h *handler) HandleCommand(evt *socketmode.Event, client *socketmode.Client) error {
	switch evt.Type {
	case socketmode.EventTypeSlashCommand:
		cmd, ok := evt.Data.(slack.SlashCommand)
		if !ok {
			log.Printf("Could not type cast the event to a SlashCommand: %v\n", evt)
			return fmt.Errorf("could not type cast event to SlashCommand")
		}

		// Handle /pim command
		if cmd.Command == "/pim" {
			// Acknowledge the command immediately
			client.Ack(*evt.Request)

			// Check if command is used in the correct channel
			channelInfo, err := h.client.GetConversationInfo(&slack.GetConversationInfoInput{
				ChannelID: cmd.ChannelID,
			})
			if err != nil {
				log.Printf("Failed to get channel info: %v", err)
				h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText("Error: Could not verify channel information.", false))
				return err
			}

			if channelInfo.Name != "pim-management" {
				_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText("This command can only be used in the #pim-management channel.", false))
				if err != nil {
					log.Printf("Failed to send error message: %v", err)
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
						log.Printf("Failed to send message with buttons: %v", err)
						return err
					}
					return nil // Return early since we've already sent the message
				} else {
					message = fmt.Sprintf("❌ **Invalid Account ID**\n\nAccount ID `%s` is not found in our system.\n\nUse `/acc` to see available accounts.", parameter)
				}
			} else {
				message = "❌ **Missing Account ID**\n\nPlease provide an AWS Account ID.\n\nUsage: `/pim [account-id]`\nExample: `/pim 904924507160`\n\nUse `/acc` to see available accounts."
			}

			_, _, err = h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
			if err != nil {
				log.Printf("Failed to send message: %v", err)
				return err
			}
		}

		// Handle /acc command
		if cmd.Command == "/acc" {
			// Acknowledge the command immediately
			client.Ack(*evt.Request)

			// Get all AWS accounts
			accounts := awspkg.GetAccounts()
			var message string
			
			if accounts != nil && len(accounts.Accounts) > 0 {
				message = fmt.Sprintf("📋 Available AWS Accounts (%d total):\n\n", len(accounts.Accounts))
				for i, account := range accounts.Accounts {
					message += fmt.Sprintf("%d. **%s**\n   ID: `%s`\n\n", i+1, account.AccountName, account.AccountId)
				}
			} else {
				message = "❌ No AWS accounts available or failed to load accounts."
			}

			_, _, err := h.client.PostMessage(cmd.ChannelID, slack.MsgOptionText(message, false))
			if err != nil {
				log.Printf("Failed to send message: %v", err)
				return err
			}
		}
	}
	return nil
}

func (h *handler) HandleInteraction(evt *socketmode.Event, client *socketmode.Client) error {
	interaction, ok := evt.Data.(slack.InteractionCallback)
	if !ok {
		log.Printf("Could not type cast the event to InteractionCallback: %v\n", evt)
		return fmt.Errorf("could not type cast event to InteractionCallback")
	}

	// Acknowledge the interaction
	client.Ack(*evt.Request)

	// Handle button clicks
	if interaction.Type == slack.InteractionTypeBlockActions {
		for _, action := range interaction.ActionCallback.BlockActions {
			if action.ActionID == "approve_access" || action.ActionID == "deny_access" {
				// Parse the button value: "userID:requestorUsername:accountID:accountName"
				parts := strings.Split(action.Value, ":")
				if len(parts) != 4 {
					log.Printf("Invalid button value format: %s", action.Value)
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
				// Exception: 'volodymyr.l' is allowed to self-approve
				if requestedUserID == approverUserID && approverUser != "volodymyr.l" {
					// Send an ephemeral message (only visible to the user who clicked)
					_, err := h.client.PostEphemeral(
						interaction.Channel.ID,
						approverUserID,
						slack.MsgOptionText("❌ *Self-Approval Not Allowed*\n\nYou cannot approve or deny your own access request. Another user must handle this request.", false),
					)
					if err != nil {
						log.Printf("Failed to send ephemeral message: %v", err)
					}
					continue // Skip processing this action
				}

				var responseText string
				var responseColor string
				
				if action.ActionID == "approve_access" {
					// Send SQS notification for approval using actual usernames
					err := awspkg.SendApprovalNotification(requestorUsername, approverUser, accountID)
					if err != nil {
						log.Printf("Failed to send SQS approval notification: %v", err)
						// Continue with Slack update even if SQS fails
					}
					
					responseText = fmt.Sprintf("✅ *Access Approved*\n\nAdmin access to account *%s* (%s) has been *APPROVED* by *%s*.\n\n🎯 Access granted to <@%s>.\n\n📨 Notification sent to management system.", 
						accountID, accountName, approverUser, requestedUserID)
					responseColor = "good"
				} else {
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
					log.Printf("Failed to update message: %v", err)
					return err
				}
			}
		}
	}

	return nil
}
