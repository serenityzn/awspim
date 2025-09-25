package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger wraps logrus.Logger with application-specific methods
type Logger struct {
	*logrus.Logger
}

// Fields is an alias for logrus.Fields for convenience
type Fields map[string]interface{}

var defaultLogger *Logger

// NewLogger creates a new structured logger instance
func NewLogger() *Logger {
	log := logrus.New()
	
	// Set formatter based on environment
	env := os.Getenv("ENVIRONMENT")
	if env == "production" || env == "prod" {
		// JSON formatter for production (better for log aggregation)
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z",
		})
	} else {
		// Text formatter for development (human-readable)
		log.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
			FullTimestamp:   true,
		})
	}

	// Set log level from environment or default to Info
	level := os.Getenv("LOG_LEVEL")
	switch level {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}

	log.SetOutput(os.Stdout)

	return &Logger{log}
}

// GetDefaultLogger returns the default logger instance
func GetDefaultLogger() *Logger {
	if defaultLogger == nil {
		defaultLogger = NewLogger()
	}
	return defaultLogger
}

// WithFields creates a new logger entry with the specified fields
func (l *Logger) WithFields(fields Fields) *logrus.Entry {
	return l.Logger.WithFields(logrus.Fields(fields))
}

// WithError creates a new logger entry with an error field
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// AWS-specific logging methods
func (l *Logger) LogAWSOperation(operation string, fields Fields) *logrus.Entry {
	if fields == nil {
		fields = Fields{}
	}
	fields["component"] = "aws"
	fields["operation"] = operation
	return l.WithFields(fields)
}

// Slack-specific logging methods
func (l *Logger) LogSlackOperation(operation string, fields Fields) *logrus.Entry {
	if fields == nil {
		fields = Fields{}
	}
	fields["component"] = "slack"
	fields["operation"] = operation
	return l.WithFields(fields)
}

// Application-specific logging methods
func (l *Logger) LogUserAction(userID, action string, fields Fields) *logrus.Entry {
	if fields == nil {
		fields = Fields{}
	}
	fields["user_id"] = userID
	fields["action"] = action
	fields["component"] = "user_action"
	return l.WithFields(fields)
}

// Security-specific logging methods
func (l *Logger) LogSecurityEvent(event string, fields Fields) *logrus.Entry {
	if fields == nil {
		fields = Fields{}
	}
	fields["component"] = "security"
	fields["event"] = event
	return l.WithFields(fields)
}

// Package-level convenience functions using default logger
func Info(msg string) {
	GetDefaultLogger().Info(msg)
}

func InfoWithFields(msg string, fields Fields) {
	GetDefaultLogger().WithFields(fields).Info(msg)
}

func Error(msg string) {
	GetDefaultLogger().Error(msg)
}

func ErrorWithFields(msg string, fields Fields) {
	GetDefaultLogger().WithFields(fields).Error(msg)
}

func Warn(msg string) {
	GetDefaultLogger().Warn(msg)
}

func WarnWithFields(msg string, fields Fields) {
	GetDefaultLogger().WithFields(fields).Warn(msg)
}

func Debug(msg string) {
	GetDefaultLogger().Debug(msg)
}

func DebugWithFields(msg string, fields Fields) {
	GetDefaultLogger().WithFields(fields).Debug(msg)
}

func WithError(err error) *logrus.Entry {
	return GetDefaultLogger().WithError(err)
}
