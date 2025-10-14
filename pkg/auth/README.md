# Multi-Factor Authentication Storage

This package implements persistent storage for user 2FA registrations using a pluggable storage interface.

## Architecture

### Storage Interface (`storage.go`)

The `RegistrationStorage` interface defines the contract for persisting user 2FA data:

```go
type RegistrationStorage interface {
    SaveRegistration(userID string, registration *UserRegistration) error
    GetRegistration(userID string) (*UserRegistration, error)
    DeleteRegistration(userID string) error
    ListRegistrations() (map[string]*UserRegistration, error)
    UpdateLastUsed(userID string, lastUsedAt time.Time) error
    RemoveBackupCode(userID string, codeIndex int) error
}
```

This interface makes it easy to swap storage backends without changing the authentication logic.

## Current Implementation: AWS Secrets Manager

### Overview

The default implementation (`storage_secretsmanager.go`) stores all user registrations in a **single AWS Secrets Manager secret** as a JSON document.

### Why Secrets Manager?

- ✅ **Purpose-built** for storing sensitive credentials like TOTP secrets
- ✅ **Encrypted by default** with AWS KMS
- ✅ Built-in versioning and rotation capabilities
- ✅ IAM-based access control
- ✅ Audit logging via CloudTrail
- ✅ Cost-effective for moderate user counts

### Storage Format

```json
{
  "version": "1.0",
  "last_updated": "2025-10-09T10:30:00Z",
  "registrations": {
    "U12345": {
      "user_id": "U12345",
      "email": "user@company.com",
      "totp_secret": "BASE32ENCODEDSECRET",
      "backup_codes": ["CODE1", "CODE2", "..."],
      "registered_at": "2025-10-01T12:00:00Z",
      "last_used_at": "2025-10-09T10:30:00Z",
      "is_active": true
    }
  }
}
```

### Configuration

Set the secret name via configuration:

```yaml
# config.yaml
mfa_storage_secret: "pim/mfa-registrations"
```

Or via environment variable:

```bash
export AWSPIM_MFA_STORAGE_SECRET="pim/mfa-registrations"
# or
export MFA_STORAGE_SECRET="pim/mfa-registrations"
```

### How It Works

1. **Initialization**: On bot startup, the storage loads all registrations from Secrets Manager into memory cache
2. **Read Operations**: All reads are served from the in-memory cache (fast)
3. **Write Operations**: Updates are written to both cache AND Secrets Manager (write-through)
4. **Consistency**: Uses mutex locking to ensure thread-safe access

### Caching Strategy

- **Load once on startup**: All user registrations are loaded into memory
- **Write-through cache**: Every update is immediately persisted to Secrets Manager
- **No TTL**: Data stays in cache for the lifetime of the application
- **Restart recovery**: On Docker restart, all data is automatically reloaded

### IAM Permissions Required

Your EC2 instance (or ECS task) needs these permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "secretsmanager:GetSecretValue",
        "secretsmanager:UpdateSecret",
        "secretsmanager:CreateSecret",
        "secretsmanager:DescribeSecret"
      ],
      "Resource": "arn:aws:secretsmanager:REGION:ACCOUNT:secret:pim/mfa-registrations*"
    }
  ]
}
```

## Alternative Storage Backends

### Future Implementations

The interface design makes it easy to add new backends:

#### DynamoDB Storage (Future)
```go
type DynamoDBStorage struct {
    client    *dynamodb.DynamoDB
    tableName string
}

func NewDynamoDBStorage(client *dynamodb.DynamoDB, tableName string) *DynamoDBStorage {
    // Implementation
}
```

#### File Storage (For Development)
```go
type FileStorage struct {
    filePath string
    mutex    sync.RWMutex
}

func NewFileStorage(filePath string) *FileStorage {
    // Implementation
}
```

### Switching Storage Backends

To switch backends, only modify the initialization in `pkg/slack/slack.go`:

```go
// Instead of:
storage, err := auth.NewSecretsManagerStorage(secretsManagerClient, cfg.GetMFAStorageSecret())

// Use DynamoDB:
storage, err := auth.NewDynamoDBStorage(dynamoClient, cfg.GetMFATableName())

