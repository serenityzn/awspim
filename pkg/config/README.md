# Configuration Guide

AWS PIM now uses **Viper** for configuration management with the following priority order:

1. **Config file** (highest priority)
2. **Environment variables** 
3. **Default values** (lowest priority)

## 🔧 Setup Options

### Option 1: Config File (Recommended)
```bash
# Copy the example config file
cp config.yaml.example config.yaml

# Edit with your values
vim config.yaml
```

### Option 2: Environment Variables
```bash
# Required variables
export SLACK_BOT_TOKEN="xoxb-your-token"
export SLACK_APP_TOKEN="xapp-your-token"  
export MANAGER_SQS_ARN="https://sqs.us-east-2.amazonaws.com/123456789012/pim-queue"

# Optional variables (have defaults)
export AWS_REGION="us-east-2"
export AWS_ACCOUNTS_SECRET="pim/aws-accounts"
export LOG_LEVEL="info"
```

## 📋 Configuration Reference

### Required Settings
- `slack_bot_token` - Slack Bot User OAuth Token
- `slack_app_token` - Slack App-Level Token  
- `manager_sqs_arn` - SQS queue ARN for notifications

### Optional Settings
- `aws_region` - AWS region (default: `us-east-2`)
- `aws_accounts_secret` - Secrets Manager secret name (default: `pim/aws-accounts`)
- `environment` - App environment (default: `development`)
- `log_level` - Logging level (default: `info`)
- `allowed_channel` - Channel for PIM commands (default: `pim-management`)
- `admin_users` - Users who can self-approve (default: `[]`)

## 🌍 Environment Variable Mapping

The application supports both prefixed and non-prefixed environment variables:

| Config Key | Environment Variables |
|------------|----------------------|
| `slack_bot_token` | `AWSPIM_SLACK_BOT_TOKEN` or `SLACK_BOT_TOKEN` |
| `slack_app_token` | `AWSPIM_SLACK_APP_TOKEN` or `SLACK_APP_TOKEN` |
| `aws_region` | `AWSPIM_AWS_REGION` or `AWS_REGION` |
| `aws_accounts_secret` | `AWSPIM_AWS_ACCOUNTS_SECRET` or `AWS_ACCOUNTS_SECRET` |
| `manager_sqs_arn` | `AWSPIM_MANAGER_SQS_ARN` or `MANAGER_SQS_ARN` |
| `environment` | `AWSPIM_ENVIRONMENT` or `ENVIRONMENT` |
| `log_level` | `AWSPIM_LOG_LEVEL` or `LOG_LEVEL` |
| `allowed_channel` | `AWSPIM_ALLOWED_CHANNEL` |
| `admin_users` | `AWSPIM_ADMIN_USERS` (comma-separated) |

## 🚀 Quick Start

1. **Config file approach:**
   ```bash
   cp config.yaml.example config.yaml
   # Edit config.yaml with your values
   go run main.go
   ```

2. **Environment variables approach:**
   ```bash
   export SLACK_BOT_TOKEN="xoxb-your-token"
   export SLACK_APP_TOKEN="xapp-your-token"
   export MANAGER_SQS_ARN="https://sqs.us-east-2.amazonaws.com/123456789012/pim-queue"
   go run main.go
   ```

## ⚠️ Migration from Environment-Only

If you were using environment variables before, they will continue to work! The new system is backward compatible.

Your existing `.env` file setup will work, but consider migrating to `config.yaml` for better organization.
