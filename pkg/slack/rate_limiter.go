package slack

import (
	"sync"
	"time"

	"github.com/serenityzn/awspim/pkg/logger"
)

// UserRateLimit tracks rate limiting for individual users
type UserRateLimit struct {
	lastRequestTime time.Time
	requestCount    int
	windowStart     time.Time
}

// RateLimiter manages rate limiting for Slack commands
type RateLimiter struct {
	users          map[string]*UserRateLimit
	mutex          sync.RWMutex
	maxRequests    int           // Max requests per window
	windowDuration time.Duration // Time window for rate limiting
	cooldownPeriod time.Duration // Minimum time between requests
}

var (
	globalRateLimiter *RateLimiter
	rateLimiterMutex  sync.RWMutex
)

// GetRateLimiter returns the global rate limiter instance
func GetRateLimiter() *RateLimiter {
	rateLimiterMutex.RLock()
	if globalRateLimiter != nil {
		rateLimiterMutex.RUnlock()
		return globalRateLimiter
	}
	rateLimiterMutex.RUnlock()

	rateLimiterMutex.Lock()
	defer rateLimiterMutex.Unlock()

	// Double-check in case another goroutine created it
	if globalRateLimiter == nil {
		globalRateLimiter = &RateLimiter{
			users:          make(map[string]*UserRateLimit),
			maxRequests:    10,                // 10 requests per window
			windowDuration: 5 * time.Minute,   // 5-minute window
			cooldownPeriod: 2 * time.Second,   // 2-second cooldown between requests
		}
	}

	return globalRateLimiter
}

// IsAllowed checks if a user is allowed to make a request based on rate limiting
func (rl *RateLimiter) IsAllowed(userID string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	userLimit, exists := rl.users[userID]

	if !exists {
		// First request from this user
		rl.users[userID] = &UserRateLimit{
			lastRequestTime: now,
			requestCount:    1,
			windowStart:     now,
		}
		return true
	}

	// Check cooldown period
	if now.Sub(userLimit.lastRequestTime) < rl.cooldownPeriod {
		log := logger.GetDefaultLogger()
		log.LogSecurityEvent("rate_limit_cooldown", logger.Fields{
			"user_id":             userID,
			"time_since_last":     now.Sub(userLimit.lastRequestTime).Seconds(),
			"cooldown_seconds":    rl.cooldownPeriod.Seconds(),
		}).Warn("User request blocked due to cooldown period")
		return false
	}

	// Check if we need to reset the window
	if now.Sub(userLimit.windowStart) >= rl.windowDuration {
		userLimit.requestCount = 1
		userLimit.windowStart = now
		userLimit.lastRequestTime = now
		return true
	}

	// Check if user has exceeded max requests in the current window
	if userLimit.requestCount >= rl.maxRequests {
		log := logger.GetDefaultLogger()
		log.LogSecurityEvent("rate_limit_exceeded", logger.Fields{
			"user_id":          userID,
			"request_count":    userLimit.requestCount,
			"max_requests":     rl.maxRequests,
			"window_duration":  rl.windowDuration.Minutes(),
		}).Warn("User exceeded rate limit")
		return false
	}

	// Allow the request
	userLimit.requestCount++
	userLimit.lastRequestTime = now

	return true
}

// GetUserStats returns rate limiting statistics for a user
func (rl *RateLimiter) GetUserStats(userID string) map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	userLimit, exists := rl.users[userID]
	if !exists {
		return map[string]interface{}{
			"exists":        false,
			"request_count": 0,
		}
	}

	now := time.Now()
	return map[string]interface{}{
		"exists":              true,
		"request_count":       userLimit.requestCount,
		"max_requests":        rl.maxRequests,
		"window_start":        userLimit.windowStart,
		"window_duration":     rl.windowDuration.Minutes(),
		"last_request_time":   userLimit.lastRequestTime,
		"time_since_last":     now.Sub(userLimit.lastRequestTime).Seconds(),
		"cooldown_remaining":  (rl.cooldownPeriod - now.Sub(userLimit.lastRequestTime)).Seconds(),
		"window_time_left":    (rl.windowDuration - now.Sub(userLimit.windowStart)).Minutes(),
	}
}

// CleanupOldEntries removes old user entries to prevent memory leaks
func (rl *RateLimiter) CleanupOldEntries() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	cleanupThreshold := 2 * rl.windowDuration // Keep entries for 2x window duration

	for userID, userLimit := range rl.users {
		if now.Sub(userLimit.lastRequestTime) > cleanupThreshold {
			delete(rl.users, userID)
		}
	}

	log := logger.GetDefaultLogger()
	log.LogSlackOperation("rate_limiter_cleanup", logger.Fields{
		"remaining_users": len(rl.users),
		"cleanup_threshold_minutes": cleanupThreshold.Minutes(),
	}).Debug("Cleaned up old rate limiter entries")
}

// StartPeriodicCleanup starts a goroutine that periodically cleans up old entries
func (rl *RateLimiter) StartPeriodicCleanup() {
	go func() {
		ticker := time.NewTicker(rl.windowDuration) // Cleanup every window duration
		defer ticker.Stop()

		for range ticker.C {
			rl.CleanupOldEntries()
		}
	}()
}
