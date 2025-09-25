package aws

import (
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-secretsmanager-caching-go/secretcache"
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/errors"
	"github.com/serenityzn/awspim/pkg/logger"
)

// SessionManager manages AWS sessions and service clients with connection reuse
type SessionManager struct {
	session     *session.Session
	sqsClient   *sqs.SQS
	secretCache *secretcache.Cache
	region      string
	mutex       sync.RWMutex
	lastUpdated time.Time
}

var (
	globalSessionManager *SessionManager
	sessionMutex         sync.RWMutex
)

// GetSessionManager returns the global session manager instance
func GetSessionManager() (*SessionManager, error) {
	sessionMutex.RLock()
	if globalSessionManager != nil {
		sessionMutex.RUnlock()
		return globalSessionManager, nil
	}
	sessionMutex.RUnlock()

	// Need to create session manager
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	// Double-check in case another goroutine created it
	if globalSessionManager != nil {
		return globalSessionManager, nil
	}

	cfg := config.Get()
	region := cfg.GetAWSRegion()

	log := logger.GetDefaultLogger()
	log.LogAWSOperation("create_session_manager", logger.Fields{
		"region": region,
	}).Info("Creating AWS session manager")

	// Create AWS session with optimized configuration
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
		// Enable connection reuse and pooling
		MaxRetries: aws.Int(3),
		// Note: HTTPClient configuration can be set separately if needed
	})
	if err != nil {
		return nil, errors.NewAWSError("failed to create AWS session", err).WithContext("region", region)
	}

	// Create secret cache
	secretCache, err := secretcache.New(func(cache *secretcache.Cache) {
		cache.Client = secretsmanager.New(sess)
		// Set cache TTL to 5 minutes for better performance
		cache.CacheItemTTL = int64(5 * time.Minute)
	})
	if err != nil {
		return nil, errors.NewAWSError("failed to create secret cache", err)
	}

	globalSessionManager = &SessionManager{
		session:     sess,
		sqsClient:   sqs.New(sess),
		secretCache: secretCache,
		region:      region,
		lastUpdated: time.Now(),
	}

	log.LogAWSOperation("session_manager_ready", logger.Fields{
		"region": region,
	}).Info("AWS session manager created successfully")

	return globalSessionManager, nil
}

// GetSQSClient returns the cached SQS client
func (sm *SessionManager) GetSQSClient() *sqs.SQS {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.sqsClient
}

// GetSecretCache returns the cached secret manager client
func (sm *SessionManager) GetSecretCache() *secretcache.Cache {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.secretCache
}

// GetSession returns the AWS session
func (sm *SessionManager) GetSession() *session.Session {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.session
}

// GetRegion returns the configured AWS region
func (sm *SessionManager) GetRegion() string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	return sm.region
}

// RefreshIfNeeded refreshes the session if it's been too long since last update
func (sm *SessionManager) RefreshIfNeeded() error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Refresh session every hour
	if time.Since(sm.lastUpdated) > time.Hour {
		log := logger.GetDefaultLogger()
		log.LogAWSOperation("refresh_session", logger.Fields{
			"last_updated": sm.lastUpdated,
			"region":       sm.region,
		}).Info("Refreshing AWS session")

		cfg := config.Get()
		newRegion := cfg.GetAWSRegion()

		// Only recreate if region changed
		if newRegion != sm.region {
			sess, err := session.NewSession(&aws.Config{
				Region:     aws.String(newRegion),
				MaxRetries: aws.Int(3),
			})
			if err != nil {
				return errors.NewAWSError("failed to refresh AWS session", err).WithContext("region", newRegion)
			}

			sm.session = sess
			sm.sqsClient = sqs.New(sess)
			sm.region = newRegion

			// Recreate secret cache with new session
			secretCache, err := secretcache.New(func(cache *secretcache.Cache) {
				cache.Client = secretsmanager.New(sess)
				cache.CacheItemTTL = int64(5 * time.Minute)
			})
			if err != nil {
				return errors.NewAWSError("failed to recreate secret cache", err)
			}
			sm.secretCache = secretCache
		}

		sm.lastUpdated = time.Now()
	}

	return nil
}

// Close cleanly shuts down the session manager
func (sm *SessionManager) Close() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	log := logger.GetDefaultLogger()
	log.LogAWSOperation("close_session_manager", logger.Fields{
		"region": sm.region,
	}).Info("Closing AWS session manager")

	// The AWS SDK handles connection cleanup automatically
	// We just need to clear our references
	sm.session = nil
	sm.sqsClient = nil
	sm.secretCache = nil
}
