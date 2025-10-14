package auth

import (
	"time"
)

// RegistrationStorage defines the interface for persisting user 2FA registrations
type RegistrationStorage interface {
	// SaveRegistration stores a user registration
	SaveRegistration(userID string, registration *UserRegistration) error
	
	// GetRegistration retrieves a user registration
	GetRegistration(userID string) (*UserRegistration, error)
	
	// DeleteRegistration removes a user registration
	DeleteRegistration(userID string) error
	
	// ListRegistrations returns all user registrations
	ListRegistrations() (map[string]*UserRegistration, error)
	
	// UpdateLastUsed updates the last used timestamp for a user
	UpdateLastUsed(userID string, lastUsedAt time.Time) error
	
	// RemoveBackupCode removes a used backup code from a user's registration
	RemoveBackupCode(userID string, codeIndex int) error
}

// StorageConfig holds configuration for storage backends
type StorageConfig struct {
	// Backend type: "secretsmanager", "dynamodb", "file", etc.
	Backend string
	
	// Secrets Manager specific
	SecretName string
	Region     string
	
	// DynamoDB specific (for future use)
	TableName string
	
	// File storage specific (for future use)
	FilePath string
}

