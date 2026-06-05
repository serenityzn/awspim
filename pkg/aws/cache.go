package aws

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/errors"
	"github.com/serenityzn/awspim/pkg/logger"
)

// AccountsCache manages cached AWS accounts data
type AccountsCache struct {
	accounts    *AwsAccounts
	lastUpdated time.Time
	ttl         time.Duration
	mutex       sync.RWMutex
}

var (
	globalAccountsCache *AccountsCache
	cacheMutex          sync.RWMutex
)

// GetAccountsCache returns the global accounts cache instance
func GetAccountsCache() *AccountsCache {
	cacheMutex.RLock()
	if globalAccountsCache != nil {
		cacheMutex.RUnlock()
		return globalAccountsCache
	}
	cacheMutex.RUnlock()

	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Double-check in case another goroutine created it
	if globalAccountsCache == nil {
		globalAccountsCache = &AccountsCache{
			ttl: 10 * time.Minute, // Cache for 10 minutes
		}
	}

	return globalAccountsCache
}

// GetAccounts returns cached accounts or fetches fresh data if cache is stale
func (ac *AccountsCache) GetAccounts() (*AwsAccounts, error) {
	ac.mutex.RLock()
	
	// Check if cache is still valid
	if ac.accounts != nil && time.Since(ac.lastUpdated) < ac.ttl {
		accounts := ac.accounts
		ac.mutex.RUnlock()
		
		log := logger.GetDefaultLogger()
		log.LogAWSOperation("get_accounts_cached", logger.Fields{
			"cache_age_seconds": time.Since(ac.lastUpdated).Seconds(),
			"accounts_count":    len(accounts.Accounts),
		}).Debug("Returning cached AWS accounts")
		
		return accounts, nil
	}
	ac.mutex.RUnlock()

	// Cache is stale or empty, fetch fresh data
	return ac.RefreshAccounts()
}

// RefreshAccounts forcefully refreshes the accounts cache
func (ac *AccountsCache) RefreshAccounts() (*AwsAccounts, error) {
	log := logger.GetDefaultLogger()
	log.LogAWSOperation("refresh_accounts_cache", logger.Fields{
		"action": "fetching_fresh_data",
	}).Info("Refreshing AWS accounts cache")

	accounts, err := ac.fetchAccountsFromSecretsManager()
	if err != nil {
		return nil, err
	}

	ac.mutex.Lock()
	ac.accounts = accounts
	ac.lastUpdated = time.Now()
	ac.mutex.Unlock()

	log.LogAWSOperation("accounts_cache_refreshed", logger.Fields{
		"accounts_count": len(accounts.Accounts),
		"cache_ttl_minutes": ac.ttl.Minutes(),
	}).Info("AWS accounts cache refreshed successfully")

	return accounts, nil
}

// fetchAccountsFromSecretsManager retrieves accounts from AWS Secrets Manager
func (ac *AccountsCache) fetchAccountsFromSecretsManager() (*AwsAccounts, error) {
	log := logger.GetDefaultLogger()
	cfg := config.Get()

	log.LogAWSOperation("fetch_accounts_from_sm", logger.Fields{
		"action": "retrieving_from_secrets_manager",
	}).Debug("Fetching AWS accounts from Secrets Manager")

	awsAccountsSecret := cfg.GetAWSAccountsSecret()
	if awsAccountsSecret == "" {
		log.LogAWSOperation("fetch_accounts_from_sm", logger.Fields{
			"error": "missing_secret_name",
		}).Error("AWS_ACCOUNTS_SECRET not configured")
		return nil, errors.NewConfigurationError("AWS_ACCOUNTS_SECRET not configured", nil)
	}

	// Get session manager for reused connections
	sessionManager, err := GetSessionManager()
	if err != nil {
		return nil, err
	}

	// Refresh session if needed
	if err := sessionManager.RefreshIfNeeded(); err != nil {
		log.LogAWSOperation("fetch_accounts_from_sm", logger.Fields{
			"error": "session_refresh_failed",
		}).WithError(err).Warn("Failed to refresh AWS session, continuing with existing session")
	}

	secretCache := sessionManager.GetSecretCache()

	secretValue, err := secretCache.GetSecretString(awsAccountsSecret)
	if err != nil {
		log.LogAWSOperation("fetch_accounts_from_sm", logger.Fields{
			"secret_name": awsAccountsSecret,
		}).WithError(err).Error("Failed to retrieve secret")
		return nil, errors.NewAWSError("failed to retrieve secret", err).WithContext("secret_name", awsAccountsSecret)
	}

	var awsAccounts AwsAccounts
	if err := json.Unmarshal([]byte(secretValue), &awsAccounts); err != nil {
		log.LogAWSOperation("fetch_accounts_from_sm", logger.Fields{
			"secret_name": awsAccountsSecret,
		}).WithError(err).Error("Failed to parse secret JSON")
		return nil, errors.NewAWSError("failed to parse secret JSON", err).WithContext("secret_name", awsAccountsSecret)
	}

	log.LogAWSOperation("fetch_accounts_from_sm", logger.Fields{
		"secret_name":    awsAccountsSecret,
		"accounts_count": len(awsAccounts.Accounts),
	}).Info("AWS accounts retrieved successfully from Secrets Manager")

	return &awsAccounts, nil
}


// InvalidateCache clears the current cache, forcing a fresh fetch on next request
func (ac *AccountsCache) InvalidateCache() {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()

	log := logger.GetDefaultLogger()
	log.LogAWSOperation("invalidate_accounts_cache", logger.Fields{
		"cache_age_seconds": time.Since(ac.lastUpdated).Seconds(),
	}).Info("Invalidating AWS accounts cache")

	ac.accounts = nil
	ac.lastUpdated = time.Time{}
}

// GetCacheStats returns statistics about the cache
func (ac *AccountsCache) GetCacheStats() map[string]interface{} {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()

	stats := map[string]interface{}{
		"ttl_minutes":       ac.ttl.Minutes(),
		"last_updated":      ac.lastUpdated,
		"cache_age_seconds": time.Since(ac.lastUpdated).Seconds(),
		"is_valid":          ac.accounts != nil && time.Since(ac.lastUpdated) < ac.ttl,
	}

	if ac.accounts != nil {
		stats["accounts_count"] = len(ac.accounts.Accounts)
	} else {
		stats["accounts_count"] = 0
	}

	return stats
}