// Or File (dev only):
storage, err := auth.NewFileStorage("/var/lib/awspim/mfa.json")
```

## Data Lifecycle

### User Registration Flow
1. User runs `/register-totp`
2. System generates TOTP secret and backup codes
3. Registration stored with `is_active: false`
4. User completes verification
5. Registration updated to `is_active: true`
6. **All persisted to Secrets Manager**

### Backup Code Usage
1. User provides backup code for authentication
2. System validates and removes used code from array
3. Updated registration saved to Secrets Manager
4. Code can never be reused

### Last Used Tracking
1. On successful MFA verification
2. `last_used_at` timestamp updated
3. Persisted to Secrets Manager for audit purposes

## Security Considerations

### Secrets Manager Benefits
- 🔐 **Encryption at rest**: Using AWS KMS
- 🔐 **Encryption in transit**: TLS/HTTPS
- 🔒 **Access control**: IAM policies
- 📝 **Audit logging**: CloudTrail tracks all access
- 🔄 **Versioning**: Previous versions retained

### In-Memory Cache Security
- ✅ Cache is private to the authenticator instance
- ✅ No external access to cached data
- ✅ Cleared on application shutdown
- ✅ Reloaded on restart (no stale data)

### TOTP Secret Protection
- ❌ **Never logged** or exposed in error messages
- ❌ **Never transmitted** to Slack or external systems
- ✅ **Only stored** in Secrets Manager and memory
- ✅ **Only used** for TOTP validation

## Performance

### Read Performance
- ⚡ **Near-instant**: All reads from in-memory cache
- 📊 No AWS API calls for reads
- 🎯 O(1) lookup time

### Write Performance
- 📝 **~100-200ms**: Includes Secrets Manager API call
- 🔄 Writes are infrequent (only on registration/backup code usage)
- 💰 Cost: ~$0.05 per 10,000 writes

### Scalability
- 👥 **User count**: Handles thousands of users easily
- 📦 **Storage size**: JSON fits well under Secrets Manager 65KB limit
- 💾 **Memory**: ~1-2KB per user registration

## Cost Estimation

### AWS Secrets Manager Pricing
- **Secret storage**: $0.40/month per secret
- **API calls**: $0.05 per 10,000 calls

### Example Cost for 100 Users
- **Storage**: $0.40/month (one secret for all users)
- **API calls**: ~$0.01/month (20 registrations + 50 logins)
- **Total**: ~$0.41/month

### Cost Comparison
| Storage Backend | 100 Users/Month | 1000 Users/Month |
|----------------|-----------------|------------------|
| Secrets Manager | $0.41 | $0.50 |
| DynamoDB On-Demand | $0.25 | $2.50 |
| DynamoDB Provisioned | $1.25 | $1.25 |

## Monitoring & Debugging

### Logging

All storage operations are logged with structured fields:

```go
// On initialization
log.LogSecurityEvent("storage_initialized", logger.Fields{
    "backend": "secretsmanager",
    "secret_name": "pim/mfa-registrations",
    "user_count": 5,
})

// On save
log.LogSecurityEvent("storage_persisted", logger.Fields{
    "secret_name": "pim/mfa-registrations",
    "user_count": 5,
})
```

### CloudWatch Metrics

Monitor via AWS CloudWatch:
- `GetSecretValue` API call count
- `UpdateSecret` API call count
- API error rates

### Troubleshooting

**Problem**: Bot fails to start with "failed to initialize storage"

**Solutions**:
1. Check IAM permissions
2. Verify secret exists or can be created
3. Check AWS region configuration
4. Review CloudWatch logs for API errors

**Problem**: Registrations lost after restart

**Solution**: This shouldn't happen! Check:
1. Secrets Manager secret still exists
2. IAM permissions still valid
3. No errors in logs during startup

## Migration Guide

### Migrating from In-Memory to Persistent Storage

If you were using an older version with in-memory storage:

1. **Before Update**: All registrations will be lost on upgrade
2. **After Update**: Users need to re-register with `/register-totp`
3. **Optional**: Export old data before upgrade (requires code changes)

### Migrating Between Storage Backends

To migrate from Secrets Manager to another backend:

```go
// 1. Read from old storage
oldStorage := auth.NewSecretsManagerStorage(...)
registrations, _ := oldStorage.ListRegistrations()

// 2. Write to new storage
newStorage := auth.NewDynamoDBStorage(...)
for userID, reg := range registrations {
    newStorage.SaveRegistration(userID, reg)
}
```

## Testing

### Unit Testing Storage

Mock the interface for testing:

```go
type MockStorage struct {
    registrations map[string]*auth.UserRegistration
}

func (m *MockStorage) SaveRegistration(userID string, reg *auth.UserRegistration) error {
    m.registrations[userID] = reg
    return nil
}

// ... implement other methods
```

### Integration Testing

Test with real AWS services (requires AWS credentials):

```bash
export AWS_REGION=us-east-2
export MFA_STORAGE_SECRET=pim/mfa-registrations-test
go test ./pkg/auth/...
```

## Best Practices

### Secret Naming
- ✅ Use hierarchical names: `pim/mfa-registrations`
- ✅ Use environment-specific names: `pim/mfa-registrations-prod`
- ❌ Avoid generic names: `registrations`

### IAM Policy
- ✅ Use least privilege principle
- ✅ Restrict to specific secret ARN
- ✅ Use separate IAM roles per environment
- ❌ Don't use `*` wildcards in production

### Backup & Recovery
- ✅ Enable Secrets Manager versioning (enabled by default)
- ✅ Set up CloudWatch alarms for API errors
- ✅ Document recovery procedures
- ✅ Test recovery process regularly

### Monitoring
- ✅ Monitor API call rates
- ✅ Set up alerts for high error rates
- ✅ Track `last_used_at` for inactive users
- ✅ Review CloudTrail logs regularly

