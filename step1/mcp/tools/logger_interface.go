package tools

import (
	"database/sql"
	"time"
)

// Logger interface defines the logging methods that tools can use
type Logger interface {
	LogInfo(message string, data map[string]interface{})
	LogError(message string, err error, data map[string]interface{})
	LogDebug(message string, data map[string]interface{})
	LogDatabaseOperation(operation string, duration time.Duration, err error, data map[string]interface{})
	LogContainerOperation(operation string, projectName string, duration time.Duration, err error, data map[string]interface{})
	LogTestExecution(testType string, testID int64, duration time.Duration, metrics map[string]interface{})
}

// ToolContext holds the context needed by tools including logger
type ToolContext struct {
	Logger Logger
	DB     *sql.DB
}

