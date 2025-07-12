package main

import (
	"time"
)

// LoggerAdapter implements the Logger interface using the main package's logging functions
type LoggerAdapter struct{}

// LogInfo logs an info message
func (l *LoggerAdapter) LogInfo(message string, data map[string]interface{}) {
	LogInfo(message, data)
}

// LogError logs an error message
func (l *LoggerAdapter) LogError(message string, err error, data map[string]interface{}) {
	LogError(message, err, data)
}

// LogDebug logs a debug message
func (l *LoggerAdapter) LogDebug(message string, data map[string]interface{}) {
	LogDebug(message, data)
}

// LogDatabaseOperation logs a database operation
func (l *LoggerAdapter) LogDatabaseOperation(operation string, duration time.Duration, err error, data map[string]interface{}) {
	LogDatabaseOperation(operation, duration, err, data)
}

// LogContainerOperation logs a container operation
func (l *LoggerAdapter) LogContainerOperation(operation string, projectName string, duration time.Duration, err error, data map[string]interface{}) {
	LogContainerOperation(operation, projectName, duration, err, data)
}

// LogTestExecution logs test execution
func (l *LoggerAdapter) LogTestExecution(testType string, testID int64, duration time.Duration, metrics map[string]interface{}) {
	LogTestExecution(testType, testID, duration, metrics)
}