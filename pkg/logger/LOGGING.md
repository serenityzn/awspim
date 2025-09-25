# Logging Configuration

This application uses structured logging with [logrus](https://github.com/sirupsen/logrus) for better observability and debugging.

## Environment Variables

Configure logging behavior using these environment variables:

### `ENVIRONMENT`
- **Values**: `production`, `prod`, `development`, `dev`
- **Default**: `development`
- **Description**: 
  - `production/prod`: JSON formatted logs for log aggregation systems
  - `development/dev`: Human-readable text formatted logs

### `LOG_LEVEL`
- **Values**: `debug`, `info`, `warn`, `error`
- **Default**: `info`
- **Description**: Sets the minimum log level to output

## Log Structure

All logs include structured fields for better filtering and analysis:

### Components
- `aws` - AWS operations (Secrets Manager, SQS)
- `slack` - Slack operations (messages, interactions)
- `user_action` - User-initiated actions
- `security` - Security-related events

### Common Fields
- `component` - The component generating the log
- `operation` - The specific operation being performed
- `user_id` - Slack user ID (when applicable)
- `account_id` - AWS account ID (when applicable)
- `channel_id` - Slack channel ID (when applicable)
- `error` - Error details (when applicable)

## Examples

### Development Mode (TEXT)
```
time="2024-01-15 14:30:25" level=info msg="User requested access to valid AWS account" component=user_action action=valid_account_request account_id=123456789012 channel_id=C1234567890 user_id=U1234567890
```

### Production Mode (JSON)
```json
{
  "level": "info",
  "msg": "User requested access to valid AWS account",
  "component": "user_action",
  "action": "valid_account_request",
  "account_id": "123456789012",
  "channel_id": "C1234567890",
  "user_id": "U1234567890",
  "time": "2024-01-15T14:30:25.123Z"
}
```

## Security Events

The application logs important security events:

- **Unauthorized channel usage** - Users trying to use commands in wrong channels
- **Invalid account requests** - Requests for non-existent AWS accounts
- **Self-approval attempts** - Users trying to approve their own requests
- **All approval/denial actions** - Complete audit trail of access decisions

## Log Aggregation

For production deployments, consider:

1. **ELK Stack** (Elasticsearch, Logstash, Kibana)
2. **AWS CloudWatch Logs**
3. **Datadog**
4. **Splunk**

The JSON format in production mode is optimized for these systems.
