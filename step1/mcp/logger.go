package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log entry
type LogLevel string

const (
	LogLevelDEBUG LogLevel = "DEBUG"
	LogLevelINFO  LogLevel = "INFO"
	LogLevelWARN  LogLevel = "WARN"
	LogLevelERROR LogLevel = "ERROR"
	LogLevelFATAL LogLevel = "FATAL"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Level       LogLevel               `json:"level"`
	Timestamp   string                 `json:"timestamp"`
	Message     string                 `json:"message"`
	Error       string                 `json:"error,omitempty"`
	RequestID   string                 `json:"request_id,omitempty"`
	Tool        string                 `json:"tool,omitempty"`
	Duration    string                 `json:"duration,omitempty"`
	SessionID   int64                  `json:"session_id,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Stack       string                 `json:"stack,omitempty"`
	Component   string                 `json:"component,omitempty"`
	Operation   string                 `json:"operation,omitempty"`
	UserContext map[string]interface{} `json:"user_context,omitempty"`
}

var (
	fileLogger *log.Logger
	logLevel   LogLevel = LogLevelINFO
	logMutex   sync.RWMutex
)

// InitializeLogging sets up the logging system
func InitializeLogging() {
	// Set log level from environment
	if level := os.Getenv("MCP_LOG_LEVEL"); level != "" {
		logLevel = LogLevel(level)
	}

	// Use environment variable or current directory
	logDir := os.Getenv("MCP_LOG_DIR")
	if logDir == "" {
		// Try to use a standard location
		homeDir, err := os.UserHomeDir()
		if err != nil {
			logDir = "logs"
		} else {
			logDir = filepath.Join(homeDir, ".speak-perf-mcp", "logs")
		}
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Failed to create logs directory: %v", err)
		return
	}

	// Create log file with timestamp
	logFile := filepath.Join(logDir, fmt.Sprintf("mcp-server-step1-%s.log", time.Now().Format("2006-01-02-15-04-05")))
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
		return
	}

	fileLogger = log.New(file, "", 0) // No prefix for clean JSON
	logWithLevel(LogLevelINFO, "MCP Server Step1 logging initialized", nil, map[string]interface{}{
		"logFile":    logFile,
		"logLevel":   logLevel,
		"version":    "1.0.0",
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	})
}

// shouldLog checks if a log level should be logged based on current log level
func shouldLog(level LogLevel) bool {
	levels := map[LogLevel]int{
		LogLevelDEBUG: 0,
		LogLevelINFO:  1,
		LogLevelWARN:  2,
		LogLevelERROR: 3,
		LogLevelFATAL: 4,
	}
	return levels[level] >= levels[logLevel]
}

// logWithLevel is the core logging function that handles all log entries
func logWithLevel(level LogLevel, message string, err error, data map[string]interface{}) {
	if !shouldLog(level) {
		return
	}

	logMutex.Lock()
	defer logMutex.Unlock()

	if fileLogger != nil {
		entry := LogEntry{
			Level:     level,
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Message:   message,
			Data:      data,
		}

		if err != nil {
			entry.Error = err.Error()
		}

		// Add stack trace for errors and above
		if level == LogLevelERROR || level == LogLevelFATAL {
			entry.Stack = getStackTrace()
		}

		jsonData, _ := json.Marshal(entry)
		fileLogger.Println(string(jsonData))

		// Also log to stderr for errors and fatal
		if level == LogLevelERROR || level == LogLevelFATAL {
			log.Printf("[%s] %s: %s", level, message, entry.Error)
		}
	}
}

// Basic logging functions
func LogDebug(message string, data map[string]interface{}) {
	logWithLevel(LogLevelDEBUG, message, nil, data)
}

func LogInfo(message string, data map[string]interface{}) {
	logWithLevel(LogLevelINFO, message, nil, data)
}

func LogWarn(message string, data map[string]interface{}) {
	logWithLevel(LogLevelWARN, message, nil, data)
}

func LogError(message string, err error, data map[string]interface{}) {
	logWithLevel(LogLevelERROR, message, err, data)
}

func LogFatal(message string, err error, data map[string]interface{}) {
	logWithLevel(LogLevelFATAL, message, err, data)
}

// Enhanced logging functions with context
func LogToolStart(tool string, requestID string, params map[string]interface{}) {
	LogInfo("Tool execution started", map[string]interface{}{
		"tool":       tool,
		"request_id": requestID,
		"params":     params,
		"component":  "tool_handler",
	})
}

func LogToolEnd(tool string, requestID string, duration time.Duration, success bool, data map[string]interface{}) {
	level := LogLevelINFO
	if !success {
		level = LogLevelERROR
	}

	logData := map[string]interface{}{
		"tool":       tool,
		"request_id": requestID,
		"duration":   duration.String(),
		"success":    success,
		"component":  "tool_handler",
	}

	for k, v := range data {
		logData[k] = v
	}

	logWithLevel(level, "Tool execution completed", nil, logData)
}

func LogDatabaseOperation(operation string, duration time.Duration, err error, data map[string]interface{}) {
	level := LogLevelDEBUG
	if err != nil {
		level = LogLevelERROR
	}

	logData := map[string]interface{}{
		"operation": operation,
		"duration":  duration.String(),
		"component": "database",
	}

	for k, v := range data {
		logData[k] = v
	}

	logWithLevel(level, "Database operation", err, logData)
}

func LogContainerOperation(operation string, projectName string, duration time.Duration, err error, data map[string]interface{}) {
	level := LogLevelINFO
	if err != nil {
		level = LogLevelERROR
	}

	logData := map[string]interface{}{
		"operation":    operation,
		"project_name": projectName,
		"duration":     duration.String(),
		"component":    "container",
	}

	for k, v := range data {
		logData[k] = v
	}

	logWithLevel(level, "Container operation", err, logData)
}

func LogTestExecution(testType string, testID int64, duration time.Duration, metrics map[string]interface{}) {
	LogInfo("Test execution completed", map[string]interface{}{
		"test_type": testType,
		"test_id":   testID,
		"duration":  duration.String(),
		"metrics":   metrics,
		"component": "test_runner",
	})
}

func LogPerformanceMetrics(operation string, duration time.Duration, data map[string]interface{}) {
	logData := map[string]interface{}{
		"operation": operation,
		"duration":  duration.String(),
		"component": "performance",
	}

	for k, v := range data {
		logData[k] = v
	}

	LogDebug("Performance metrics", logData)
}

// getStackTrace returns the current stack trace as a string
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// GenerateRequestID generates a unique request ID for tracking
func GenerateRequestID() string {
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), os.Getpid())
}
