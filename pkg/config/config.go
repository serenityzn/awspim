package config

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/serenityzn/awspim/pkg/errors"
)

// Config holds all configuration values for the application
type Config struct {
	// Slack Configuration
	SlackBotToken string `mapstructure:"slack_bot_token"`
	SlackAppToken string `mapstructure:"slack_app_token"`
	
	// AWS Configuration
	AWSRegion         string `mapstructure:"aws_region"`
	AWSAccountsSecret string `mapstructure:"aws_accounts_secret"`
	ManagerSQSARN     string `mapstructure:"manager_sqs_arn"`
	
	// Application Configuration
	Environment string `mapstructure:"environment"`
	LogLevel    string `mapstructure:"log_level"`
	
	// Security Configuration
	AdminUsers     []string `mapstructure:"admin_users"`
	AllowedChannel string   `mapstructure:"allowed_channel"`
	
	// Multi-Factor Authentication Configuration
	SESFromEmail           string   `mapstructure:"ses_from_email"`
	AllowedEmailDomains    []string `mapstructure:"allowed_email_domains"`
	RequireMultiFactorAuth bool     `mapstructure:"require_multi_factor_auth"`
	TOTPIssuer             string   `mapstructure:"totp_issuer"`
	MFAStorageSecret       string   `mapstructure:"mfa_storage_secret"` // Secrets Manager secret name for storing user 2FA registrations

	// Response queue — assigner sends results back here
	ResponseSQSURL string `mapstructure:"response_sqs_url"`

	// Testing
	TestMode bool `mapstructure:"test_mode"`
}

var (
    globalConfig *Config
    configMutex  sync.RWMutex // Mutex for protecting globalConfig
)

// Load loads configuration from config file first, then environment variables
func Load() (*Config, error) {
	v := viper.New()
	
	// Set config file search paths and name
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/awspim")
	
	// Set environment variable settings
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	
	// Manually bind environment variables to support both prefixed and non-prefixed
	v.BindEnv("slack_bot_token", "AWSPIM_SLACK_BOT_TOKEN", "SLACK_BOT_TOKEN")
	v.BindEnv("slack_app_token", "AWSPIM_SLACK_APP_TOKEN", "SLACK_APP_TOKEN")
	v.BindEnv("aws_region", "AWSPIM_AWS_REGION", "AWS_REGION")
	v.BindEnv("aws_accounts_secret", "AWSPIM_AWS_ACCOUNTS_SECRET", "AWS_ACCOUNTS_SECRET")
	v.BindEnv("manager_sqs_arn", "AWSPIM_MANAGER_SQS_ARN", "MANAGER_SQS_ARN")
	v.BindEnv("environment", "AWSPIM_ENVIRONMENT", "ENVIRONMENT")
	v.BindEnv("log_level", "AWSPIM_LOG_LEVEL", "LOG_LEVEL")
	v.BindEnv("allowed_channel", "AWSPIM_ALLOWED_CHANNEL")
	v.BindEnv("admin_users", "AWSPIM_ADMIN_USERS")
	v.BindEnv("require_multi_factor_auth", "AWSPIM_REQUIRE_MULTI_FACTOR_AUTH")
	v.BindEnv("ses_from_email", "AWSPIM_SES_FROM_EMAIL", "SES_FROM_EMAIL")
	v.BindEnv("totp_issuer", "AWSPIM_TOTP_ISSUER")
	v.BindEnv("allowed_email_domains", "AWSPIM_ALLOWED_EMAIL_DOMAINS")
	v.BindEnv("mfa_storage_secret", "AWSPIM_MFA_STORAGE_SECRET", "MFA_STORAGE_SECRET")
	v.BindEnv("response_sqs_url", "AWSPIM_RESPONSE_SQS_URL", "RESPONSE_SQS_URL")
	v.BindEnv("test_mode", "AWSPIM_TEST_MODE")
	
	// Set defaults
	setDefaults(v)
	
	// Try to read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file was found but another error was produced
			return nil, errors.NewConfigurationError("failed to read config file", err)
		}
		// Config file not found; proceed with environment variables and defaults
	}
	
	// Unmarshal config
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, errors.NewConfigurationError("failed to unmarshal config", err)
	}
	
	// Validate required fields
	if err := validateConfig(&config); err != nil {
		return nil, err // validateConfig already returns AppError
	}
	
	// Store globally
    configMutex.Lock()
	globalConfig = &config
    configMutex.Unlock()
	
	return &config, nil
}

// Get returns the globally loaded configuration
func Get() *Config {
    configMutex.RLock()
    defer configMutex.RUnlock()
	if globalConfig == nil {
		// This should ideally not happen if Load() is called at startup
		// but provides a fallback for safety.
		panic("configuration not loaded. Call config.Load() first.")
	}
	return globalConfig
}

