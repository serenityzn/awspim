package errors

import (
	"fmt"
)

// ErrorType represents different types of errors in the application
type ErrorType string

const (
	ErrorTypeValidation     ErrorType = "validation"
	ErrorTypeConfiguration ErrorType = "configuration"
	ErrorTypeAWS           ErrorType = "aws"
	ErrorTypeSlack         ErrorType = "slack"
	ErrorTypeSecurity      ErrorType = "security"
	ErrorTypeInternal      ErrorType = "internal"
)

// AppError represents a structured application error
type AppError struct {
	Type    ErrorType
	Message string
	Cause   error
	Context map[string]interface{}
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap returns the underlying error for error unwrapping
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithContext adds context to the error
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewValidationError creates a new validation error
func NewValidationError(message string, cause error) *AppError {
	return &AppError{
		Type:    ErrorTypeValidation,
		Message: message,
		Cause:   cause,
	}
}

// NewConfigurationError creates a new configuration error
func NewConfigurationError(message string, cause error) *AppError {
	return &AppError{
		Type:    ErrorTypeConfiguration,
		Message: message,
		Cause:   cause,
	}
}

// NewAWSError creates a new AWS-related error
func NewAWSError(message string, cause error) *AppError {
	return &AppError{
		Type:    ErrorTypeAWS,
		Message: message,
		Cause:   cause,
	}
}

// NewSlackError creates a new Slack-related error
func NewSlackError(message string, cause error) *AppError {
	return &AppError{
		Type:    ErrorTypeSlack,
		Message: message,
		Cause:   cause,
	}
}

// NewSecurityError creates a new security-related error
func NewSecurityError(message string, cause error) *AppError {
	return &AppError{
		Type:    ErrorTypeSecurity,
		Message: message,
		Cause:   cause,
	}
}

// NewInternalError creates a new internal error
func NewInternalError(message string, cause error) *AppError {
	return &AppError{
		Type:    ErrorTypeInternal,
		Message: message,
		Cause:   cause,
	}
}

// IsType checks if an error is of a specific type
func IsType(err error, errorType ErrorType) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type == errorType
	}
	return false
}

// GetType returns the error type if it's an AppError, otherwise returns ErrorTypeInternal
func GetType(err error) ErrorType {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type
	}
	return ErrorTypeInternal
}

// GetContext returns the context of an AppError, or nil if not an AppError
func GetContext(err error) map[string]interface{} {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Context
	}
	return nil
}
