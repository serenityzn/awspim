package auth

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/serenityzn/awspim/pkg/errors"
	"github.com/serenityzn/awspim/pkg/logger"
)

// SecretsManagerStorage implements RegistrationStorage using AWS Secrets Manager
type SecretsManagerStorage struct {
	client     *secretsmanager.SecretsManager
	secretName string
	cache      map[string]*UserRegistration
	mutex      sync.RWMutex
}

// StoredRegistrations is the JSON structure stored in Secrets Manager
type StoredRegistrations struct {
	Version       string                       `json:"version"`
	LastUpdated   time.Time                    `json:"last_updated"`
	Registrations map[string]*UserRegistration `json:"registrations"`
}

// NewSecretsManagerStorage creates a new Secrets Manager storage backend
func NewSecretsManagerStorage(client *secretsmanager.SecretsManager, secretName string) (*SecretsManagerStorage, error) {
	log := logger.GetDefaultLogger()
	
	storage := &SecretsManagerStorage{
		client:     client,
		secretName: secretName,
		cache:      make(map[string]*UserRegistration),
	}
	
	// Load initial data from Secrets Manager
	if err := storage.load(); err != nil {
		log.LogSecurityEvent("storage_initialization_failed", logger.Fields{
			"backend":     "secretsmanager",
			"secret_name": secretName,
		}).WithError(err).Warn("Failed to load initial data from Secrets Manager, starting with empty storage")
		
		// Initialize empty secret if it doesn't exist
		if err := storage.initializeSecret(); err != nil {
			return nil, errors.NewInternalError("failed to initialize secret", err)
		}
	}
	
	log.LogSecurityEvent("storage_initialized", logger.Fields{
		"backend":     "secretsmanager",
		"secret_name": secretName,
		"user_count":  len(storage.cache),
	}).Info("Secrets Manager storage initialized successfully")
	
	return storage, nil
}

// SaveRegistration stores a user registration
func (s *SecretsManagerStorage) SaveRegistration(userID string, registration *UserRegistration) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	s.cache[userID] = registration
	return s.persist()
}

// GetRegistration retrieves a user registration
func (s *SecretsManagerStorage) GetRegistration(userID string) (*UserRegistration, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	registration, exists := s.cache[userID]
	if !exists {
		return nil, errors.NewInternalError("user registration not found", nil).
			WithContext("user_id", userID)
	}
	
	return registration, nil
}

// DeleteRegistration removes a user registration
func (s *SecretsManagerStorage) DeleteRegistration(userID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	delete(s.cache, userID)
	return s.persist()
}

// ListRegistrations returns all user registrations
func (s *SecretsManagerStorage) ListRegistrations() (map[string]*UserRegistration, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	// Return a copy to prevent external modification
	copy := make(map[string]*UserRegistration, len(s.cache))
	for k, v := range s.cache {
		copy[k] = v
	}
	
	return copy, nil
}

// UpdateLastUsed updates the last used timestamp for a user
func (s *SecretsManagerStorage) UpdateLastUsed(userID string, lastUsedAt time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	registration, exists := s.cache[userID]
	if !exists {
		return errors.NewInternalError("user registration not found", nil).
			WithContext("user_id", userID)
	}
	
	registration.LastUsedAt = lastUsedAt
	return s.persist()
}

// RemoveBackupCode removes a used backup code from a user's registration
func (s *SecretsManagerStorage) RemoveBackupCode(userID string, codeIndex int) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	registration, exists := s.cache[userID]
	if !exists {
		return errors.NewInternalError("user registration not found", nil).
			WithContext("user_id", userID)
	}
	
	if codeIndex < 0 || codeIndex >= len(registration.BackupCodes) {
		return errors.NewValidationError("invalid backup code index", nil).
			WithContext("index", codeIndex)
	}
	
	// Remove the backup code at the specified index
	registration.BackupCodes = append(
		registration.BackupCodes[:codeIndex],
		registration.BackupCodes[codeIndex+1:]...,
	)
	
	return s.persist()
}

// load reads all registrations from Secrets Manager into cache
func (s *SecretsManagerStorage) load() error {
	log := logger.GetDefaultLogger()
	
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(s.secretName),
	}
	
	result, err := s.client.GetSecretValue(input)
	if err != nil {
		return err
	}
	
	if result.SecretString == nil {
		return fmt.Errorf("secret contains no data")
	}
	
	var stored StoredRegistrations
	if err := json.Unmarshal([]byte(*result.SecretString), &stored); err != nil {
		return errors.NewInternalError("failed to unmarshal stored registrations", err)
	}
	
	// Load into cache
	s.cache = stored.Registrations
	if s.cache == nil {
		s.cache = make(map[string]*UserRegistration)
	}
	
	log.LogSecurityEvent("storage_loaded", logger.Fields{
		"secret_name":  s.secretName,
		"user_count":   len(s.cache),
		"version":      stored.Version,
		"last_updated": stored.LastUpdated,
	}).Info("Loaded user registrations from Secrets Manager")
	
	return nil
}

// persist writes all registrations from cache to Secrets Manager
func (s *SecretsManagerStorage) persist() error {
	log := logger.GetDefaultLogger()
	
	stored := StoredRegistrations{
		Version:       "1.0",
		LastUpdated:   time.Now(),
		Registrations: s.cache,
	}
	
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return errors.NewInternalError("failed to marshal registrations", err)
	}
	
	input := &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(s.secretName),
		SecretString: aws.String(string(data)),
	}
	
	_, err = s.client.UpdateSecret(input)
	if err != nil {
		log.LogSecurityEvent("storage_persist_failed", logger.Fields{
			"secret_name": s.secretName,
			"user_count":  len(s.cache),
		}).WithError(err).Error("Failed to persist registrations to Secrets Manager")
		return errors.NewInternalError("failed to update secret", err)
	}
	
	log.LogSecurityEvent("storage_persisted", logger.Fields{
		"secret_name": s.secretName,
		"user_count":  len(s.cache),
	}).Debug("Persisted user registrations to Secrets Manager")
	
	return nil
}

// initializeSecret creates an empty secret if it doesn't exist
func (s *SecretsManagerStorage) initializeSecret() error {
	log := logger.GetDefaultLogger()
	
	stored := StoredRegistrations{
		Version:       "1.0",
		LastUpdated:   time.Now(),
		Registrations: make(map[string]*UserRegistration),
	}
	
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return errors.NewInternalError("failed to marshal initial data", err)
	}
	
	// Try to create the secret
	createInput := &secretsmanager.CreateSecretInput{
		Name:         aws.String(s.secretName),
		Description:  aws.String("AWS PIM - User 2FA registrations (TOTP + Email)"),
		SecretString: aws.String(string(data)),
	}
	
	_, err = s.client.CreateSecret(createInput)
	if err != nil {
		// If secret already exists, try to update it
		updateInput := &secretsmanager.UpdateSecretInput{
			SecretId:     aws.String(s.secretName),
			SecretString: aws.String(string(data)),
		}
		
		_, err = s.client.UpdateSecret(updateInput)
		if err != nil {
			return err
		}
	}
	
	log.LogSecurityEvent("storage_initialized_empty", logger.Fields{
		"secret_name": s.secretName,
	}).Info("Initialized empty Secrets Manager secret for user registrations")
	
	return nil
}