// setDefaults sets default values for configuration
func setDefaults(v *viper.Viper) {
	// AWS defaults
	v.SetDefault("aws_region", "us-east-2")
	v.SetDefault("aws_accounts_secret", "pim/aws-accounts")
	
	// Application defaults
	v.SetDefault("environment", "development")
	v.SetDefault("log_level", "info")
	
	// Security defaults
	v.SetDefault("allowed_channel", "pim-management")
	v.SetDefault("admin_users", []string{})
	
	// Multi-factor authentication defaults
	v.SetDefault("require_multi_factor_auth", false)
	v.SetDefault("allowed_email_domains", []string{"company.com"})
	v.SetDefault("totp_issuer", "AWS PIM")
	v.SetDefault("mfa_storage_secret", "pim/mfa-registrations")
	v.SetDefault("test_mode", false)
}

// validateConfig validates that all required configuration is present
func validateConfig(config *Config) error {
	var missing []string
	
	// Check required Slack configuration
	if config.SlackBotToken == "" {
		missing = append(missing, "slack_bot_token (AWSPIM_SLACK_BOT_TOKEN or SLACK_BOT_TOKEN)")
	}
	if config.SlackAppToken == "" {
		missing = append(missing, "slack_app_token (AWSPIM_SLACK_APP_TOKEN or SLACK_APP_TOKEN)")
	}
	
	// Check required AWS configuration
	if config.ManagerSQSARN == "" {
		missing = append(missing, "manager_sqs_arn (AWSPIM_MANAGER_SQS_ARN or MANAGER_SQS_ARN)")
	}
	
	// Check required Multi-factor authentication configuration (only if MFA is enabled)
	if config.RequireMultiFactorAuth && config.SESFromEmail == "" {
		missing = append(missing, "ses_from_email (AWSPIM_SES_FROM_EMAIL or SES_FROM_EMAIL)")
	}
	
	if len(missing) > 0 {
		return errors.NewValidationError(
			fmt.Sprintf("missing required configuration: %s", strings.Join(missing, ", ")),
			nil,
		).WithContext("missing_fields", missing)
	}
	
	return nil
}

// GetSlackBotToken returns the Slack bot token
func (c *Config) GetSlackBotToken() string {
	return c.SlackBotToken
}

// GetSlackAppToken returns the Slack app token
func (c *Config) GetSlackAppToken() string {
	return c.SlackAppToken
}

// GetAWSRegion returns the AWS region
func (c *Config) GetAWSRegion() string {
	return c.AWSRegion
}

// GetAWSAccountsSecret returns the AWS accounts secret name
func (c *Config) GetAWSAccountsSecret() string {
	return c.AWSAccountsSecret
}

// GetManagerSQSARN returns the SQS ARN for approval notifications
func (c *Config) GetManagerSQSARN() string {
	return c.ManagerSQSARN
}

// GetEnvironment returns the environment setting
func (c *Config) GetEnvironment() string {
	return c.Environment
}

// GetLogLevel returns the log level
func (c *Config) GetLogLevel() string {
	return c.LogLevel
}

// GetAdminUsers returns the list of admin users who can self-approve
func (c *Config) GetAdminUsers() []string {
	return c.AdminUsers
}

// GetAllowedChannel returns the channel where PIM commands are allowed
func (c *Config) GetAllowedChannel() string {
	return c.AllowedChannel
}

// IsAdminUser checks if a user is an admin user
func (c *Config) IsAdminUser(username string) bool {
	for _, admin := range c.AdminUsers {
		if admin == username {
			return true
		}
	}
	return false
}

// GetSESFromEmail returns the SES from email address
func (c *Config) GetSESFromEmail() string {
	return c.SESFromEmail
}

// GetAllowedEmailDomains returns the list of allowed email domains
func (c *Config) GetAllowedEmailDomains() []string {
	return c.AllowedEmailDomains
}

// IsRequireMultiFactorAuth returns whether multi-factor authentication is required
func (c *Config) IsRequireMultiFactorAuth() bool {
	return c.RequireMultiFactorAuth
}

// GetTOTPIssuer returns the TOTP issuer name
func (c *Config) GetTOTPIssuer() string {
	return c.TOTPIssuer
}

// GetMFAStorageSecret returns the Secrets Manager secret name for MFA storage
func (c *Config) GetMFAStorageSecret() string {
	return c.MFAStorageSecret
}

// GetResponseSQSURL returns the SQS URL for receiving results from the assigner
func (c *Config) GetResponseSQSURL() string {
	return c.ResponseSQSURL
}

// IsTestMode returns whether test mode is enabled
func (c *Config) IsTestMode() bool {
	return c.TestMode
}

// IsAllowedEmailDomain checks if an email domain is allowed
func (c *Config) IsAllowedEmailDomain(email string) bool {
	if len(c.AllowedEmailDomains) == 0 {
		return false
	}
	
	// Extract domain from email
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]
	
	for _, allowedDomain := range c.AllowedEmailDomains {
		if domain == allowedDomain {
			return true
		}
	}
	return false
}
