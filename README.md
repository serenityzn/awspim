# AWS PIM Management System

A Slack bot for managing AWS Privileged Identity Management (PIM) access requests with approval workflows.

## 🚀 Quick Start

1. **Configuration Setup:**
   ```bash
   # Option 1: Config file (recommended)
   cp config.yaml.example config.yaml
   # Edit config.yaml with your values
   
   # Option 2: Environment variables
   export SLACK_BOT_TOKEN="xoxb-your-token"
   export SLACK_APP_TOKEN="xapp-your-token"  
   export MANAGER_SQS_ARN="arn:aws:sqs:region:account:queue"
   ```

2. **Run the application:**
   ```bash
   go run main.go
   ```

## 📋 Features

- **Slack Integration**: Slash commands for requesting AWS access
- **Approval Workflow**: Interactive buttons for approve/deny decisions
- **AWS Integration**: SQS notifications and Secrets Manager for account data
- **Security**: Channel restrictions and admin user management
- **Logging**: Structured JSON logging for monitoring and debugging

## 🔧 Configuration

### Priority Order
1. **Config file** (highest priority)
2. **Environment variables** 
3. **Default values** (lowest priority)

### Config File Locations
The app searches for `config.yaml` in:
- `./config.yaml` (current directory)
- `./config/config.yaml` (config subdirectory)
- `/etc/awspim/config.yaml` (system-wide)

### Required Settings
```yaml
# Slack Configuration
slack_bot_token: "xoxb-your-slack-bot-token"
slack_app_token: "xapp-your-slack-app-token"

# AWS Configuration  
manager_sqs_arn: "arn:aws:sqs:us-east-2:123456789012:pim-queue"
```

### Optional Settings (with defaults)
```yaml
aws_region: "us-east-2"
aws_accounts_secret: "pim/aws-accounts"
environment: "development"
log_level: "info"
allowed_channel: "pim-management"
admin_users:
  - "volodymyr.l"
```

### Environment Variable Mapping
| Config Key | Environment Variables |
|------------|----------------------|
| `slack_bot_token` | `SLACK_BOT_TOKEN` or `AWSPIM_SLACK_BOT_TOKEN` |
| `slack_app_token` | `SLACK_APP_TOKEN` or `AWSPIM_SLACK_APP_TOKEN` |
| `manager_sqs_arn` | `MANAGER_SQS_ARN` or `AWSPIM_MANAGER_SQS_ARN` |
| `aws_region` | `AWS_REGION` or `AWSPIM_AWS_REGION` |
| `aws_accounts_secret` | `AWS_ACCOUNTS_SECRET` or `AWSPIM_AWS_ACCOUNTS_SECRET` |

## 🔐 AWS Setup

### 1. Secrets Manager Secret
Create a secret containing AWS account information:
```json
{
  "accounts": [
    {
      "accountid": "123456789012",
      "accountname": "Production Environment"
    },
    {
      "accountid": "210987654321", 
      "accountname": "Development Environment"
    }
  ]
}
```

### 2. SQS Queue
Create an SQS queue for approval notifications. The application will send messages with this structure:
```json
{
  "requestor": "username",
  "approver": "approver_username", 
  "account": "123456789012",
  "datetime": "2024-01-15 14:30"
}
```

### 3. IAM Permissions
The application needs:
- `secretsmanager:GetSecretValue` on the accounts secret
- `sqs:SendMessage` on the notification queue

## 📱 Slack Setup

### 1. Create Slack App
1. Go to [api.slack.com/apps](https://api.slack.com/apps)
2. Create new app "From scratch"
3. Choose your workspace

### 2. Configure OAuth & Permissions
Add these Bot Token Scopes:
- `chat:write`
- `commands`
- `channels:read`
- `users:read`

### 3. Enable Socket Mode
1. Go to Socket Mode settings
2. Enable Socket Mode
3. Generate App-Level Token with `connections:write` scope

### 4. Create Slash Commands
Create these slash commands:
- `/pim [account-id]` - Request access to AWS account
- `/acc` - List available AWS accounts

### 5. Configure Interactivity
1. Enable Interactivity & Shortcuts
2. Socket Mode handles the endpoint automatically

## 🎮 Usage

### Commands

**Request AWS Access:**
```
/pim 123456789012
```

**List Available Accounts:**
```
/acc
```

### Approval Workflow
1. User runs `/pim [account-id]` in `#pim-management` channel
2. Bot validates account ID and posts approval request with buttons
3. Another user clicks "✅ Approve Access" or "❌ Deny Access"
4. Bot updates message and sends SQS notification
5. External system processes the approval

### Security Features
- **Channel Restriction**: Commands only work in configured channel (default: `pim-management`)
- **No Self-Approval**: Users cannot approve their own requests (except configured admin users)
- **Account Validation**: Only valid account IDs from Secrets Manager are accepted
- **Audit Logging**: All actions are logged with user context

## 📊 Logging

### Log Format
All logs are output in JSON format for easy parsing:
```json
{
  "level": "info",
  "msg": "User requested access to valid AWS account",
  "component": "user_action",
  "action": "valid_account_request",
  "account_id": "123456789012",
  "user_id": "U1234567890",
  "time": "2024-01-15T14:30:25.123Z"
}
```

### Log Levels
Configure via `LOG_LEVEL` environment variable:
- `debug` - Detailed operation logs
- `info` - General operational messages (default)
- `warn` - Warning conditions
- `error` - Error conditions

### Log Components
- `aws` - AWS operations (Secrets Manager, SQS)
- `slack` - Slack operations (messages, interactions)
- `user_action` - User-initiated actions
- `security` - Security-related events

### Security Events Logged
- Unauthorized channel usage
- Invalid account requests  
- Self-approval attempts
- All approval/denial decisions

## 🏗️ Architecture

### Components
- **main.go** - Application entry point
- **pkg/config** - Configuration management with Viper
- **pkg/logger** - Structured logging with Logrus
- **pkg/aws** - AWS integrations (Secrets Manager, SQS)
- **pkg/slack** - Slack bot implementation

### Dependencies
- `github.com/slack-go/slack` - Slack SDK
- `github.com/aws/aws-sdk-go` - AWS SDK
- `github.com/sirupsen/logrus` - Structured logging
- `github.com/spf13/viper` - Configuration management

## 🔧 Development

### Build
```bash
go build -o awspim main.go
```

### Run with Debug Logging
```bash
LOG_LEVEL=debug go run main.go
```

### Environment Variables for Testing
```bash
export SLACK_BOT_TOKEN="xoxb-your-dev-token"
export SLACK_APP_TOKEN="xapp-your-dev-token"
export MANAGER_SQS_ARN="arn:aws:sqs:us-east-2:123456789012:dev-queue"
export LOG_LEVEL="debug"
go run main.go
```

## 🚨 Troubleshooting

### Config Issues
**Problem**: "Failed to load configuration" error
**Solution**: Check config file location and required fields

**Problem**: Environment variables not working
**Solution**: Ensure variables are exported: `export VAR=value`

### Slack Issues  
**Problem**: "SLACK_BOT_TOKEN not configured"
**Solution**: Verify token is correct and has proper scopes

**Problem**: Commands not working
**Solution**: Check if bot is in the correct channel and has permissions

### AWS Issues
**Problem**: "MANAGER_SQS_ARN not configured"  
**Solution**: Verify SQS ARN format and IAM permissions

**Problem**: "Failed to retrieve secret"
**Solution**: Check AWS credentials and secret name/permissions

## 📄 License

[Add your license information here]

## 🤝 Contributing

[Add contribution guidelines here]