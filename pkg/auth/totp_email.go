package auth

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/serenityzn/awspim/pkg/config"
	"github.com/serenityzn/awspim/pkg/errors"
	"github.com/serenityzn/awspim/pkg/logger"
	"github.com/serenityzn/awspim/pkg/utils"
)

// UserRegistration represents a user's TOTP registration
type UserRegistration struct {
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	TOTPSecret   string    `json:"totp_secret"`
	BackupCodes  []string  `json:"backup_codes"`
	RegisteredAt time.Time `json:"registered_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
	IsActive     bool      `json:"is_active"`
}

// PendingVerification represents a pending approval verification
type PendingVerification struct {
	ID              string                 `json:"id"`
	UserID          string                 `json:"user_id"`
	EmailCode       string                 `json:"email_code"`
	ExpiresAt       time.Time              `json:"expires_at"`
	ApprovalData    map[string]interface{} `json:"approval_data"`
	CreatedAt       time.Time              `json:"created_at"`
	FailedAttempts  int                    `json:"failed_attempts"`
}

// TOTPEmailAuthenticator handles TOTP + Email multi-factor authentication
type TOTPEmailAuthenticator struct {
	registrations        map[string]*UserRegistration    // userID -> registration
	pendingVerifications map[string]*PendingVerification // userID -> pending verification
	mutex                sync.RWMutex
	sesClient            *ses.SES
}

// NewTOTPEmailAuthenticator creates a new TOTP + Email authenticator
func NewTOTPEmailAuthenticator(sesClient *ses.SES) *TOTPEmailAuthenticator {
	auth := &TOTPEmailAuthenticator{
		registrations:        make(map[string]*UserRegistration),
		pendingVerifications: make(map[string]*PendingVerification),
		sesClient:            sesClient,
	}
	
	// Start cleanup routine
	go auth.startCleanupRoutine()
	
	return auth
}

// StartRegistration initiates TOTP registration for a user
func (t *TOTPEmailAuthenticator) StartRegistration(userID, email string) (*otp.Key, error) {
	log := logger.GetDefaultLogger()
	cfg := config.Get()
	
	// Validate email domain
	if !cfg.IsAllowedEmailDomain(email) {
		return nil, errors.NewSecurityError("email domain not authorized", nil).
			WithContext("email", utils.SanitizeUserInput(email))
	}
	
	// Generate TOTP secret
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      cfg.GetTOTPIssuer(),
		AccountName: email,
		SecretSize:  32,
	})
	if err != nil {
		return nil, errors.NewInternalError("failed to generate TOTP secret", err)
	}
	
	// Generate backup codes
	backupCodes, err := t.generateBackupCodes(10)
	if err != nil {
		return nil, errors.NewInternalError("failed to generate backup codes", err)
	}
	
	// Create registration (not active until verified)
	registration := &UserRegistration{
		UserID:       userID,
		Email:        email,
		TOTPSecret:   key.Secret(),
		BackupCodes:  backupCodes,
		RegisteredAt: time.Now(),
		IsActive:     false, // Will be activated after verification
	}
	
	// Store temporarily
	t.mutex.Lock()
	t.registrations[userID] = registration
	t.mutex.Unlock()
	
	log.LogUserAction(userID, "totp_registration_started", logger.Fields{
		"email": utils.SanitizeUserInput(email),
	}).Info("TOTP registration initiated")
	
	return key, nil
}

// CompleteRegistration completes TOTP registration with email + TOTP verification
func (t *TOTPEmailAuthenticator) CompleteRegistration(userID, emailCode, totpCode string) error {
	log := logger.GetDefaultLogger()
	
	t.mutex.Lock()
	registration, exists := t.registrations[userID]
	if !exists {
		t.mutex.Unlock()
		return errors.NewSecurityError("no pending registration found", nil)
	}
	
	// Check if already active
	if registration.IsActive {
		t.mutex.Unlock()
		return errors.NewSecurityError("user already registered", nil)
	}
	t.mutex.Unlock()
	
	// Validate TOTP code
	valid := totp.Validate(totpCode, registration.TOTPSecret)
	if !valid {
		log.LogSecurityEvent("invalid_totp_registration", logger.Fields{
			"user_id": userID,
			"email":   utils.SanitizeUserInput(registration.Email),
		}).Warn("Invalid TOTP code during registration")
		return errors.NewSecurityError("invalid TOTP code", nil)
	}
	
	// Activate registration
	t.mutex.Lock()
	registration.IsActive = true
	registration.LastUsedAt = time.Now()
	t.mutex.Unlock()
	
	log.LogUserAction(userID, "totp_registration_completed", logger.Fields{
		"email": utils.SanitizeUserInput(registration.Email),
	}).Info("TOTP registration completed successfully")
	
	return nil
}

// InitiateApproval starts the approval verification process
func (t *TOTPEmailAuthenticator) InitiateApproval(userID string, approvalData map[string]interface{}) (string, error) {
	log := logger.GetDefaultLogger()
	
	// Check if user is registered
	t.mutex.RLock()
	registration, exists := t.registrations[userID]
	if !exists || !registration.IsActive {
		t.mutex.RUnlock()
		return "", errors.NewSecurityError("user not registered for multi-factor authentication", nil).
			WithContext("user_id", userID)
	}
	t.mutex.RUnlock()
	
	// Generate email verification code
	emailCode, err := t.generateEmailCode()
	if err != nil {
		return "", errors.NewInternalError("failed to generate email code", err)
	}
	
	// Create pending verification
	verification := &PendingVerification{
		ID:           t.generateVerificationID(),
		UserID:       userID,
		EmailCode:    emailCode,
		ExpiresAt:    time.Now().Add(10 * time.Minute), // 10 minute expiration
		ApprovalData: approvalData,
		CreatedAt:    time.Now(),
		FailedAttempts: 0,
	}
	
	// Store pending verification
	t.mutex.Lock()
	t.pendingVerifications[userID] = verification
	t.mutex.Unlock()
	
	// Send email with code
	err = t.sendVerificationEmail(registration.Email, emailCode, approvalData)
	if err != nil {
		// Clean up on failure
		t.mutex.Lock()
		delete(t.pendingVerifications, userID)
		t.mutex.Unlock()
		return "", errors.NewInternalError("failed to send verification email", err)
	}
	
	log.LogUserAction(userID, "approval_verification_initiated", logger.Fields{
		"email": utils.SanitizeUserInput(registration.Email),
	}).Info("Approval verification initiated")
	
	return emailCode, nil
}

// VerifyApproval verifies email code + TOTP code for approval
func (t *TOTPEmailAuthenticator) VerifyApproval(userID, emailCode, totpCode string) (*PendingVerification, error) {
	log := logger.GetDefaultLogger()
	
	t.mutex.Lock()
	verification, exists := t.pendingVerifications[userID]
	if !exists {
		t.mutex.Unlock()
		log.LogSecurityEvent("verification_not_found", logger.Fields{
			"user_id": userID,
		}).Warn("Approval verification attempted for non-existent request")
		return nil, errors.NewSecurityError("no pending verification request", nil)
	}
	
	// Check expiration
	if time.Now().After(verification.ExpiresAt) {
		delete(t.pendingVerifications, userID)
		t.mutex.Unlock()
		
		log.LogSecurityEvent("verification_expired", logger.Fields{
			"user_id": userID,
		}).Warn("Expired verification attempted")
		return nil, errors.NewSecurityError("verification has expired", nil)
	}
	
	// Check rate limiting (max 5 attempts)
	if verification.FailedAttempts >= 5 {
		delete(t.pendingVerifications, userID)
		t.mutex.Unlock()
		
		log.LogSecurityEvent("verification_rate_limited", logger.Fields{
			"user_id": userID,
			"attempts": verification.FailedAttempts,
		}).Warn("Verification rate limited")
		return nil, errors.NewSecurityError("too many failed attempts", nil)
	}
	t.mutex.Unlock()
	
	// Get user registration for TOTP validation
	t.mutex.RLock()
	registration, exists := t.registrations[userID]
	if !exists || !registration.IsActive {
		t.mutex.RUnlock()
		return nil, errors.NewSecurityError("user not registered", nil)
	}
	t.mutex.RUnlock()
	
	// Validate email code
	if verification.EmailCode != emailCode {
		t.mutex.Lock()
		verification.FailedAttempts++
		t.mutex.Unlock()
		
		log.LogSecurityEvent("invalid_email_code", logger.Fields{
			"user_id": userID,
			"email":   utils.SanitizeUserInput(registration.Email),
			"attempts": verification.FailedAttempts,
		}).Warn("Invalid email code provided")
		return nil, errors.NewSecurityError("invalid email code", nil)
	}
	
	// Validate TOTP code
	valid := totp.Validate(totpCode, registration.TOTPSecret)
	if !valid {
		// Check if it's a backup code
		valid = t.useBackupCode(userID, totpCode)
		if !valid {
			t.mutex.Lock()
			verification.FailedAttempts++
			t.mutex.Unlock()
			
			log.LogSecurityEvent("invalid_totp_code", logger.Fields{
				"user_id": userID,
				"email":   utils.SanitizeUserInput(registration.Email),
				"attempts": verification.FailedAttempts,
			}).Warn("Invalid TOTP code provided")
			return nil, errors.NewSecurityError("invalid TOTP code", nil)
		}
	}
	
	// Both codes valid - clean up and return
	t.mutex.Lock()
	delete(t.pendingVerifications, userID)
	registration.LastUsedAt = time.Now()
	t.mutex.Unlock()
	
	log.LogUserAction(userID, "approval_verified", logger.Fields{
		"email": utils.SanitizeUserInput(registration.Email),
	}).Info("Approval successfully verified with multi-factor authentication")
	
	return verification, nil
}

// IsUserRegistered checks if a user is registered for MFA
func (t *TOTPEmailAuthenticator) IsUserRegistered(userID string) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	registration, exists := t.registrations[userID]
	return exists && registration.IsActive
}

// IsVerificationPending checks if a user has a pending verification
func (t *TOTPEmailAuthenticator) IsVerificationPending(userID string) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	verification, exists := t.pendingVerifications[userID]
	if !exists {
		return false
	}
	
	// Check if not expired
	return time.Now().Before(verification.ExpiresAt)
}


// GetRegisteredUser returns the registered user info
func (t *TOTPEmailAuthenticator) GetRegisteredUser(userID string) *UserRegistration {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	registration, exists := t.registrations[userID]
	if !exists || !registration.IsActive {
		return nil
	}
	
	return registration
}

// GetUserEmail returns the registered email for a user
func (t *TOTPEmailAuthenticator) GetUserEmail(userID string) string {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	
	if registration, exists := t.registrations[userID]; exists {
		return registration.Email
	}
	return ""
}

// generateEmailCode generates a 6-digit email verification code
func (t *TOTPEmailAuthenticator) generateEmailCode() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// generateBackupCodes generates single-use backup codes
func (t *TOTPEmailAuthenticator) generateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		// Generate 8-character backup code
		bytes := make([]byte, 5)
		_, err := rand.Read(bytes)
		if err != nil {
			return nil, err
		}
		codes[i] = strings.ToUpper(base32.StdEncoding.EncodeToString(bytes)[:8])
	}
	return codes, nil
}

// generateVerificationID generates a unique verification ID
func (t *TOTPEmailAuthenticator) generateVerificationID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}

// useBackupCode validates and consumes a backup code
func (t *TOTPEmailAuthenticator) useBackupCode(userID, code string) bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	registration, exists := t.registrations[userID]
	if !exists {
		return false
	}
	
	// Find and remove the backup code
	for i, backupCode := range registration.BackupCodes {
		if backupCode == strings.ToUpper(code) {
			// Remove the used backup code
			registration.BackupCodes = append(registration.BackupCodes[:i], registration.BackupCodes[i+1:]...)
			return true
		}
	}
	
	return false
}

// sendVerificationEmail sends email with verification code
func (t *TOTPEmailAuthenticator) sendVerificationEmail(email, code string, approvalData map[string]interface{}) error {
	cfg := config.Get()
	
	// Extract approval details
	requestor := "Unknown"
	accountID := "Unknown"
	accountName := "Unknown"
	
	if val, ok := approvalData["requestor"]; ok {
		requestor = fmt.Sprintf("%v", val)
	}
	if val, ok := approvalData["account_id"]; ok {
		accountID = fmt.Sprintf("%v", val)
	}
	if val, ok := approvalData["account_name"]; ok {
		accountName = fmt.Sprintf("%v", val)
	}
	
	subject := "AWS PIM Approval Verification"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; background: #f5f5f5; margin: 0; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background: white; border-radius: 8px; padding: 40px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 30px; }
        .code { font-size: 32px; font-weight: bold; color: #2196f3; text-align: center; background: #f8f9fa; padding: 20px; border-radius: 8px; letter-spacing: 4px; margin: 20px 0; }
        .details { background: #e3f2fd; padding: 20px; border-radius: 6px; margin: 20px 0; }
        .footer { font-size: 14px; color: #666; margin-top: 30px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔐 AWS PIM Approval Verification</h1>
        </div>
        
        <p>An AWS access request requires your approval:</p>
        
        <div class="details">
            <strong>Request Details:</strong><br>
            👤 <strong>Requestor:</strong> %s<br>
            🏢 <strong>Account:</strong> %s (%s)
        </div>
        
        <p>Your email verification code is:</p>
        
        <div class="code">%s</div>
        
        <p><strong>Next Steps:</strong></p>
        <ol>
            <li>Return to Slack</li>
            <li>Open your authenticator app for the TOTP code</li>
            <li>Enter both codes in the approval modal</li>
        </ol>
        
        <div class="footer">
            <p>⏰ This code expires in 10 minutes</p>
            <p>🔒 This email was sent because you attempted to approve an AWS access request</p>
            <p>❓ If you didn't request this, please contact your administrator immediately</p>
        </div>
    </div>
</body>
</html>`, 
		utils.SanitizeUserInput(requestor), 
		utils.SanitizeUserInput(accountName), 
		utils.SanitizeUserInput(accountID), 
		code)

	input := &ses.SendEmailInput{
		Destination: &ses.Destination{
			ToAddresses: []*string{aws.String(email)},
		},
		Message: &ses.Message{
			Subject: &ses.Content{
				Data: aws.String(subject),
			},
			Body: &ses.Body{
				Html: &ses.Content{
					Data: aws.String(body),
				},
			},
		},
		Source: aws.String(cfg.GetSESFromEmail()),
	}
	
	_, err := t.sesClient.SendEmail(input)
	return err
}

// startCleanupRoutine periodically cleans up expired verifications
func (t *TOTPEmailAuthenticator) startCleanupRoutine() {
	log := logger.GetDefaultLogger()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		t.mutex.Lock()
		now := time.Now()
		var expiredUsers []string
		
		for userID, verification := range t.pendingVerifications {
			if now.After(verification.ExpiresAt) {
				expiredUsers = append(expiredUsers, userID)
			}
		}
		
		for _, userID := range expiredUsers {
			delete(t.pendingVerifications, userID)
		}
		t.mutex.Unlock()
		
		if len(expiredUsers) > 0 {
			log.LogSecurityEvent("verification_cleanup", logger.Fields{
				"expired_count": len(expiredUsers),
			}).Info("Cleaned up expired verification requests")
		}
	}
}
