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

[GNU General Public License v3.0]

## 🤝 Contributing

We welcome contributions to AWS PIM! Please follow these guidelines to ensure a smooth collaboration process.

### 🚀 Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/your-username/awspim.git
   cd awspim
   ```
3. **Create a feature branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

### 🛠️ Development Setup

1. **Install Go 1.23.3 or later**
2. **Install dependencies**:
   ```bash
   go mod tidy
   ```
3. **Set up your environment**:
   ```bash
   cp config.yaml.example config.yaml
   # Edit config.yaml with your test values
   ```
4. **Run the application**:
   ```bash
   go run main.go
   ```

### 📝 Code Standards

#### **Go Style Guidelines**
- Follow [Effective Go](https://golang.org/doc/effective_go.html) principles
- Use `gofmt` for code formatting
- Run `go vet` to check for issues
- Use meaningful variable and function names
- Add comments for exported functions and complex logic

#### **Project Structure**
```
pkg/
├── config/     # Configuration management
├── logger/     # Structured logging
├── aws/        # AWS integrations
└── slack/      # Slack bot functionality
```

#### **Naming Conventions**
- **Functions**: `CamelCase` for exported, `camelCase` for internal
- **Variables**: `camelCase`
- **Constants**: `UPPER_SNAKE_CASE`
- **Files**: `snake_case.go`

### 🧪 Testing

#### **Running Tests**
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for specific package
go test ./pkg/config
```

#### **Writing Tests**
- Place tests in `*_test.go` files
- Use table-driven tests where appropriate
- Mock external dependencies (AWS, Slack)
- Test both success and error cases

**Example test structure:**
```go
func TestConfigLoad(t *testing.T) {
    tests := []struct {
        name    string
        setup   func()
        want    *Config
        wantErr bool
    }{
        // Test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### 🔒 Security Guidelines

- **Never commit secrets** or credentials
- **Validate all inputs** from Slack and external sources
- **Use structured logging** for security events
- **Follow least privilege** principle for AWS permissions
- **Sanitize user inputs** before logging or processing

### 📊 Logging Standards

Use the structured logger for all log messages:
```go
log := logger.GetDefaultLogger()

// Good examples
log.LogUserAction(userID, "action_name", logger.Fields{
    "account_id": accountID,
    "additional": "context",
}).Info("User performed action")

log.LogSecurityEvent("event_type", logger.Fields{
    "user_id": userID,
    "details": "specific_details",
}).Warn("Security event occurred")
```

### 🚨 Error Handling

- **Return errors** instead of panicking
- **Wrap errors** with context using `fmt.Errorf`
- **Log errors** with appropriate context
- **Handle errors gracefully** in user-facing operations

```go
// Good error handling
result, err := someOperation()
if err != nil {
    log.WithError(err).Error("Operation failed")
    return fmt.Errorf("failed to perform operation: %w", err)
}
```

### 📋 Pull Request Process

#### **Before Submitting**
1. **Test your changes**:
   ```bash
   go test ./...
   go vet ./...
   go fmt ./...
   ```
2. **Update documentation** if needed
3. **Add/update tests** for new functionality
4. **Ensure no linting errors**

#### **PR Requirements**
- **Clear title** describing the change
- **Detailed description** explaining what and why
- **Link related issues** if applicable
- **Small, focused changes** (prefer multiple small PRs)
- **Updated tests** for new functionality

#### **PR Template**
```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Tests pass locally
- [ ] New tests added for new functionality
- [ ] Manual testing completed

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No sensitive information committed
```

### 🏷️ Commit Guidelines

#### **Commit Message Format**
```
type(scope): short description

Longer description if needed

Fixes #123
```

#### **Types**
- `feat`: New features
- `fix`: Bug fixes
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

#### **Examples**
```bash
feat(slack): add support for custom approval messages
fix(aws): handle empty SQS ARN configuration
docs(readme): update installation instructions
test(config): add validation tests for required fields
```

### 🐛 Reporting Issues

When reporting bugs, please include:
- **Go version** (`go version`)
- **Operating system** and version
- **Steps to reproduce** the issue
- **Expected vs actual behavior**
- **Error messages** and logs
- **Configuration** (sanitized, no secrets)

### 💡 Feature Requests

For new features, please provide:
- **Use case** description
- **Proposed solution** or approach
- **Alternative solutions** considered
- **Additional context** or examples

### 📞 Getting Help

- **GitHub Issues** for bugs and feature requests
- **GitHub Discussions** for questions and general discussion
- **Code Review** feedback is always welcome

### 🙏 Recognition

Contributors will be recognized in:
- **GitHub contributors** page
- **Release notes** for significant contributions
- **Documentation** credits

Thank you for contributing to AWS PIM! 🚀