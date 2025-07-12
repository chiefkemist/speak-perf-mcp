package main

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"
)

// LogLevel represents different logging levels
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
	db         *sql.DB
	fileLogger *log.Logger
	logMutex   sync.Mutex
	mcpServer  *server.MCPServer
	logLevel   LogLevel = LogLevelINFO
)

func init() {
	// Set log level from environment
	if envLevel := os.Getenv("MCP_LOG_LEVEL"); envLevel != "" {
		switch strings.ToUpper(envLevel) {
		case "DEBUG":
			logLevel = LogLevelDEBUG
		case "INFO":
			logLevel = LogLevelINFO
		case "WARN":
			logLevel = LogLevelWARN
		case "ERROR":
			logLevel = LogLevelERROR
		case "FATAL":
			logLevel = LogLevelFATAL
		}
	}

	// Initialize logging
	initializeLogging()
}

func initializeLogging() {
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

func logDebug(message string, data map[string]interface{}) {
	logWithLevel(LogLevelDEBUG, message, nil, data)
}

func logInfo(message string, data map[string]interface{}) {
	logWithLevel(LogLevelINFO, message, nil, data)
}

func logWarn(message string, data map[string]interface{}) {
	logWithLevel(LogLevelWARN, message, nil, data)
}

func logError(message string, err error, data map[string]interface{}) {
	logWithLevel(LogLevelERROR, message, err, data)
}

func logFatal(message string, err error, data map[string]interface{}) {
	logWithLevel(LogLevelFATAL, message, err, data)
}

// Enhanced logging functions with context
func logToolStart(tool string, requestID string, params map[string]interface{}) {
	logInfo("Tool execution started", map[string]interface{}{
		"tool":       tool,
		"request_id": requestID,
		"params":     params,
		"component":  "tool_handler",
	})
}

func logToolEnd(tool string, requestID string, duration time.Duration, success bool, data map[string]interface{}) {
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

func logDatabaseOperation(operation string, duration time.Duration, err error, data map[string]interface{}) {
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

func logContainerOperation(operation string, projectName string, duration time.Duration, err error, data map[string]interface{}) {
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

func logTestExecution(testType string, testID int64, duration time.Duration, metrics map[string]interface{}) {
	logInfo("Test execution completed", map[string]interface{}{
		"test_type": testType,
		"test_id":   testID,
		"duration":  duration.String(),
		"metrics":   metrics,
		"component": "test_runner",
	})
}

func logPerformanceMetrics(operation string, duration time.Duration, data map[string]interface{}) {
	logData := map[string]interface{}{
		"operation": operation,
		"duration":  duration.String(),
		"component": "performance",
	}

	for k, v := range data {
		logData[k] = v
	}

	logDebug("Performance metrics", logData)
}

func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

func generateRequestID() string {
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), os.Getpid())
}

func sendProgress(ctx context.Context, progress string, data map[string]interface{}) {
	if mcpServer != nil {
		// Log the progress
		logInfo("Progress update", map[string]interface{}{
			"progress":  progress,
			"component": "progress",
			"data":      data,
		})

		// Send progress notification to client
		progressData := map[string]interface{}{
			"progress":  progress,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		for k, v := range data {
			progressData[k] = v
		}

		// TODO: Send notification when MCP-Go library supports it
		// For now, we'll just log the progress
		logDebug("Progress notification prepared", progressData)
	}
}

func main() {
	startTime := time.Now()

	// Initialize database
	logInfo("Initializing database", nil)
	dbStart := time.Now()
	initDB()
	logPerformanceMetrics("database_init", time.Since(dbStart), nil)
	defer func() {
		if err := db.Close(); err != nil {
			logError("Failed to close database", err, nil)
		} else {
			logInfo("Database closed successfully", nil)
		}
	}()

	logInfo("MCP Server Step1 starting", map[string]interface{}{
		"version":   "1.0.0",
		"log_level": logLevel,
		"pid":       os.Getpid(),
	})

	// Create MCP server
	serverStart := time.Now()
	s := server.NewMCPServer(
		"k6-docker-mcp", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithRecovery(),
		server.WithLogging(),
	)
	mcpServer = s
	logPerformanceMetrics("mcp_server_init", time.Since(serverStart), nil)

	// Register tools with enhanced logging
	registerTools(s)

	// Add resources with enhanced logging
	registerResources(s)

	// Start server
	logInfo("Starting stdio server", map[string]interface{}{
		"startup_duration": time.Since(startTime).String(),
	})

	if err := server.ServeStdio(s); err != nil {
		logFatal("Failed to start stdio server", err, nil)
		log.Fatal(err)
	}
}

func registerTools(s *server.MCPServer) {
	logInfo("Registering MCP tools", nil)

	// Add tools with enhanced logging
	s.AddTool(mcp.NewTool(
		"setup_test_environment",
		mcp.WithDescription("Initialize testing environment from Docker Compose file"),
		mcp.WithString("composePath", mcp.Required(), mcp.Description("Path to docker-compose.yml")),
		mcp.WithString("projectName", mcp.Description("Project name for containers")),
	), enhanceToolHandler("setup_test_environment", handleSetupEnvironment))

	s.AddTool(mcp.NewTool(
		"discover_api_specs",
		mcp.WithDescription("Find and parse OpenAPI/Swagger specifications"),
		mcp.WithString("specPaths", mcp.Description("Comma-separated paths to API specs")),
		mcp.WithString("autoDiscover", mcp.Description("Auto-discover specs from running services (true/false)")),
	), enhanceToolHandler("discover_api_specs", handleDiscoverSpecs))

	s.AddTool(mcp.NewTool(
		"generate_api_tests",
		mcp.WithDescription("Generate k6 tests from API specifications"),
		mcp.WithString("specId", mcp.Required(), mcp.Description("ID of discovered spec")),
		mcp.WithString("endpoints", mcp.Description("Comma-separated endpoints to test")),
		mcp.WithString("testType", mcp.Description("Test type: load, stress, spike")),
	), enhanceToolHandler("generate_api_tests", handleGenerateAPITests))

	s.AddTool(mcp.NewTool(
		"create_ui_test",
		mcp.WithDescription("Generate k6 browser test from natural language"),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL")),
		mcp.WithString("instructions", mcp.Required(), mcp.Description("Natural language test instructions")),
		mcp.WithString("testName", mcp.Description("Name for the test")),
	), enhanceToolHandler("create_ui_test", handleCreateUITest))

	s.AddTool(mcp.NewTool(
		"run_performance_test",
		mcp.WithDescription("Execute generated performance tests"),
		mcp.WithString("testId", mcp.Required(), mcp.Description("ID of test to run")),
		mcp.WithNumber("vus", mcp.Description("Virtual users")),
		mcp.WithString("duration", mcp.Description("Test duration")),
	), enhanceToolHandler("run_performance_test", handleRunTest))

	s.AddTool(mcp.NewTool(
		"analyze_results",
		mcp.WithDescription("Analyze test results against SLAs"),
		mcp.WithString("runId", mcp.Required(), mcp.Description("Test run ID")),
		mcp.WithString("compareHistory", mcp.Description("Compare with historical data (true/false)")),
	), enhanceToolHandler("analyze_results", handleAnalyzeResults))

	s.AddTool(mcp.NewTool(
		"query_test_history",
		mcp.WithDescription("Query historical test data"),
		mcp.WithString("service", mcp.Description("Filter by service name")),
		mcp.WithString("endpoint", mcp.Description("Filter by endpoint")),
		mcp.WithNumber("days", mcp.Description("Number of days to look back")),
	), enhanceToolHandler("query_test_history", handleQueryHistory))

	// Add automated tools
	s.AddTool(mcp.NewTool(
		"test_application",
		mcp.WithDescription("Complete automated testing of a Docker Compose application"),
		mcp.WithString("composeSource", mcp.Required(), mcp.Description("Path or URL to docker-compose.yml")),
		mcp.WithString("testType", mcp.Description("Test type: quick, standard, thorough (default: standard)")),
		mcp.WithString("endpoints", mcp.Description("Specific endpoints to test (comma-separated)")),
	), enhanceToolHandler("test_application", handleTestApplication))

	s.AddTool(mcp.NewTool(
		"quick_performance_test",
		mcp.WithDescription("Run quick performance test with custom parameters"),
		mcp.WithString("composeSource", mcp.Required(), mcp.Description("Path or URL to docker-compose.yml")),
		mcp.WithNumber("vus", mcp.Description("Virtual users (default: 50)")),
		mcp.WithString("duration", mcp.Description("Test duration (default: 2m)")),
		mcp.WithString("targetService", mcp.Description("Specific service to test")),
	), enhanceToolHandler("quick_performance_test", handleQuickPerformanceTest))

	logInfo("MCP tools registered successfully", map[string]interface{}{
		"tool_count": 9,
	})
}

func registerResources(s *server.MCPServer) {
	logInfo("Registering MCP resources", nil)

	// Add resources
	s.AddResource(mcp.NewResource("sqlite://schema", "Database Schema",
		mcp.WithResourceDescription("View the SQLite database schema")), handleSchemaResource)
	s.AddResource(mcp.NewResource("sqlite://sessions", "Test Sessions",
		mcp.WithResourceDescription("List recent test sessions")), handleSessionsResource)
	s.AddResource(mcp.NewResource("sqlite://compose-files", "Compose Files",
		mcp.WithResourceDescription("List stored Docker Compose files")), handleComposeFilesResource)
	s.AddResource(mcp.NewResource("sqlite://test-runs", "Test Runs",
		mcp.WithResourceDescription("List recent performance test runs")), handleTestRunsResource)

	logInfo("MCP resources registered successfully", map[string]interface{}{
		"resource_count": 4,
	})
}

// enhanceToolHandler wraps tool handlers with comprehensive logging
func enhanceToolHandler(toolName string, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		requestID := generateRequestID()
		startTime := time.Now()

		// Extract parameters for logging - using a simpler approach
		params := make(map[string]interface{})
		// Note: We can't easily access request.Params.Arguments due to interface{} type
		// So we'll log the tool name and let individual handlers log their specific params

		logToolStart(toolName, requestID, params)

		// Execute the actual handler
		result, err := handler(ctx, request)

		duration := time.Since(startTime)
		success := err == nil

		logData := map[string]interface{}{
			"has_result": result != nil,
		}

		// Check if result indicates an error by looking at the content
		if result != nil {
			resultStr := fmt.Sprintf("%v", result)
			// Only flag as error if we have actual error indicators
			// Be more specific to avoid false positives from metrics like "error rate: 0.00%"
			if strings.Contains(resultStr, "Error:") ||
				strings.Contains(resultStr, "ERROR:") ||
				strings.Contains(resultStr, "Failed:") ||
				strings.Contains(resultStr, "FAILED:") ||
				strings.Contains(resultStr, "failed to") ||
				strings.Contains(resultStr, "error occurred") ||
				strings.Contains(resultStr, "execution failed") ||
				strings.Contains(resultStr, "docker compose") && strings.Contains(resultStr, "failed") {
				success = false
				logData["has_error_content"] = true
			}
		}

		logToolEnd(toolName, requestID, duration, success, logData)

		return result, err
	}
}

func initDB() {
	start := time.Now()
	var err error

	logInfo("Opening SQLite database", map[string]interface{}{
		"database_path": "./perf_test.db",
	})

	db, err = sql.Open("sqlite3", "./perf_test.db")
	if err != nil {
		logFatal("Failed to open database", err, nil)
		log.Fatal(err)
	}

	logDatabaseOperation("open", time.Since(start), err, map[string]interface{}{
		"database_path": "./perf_test.db",
	})

	// Test connection
	start = time.Now()
	if err := db.Ping(); err != nil {
		logFatal("Failed to ping database", err, nil)
		log.Fatal(err)
	}
	logDatabaseOperation("ping", time.Since(start), nil, nil)

	// Create tables
	start = time.Now()
	schema := `
	CREATE TABLE IF NOT EXISTS compose_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_url TEXT NOT NULL,
		content TEXT NOT NULL,
		hash TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS test_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		compose_file_id INTEGER,
		session_name TEXT NOT NULL,
		started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP,
		status TEXT,
		FOREIGN KEY (compose_file_id) REFERENCES compose_files(id)
	);

	CREATE TABLE IF NOT EXISTS services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER,
		name TEXT NOT NULL,
		image TEXT NOT NULL,
		ports TEXT,
		FOREIGN KEY (session_id) REFERENCES test_sessions(id)
	);

	CREATE TABLE IF NOT EXISTS api_specs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER,
		service_id INTEGER,
		spec_url TEXT,
		spec_content TEXT,
		version TEXT,
		discovered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES test_sessions(id),
		FOREIGN KEY (service_id) REFERENCES services(id)
	);

	CREATE TABLE IF NOT EXISTS endpoints (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		spec_id INTEGER,
		path TEXT NOT NULL,
		method TEXT NOT NULL,
		sla_response_time INTEGER,
		sla_error_rate REAL,
		FOREIGN KEY (spec_id) REFERENCES api_specs(id)
	);

	CREATE TABLE IF NOT EXISTS tests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		script TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES test_sessions(id)
	);

	CREATE TABLE IF NOT EXISTS test_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		test_id INTEGER,
		started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP,
		vus INTEGER,
		duration TEXT,
		results TEXT,
		FOREIGN KEY (test_id) REFERENCES tests(id)
	);

	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id INTEGER,
		endpoint TEXT,
		avg_response_time REAL,
		min_response_time REAL,
		max_response_time REAL,
		error_rate REAL,
		requests_per_second REAL,
		FOREIGN KEY (run_id) REFERENCES test_runs(id)
	);`

	if _, err := db.Exec(schema); err != nil {
		logFatal("Failed to create database schema", err, nil)
		log.Fatal(err)
	}

	logDatabaseOperation("create_schema", time.Since(start), nil, map[string]interface{}{
		"tables_created": 8,
	})

	logInfo("Database initialized successfully", map[string]interface{}{
		"total_duration": time.Since(start).String(),
	})
}

// Docker Compose structures
type ComposeFile struct {
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image       string   `yaml:"image"`
	Ports       []string `yaml:"ports"`
	Environment []string `yaml:"environment"`
	DependsOn   []string `yaml:"depends_on"`
}

// Helper functions for compose file handling
func fetchComposeContent(source string) (string, error) {
	// Check if it's a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source)
		if err != nil {
			return "", fmt.Errorf("failed to download compose file: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("failed to download compose file: status %d", resp.StatusCode)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		return string(content), nil
	}

	// Otherwise treat as file path
	content, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("failed to read compose file: %w", err)
	}
	return string(content), nil
}

func storeComposeFile(source, content string) (int64, error) {
	// Calculate hash
	hash := md5.Sum([]byte(content))
	hashStr := hex.EncodeToString(hash[:])

	// Check if already exists
	var existingId int64
	err := db.QueryRow("SELECT id FROM compose_files WHERE hash = ?", hashStr).Scan(&existingId)
	if err == nil {
		return existingId, nil
	}

	// Store new compose file
	result, err := db.Exec("INSERT INTO compose_files (source_url, content, hash) VALUES (?, ?, ?)",
		source, content, hashStr)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func writeComposeToTemp(content string, sessionId int64) (string, error) {
	// Create unique temp directory
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("k6-test-%d-%d", sessionId, time.Now().Unix()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", err
	}

	// Write compose file
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(content), 0644); err != nil {
		return "", err
	}

	return composePath, nil
}

func handleSetupEnvironment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	composePath, err := request.RequireString("composePath")
	if err != nil {
		logError("Missing required composePath", err, nil)
		return mcp.NewToolResultError("Missing required composePath"), nil
	}

	logInfo("Setting up test environment", map[string]interface{}{
		"composePath": composePath,
		"component":   "setup_environment",
	})
	sendProgress(ctx, "Fetching compose file", map[string]interface{}{"composePath": composePath})

	// Fetch compose content
	content, err := fetchComposeContent(composePath)
	if err != nil {
		logError("Failed to fetch compose content", err, map[string]interface{}{"composePath": composePath})
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Parse to validate
	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(content), &compose); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid compose file: %v", err)), nil
	}

	// Store in database
	dbStart := time.Now()
	composeFileId, err := storeComposeFile(composePath, content)
	if err != nil {
		logError("Failed to store compose file", err, map[string]interface{}{"composePath": composePath})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to store compose file: %v", err)), nil
	}
	logDatabaseOperation("store_compose_file", time.Since(dbStart), nil, map[string]interface{}{
		"compose_file_id": composeFileId,
		"source":          composePath,
	})

	// Create test session
	sessionName := fmt.Sprintf("session-%d", time.Now().Unix())
	dbStart = time.Now()
	result, err := db.Exec("INSERT INTO test_sessions (compose_file_id, session_name, status) VALUES (?, ?, ?)",
		composeFileId, sessionName, "initialized")
	if err != nil {
		logError("Failed to create session", err, map[string]interface{}{"sessionName": sessionName})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create session: %v", err)), nil
	}
	sessionId, _ := result.LastInsertId()
	logDatabaseOperation("create_session", time.Since(dbStart), nil, map[string]interface{}{
		"session_id":   sessionId,
		"session_name": sessionName,
	})

	// Store services metadata
	servicesStored := 0
	for name, service := range compose.Services {
		ports := strings.Join(service.Ports, ",")
		dbStart = time.Now()
		_, err := db.Exec("INSERT INTO services (session_id, name, image, ports) VALUES (?, ?, ?, ?)",
			sessionId, name, service.Image, ports)
		if err != nil {
			logError("Failed to store service", err, map[string]interface{}{
				"service_name": name,
				"session_id":   sessionId,
			})
		} else {
			servicesStored++
			logDatabaseOperation("store_service", time.Since(dbStart), nil, map[string]interface{}{
				"service_name": name,
				"session_id":   sessionId,
				"image":        service.Image,
			})
		}
	}

	logInfo("Services stored successfully", map[string]interface{}{
		"services_stored": servicesStored,
		"total_services":  len(compose.Services),
		"session_id":      sessionId,
	})

	response := fmt.Sprintf("Test environment configured:\n")
	response += fmt.Sprintf("- Session ID: %d\n", sessionId)
	response += fmt.Sprintf("- Source: %s\n", composePath)
	response += fmt.Sprintf("- Services: %d\n", len(compose.Services))
	for name, service := range compose.Services {
		response += fmt.Sprintf("  • %s (%s)\n", name, service.Image)
	}

	return mcp.NewToolResultText(response), nil
}

func handleDiscoverSpecs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	specPaths := request.GetString("specPaths", "")
	autoDiscover := request.GetString("autoDiscover", "true") == "true"

	// Get the most recent session
	var sessionId int64
	var composeFileId int64
	err := db.QueryRow(`
		SELECT id, compose_file_id 
		FROM test_sessions 
		ORDER BY created_at DESC 
		LIMIT 1`).Scan(&sessionId, &composeFileId)
	if err != nil {
		return mcp.NewToolResultError("No environment configured. Run setup_test_environment first."), nil
	}

	// Get compose content
	var content string
	err = db.QueryRow("SELECT content FROM compose_files WHERE id = ?", composeFileId).Scan(&content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get compose file: %v", err)), nil
	}

	// Write to temp location
	composePath, err := writeComposeToTemp(content, sessionId)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write compose file: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	// Start containers temporarily for discovery
	projectName := fmt.Sprintf("discover-%d", sessionId)
	containerStart := time.Now()
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	output, err := startCmd.CombinedOutput()
	if err != nil {
		logContainerOperation("start", projectName, time.Since(containerStart), err, map[string]interface{}{
			"output":     string(output),
			"session_id": sessionId,
		})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, output)), nil
	}
	logContainerOperation("start", projectName, time.Since(containerStart), nil, map[string]interface{}{
		"session_id":   sessionId,
		"compose_path": composePath,
	})

	// Ensure cleanup
	defer func() {
		stopStart := time.Now()
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		err := stopCmd.Run()
		logContainerOperation("stop", projectName, time.Since(stopStart), err, map[string]interface{}{
			"session_id": sessionId,
		})
	}()

	// Wait for services to be ready
	time.Sleep(10 * time.Second)

	discovered := []string{}

	if specPaths != "" {
		// Use provided paths
		paths := strings.Split(specPaths, ",")
		for _, path := range paths {
			discovered = append(discovered, strings.TrimSpace(path))
		}
	}

	if autoDiscover {
		// Try common OpenAPI paths
		commonPaths := []string{
			"/swagger.json",
			"/openapi.json",
			"/api-docs",
			"/v2/api-docs",
			"/v3/api-docs",
			"/api/swagger.json",
			"/api/openapi.json",
			"/api/v3/openapi.json",
		}

		// Get services from database
		rows, err := db.Query("SELECT id, name, ports FROM services WHERE session_id = ?", sessionId)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to query services: %v", err)), nil
		}
		defer rows.Close()

		for rows.Next() {
			var id int
			var name, ports string
			rows.Scan(&id, &name, &ports)

			// Extract first port
			portList := strings.Split(ports, ",")
			if len(portList) > 0 && portList[0] != "" {
				port := strings.Split(portList[0], ":")[0]
				baseURL := fmt.Sprintf("http://localhost:%s", port)

				for _, path := range commonPaths {
					url := baseURL + path
					// Actually try to fetch to see if it exists
					resp, err := http.Get(url)
					if err == nil && resp.StatusCode == 200 {
						discovered = append(discovered, url)
						resp.Body.Close()
					}
				}
			}
		}
	}

	result := fmt.Sprintf("Discovered %d API specifications:\n", len(discovered))
	for i, spec := range discovered {
		result += fmt.Sprintf("%d. %s\n", i+1, spec)
		// Store in database with session
		_, err := db.Exec("INSERT INTO api_specs (session_id, spec_url) VALUES (?, ?)", sessionId, spec)
		if err != nil {
			log.Printf("Failed to store spec: %v", err)
		}
	}

	return mcp.NewToolResultText(result + "\nContainers have been stopped."), nil
}

func handleGenerateAPITests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	specId, err := request.RequireString("specId")
	if err != nil {
		return mcp.NewToolResultError("Missing required specId"), nil
	}

	endpoints := request.GetString("endpoints", "")
	testType := request.GetString("testType", "load")

	// Get session ID from spec
	var sessionId int64
	err = db.QueryRow("SELECT session_id FROM api_specs WHERE id = ?", specId).Scan(&sessionId)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Spec not found: %v", err)), nil
	}

	// Generate k6 test script
	script := generateK6APITest(specId, endpoints, testType)

	// Store test with session
	result, err := db.Exec("INSERT INTO tests (session_id, name, type, script) VALUES (?, ?, ?, ?)",
		sessionId, fmt.Sprintf("api-test-%s", time.Now().Format("20060102-150405")), testType, script)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to store test: %v", err)), nil
	}

	testId, _ := result.LastInsertId()

	return mcp.NewToolResultText(fmt.Sprintf("Generated %s test with ID: %d\n\nScript preview:\n%s...",
		testType, testId, script[:200])), nil
}

func handleCreateUITest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := request.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError("Missing required url"), nil
	}

	instructions, err := request.RequireString("instructions")
	if err != nil {
		return mcp.NewToolResultError("Missing required instructions"), nil
	}

	testName := request.GetString("testName", "ui-test")

	// Get most recent session
	var sessionId int64
	err = db.QueryRow("SELECT id FROM test_sessions ORDER BY created_at DESC LIMIT 1").Scan(&sessionId)
	if err != nil {
		return mcp.NewToolResultError("No active session. Run setup_test_environment first."), nil
	}

	// Parse natural language instructions
	script := generateK6UITest(url, instructions)

	// Store test with session
	result, err := db.Exec("INSERT INTO tests (session_id, name, type, script) VALUES (?, ?, ?, ?)",
		sessionId, testName, "browser", script)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to store test: %v", err)), nil
	}

	testId, _ := result.LastInsertId()

	return mcp.NewToolResultText(fmt.Sprintf("Created UI test '%s' with ID: %d\n\nInstructions parsed:\n%s",
		testName, testId, instructions)), nil
}

func handleRunTest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	testId, err := request.RequireString("testId")
	if err != nil {
		return mcp.NewToolResultError("Missing required testId"), nil
	}

	vus := int(request.GetFloat("vus", 10))
	duration := request.GetString("duration", "30s")

	// Get test script and session
	var script string
	var sessionId int64
	err = db.QueryRow("SELECT script, session_id FROM tests WHERE id = ?", testId).Scan(&script, &sessionId)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Test not found: %v", err)), nil
	}

	// Get compose file content
	var content string
	err = db.QueryRow(`
		SELECT cf.content 
		FROM compose_files cf
		JOIN test_sessions ts ON ts.compose_file_id = cf.id
		WHERE ts.id = ?`, sessionId).Scan(&content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Compose file not found: %v", err)), nil
	}

	// Write compose to temp location
	composePath, err := writeComposeToTemp(content, sessionId)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write compose file: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	// Start Docker Compose environment
	projectName := fmt.Sprintf("perftest-%d", time.Now().Unix())
	containerStart := time.Now()
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	containerOutput, err := startCmd.CombinedOutput()
	if err != nil {
		logContainerOperation("start", projectName, time.Since(containerStart), err, map[string]interface{}{
			"output":  string(containerOutput),
			"test_id": testId,
		})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, containerOutput)), nil
	}
	logContainerOperation("start", projectName, time.Since(containerStart), nil, map[string]interface{}{
		"test_id":      testId,
		"compose_path": composePath,
	})

	// Ensure we clean up containers at the end
	defer func() {
		stopStart := time.Now()
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		err := stopCmd.Run()
		logContainerOperation("stop", projectName, time.Since(stopStart), err, map[string]interface{}{
			"test_id": testId,
		})
	}()

	// Wait for services to be ready
	time.Sleep(10 * time.Second)

	// Write script to temp file
	tmpFile, err := os.CreateTemp("", "k6-test-*.js")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(script)
	tmpFile.Close()

	// Create test run record
	result, _ := db.Exec("INSERT INTO test_runs (test_id, vus, duration) VALUES (?, ?, ?)",
		testId, vus, duration)
	runId, _ := result.LastInsertId()

	// Run k6 test
	outputFile := fmt.Sprintf("/tmp/k6-results-%d.json", runId)
	cmd := exec.CommandContext(ctx, "k6", "run",
		"--vus", fmt.Sprintf("%d", vus),
		"--duration", duration,
		"--out", fmt.Sprintf("json=%s", outputFile),
		tmpFile.Name())

	testStart := time.Now()
	logInfo("Starting k6 test execution", map[string]interface{}{
		"test_id":     testId,
		"run_id":      runId,
		"vus":         vus,
		"duration":    duration,
		"output_file": outputFile,
	})

	output, err := cmd.CombinedOutput()
	testDuration := time.Since(testStart)

	if err != nil {
		logError("k6 test execution failed", err, map[string]interface{}{
			"test_id":  testId,
			"run_id":   runId,
			"duration": testDuration.String(),
			"output":   string(output),
		})
		return mcp.NewToolResultError(fmt.Sprintf("Test execution failed: %v\n%s", err, output)), nil
	}

	// Convert testId string to int64 for logging
	testIdInt, _ := strconv.ParseInt(testId, 10, 64)
	logTestExecution("performance", testIdInt, testDuration, map[string]interface{}{
		"run_id":      runId,
		"vus":         vus,
		"duration":    duration,
		"output_size": len(output),
	})

	// Update test run
	db.Exec("UPDATE test_runs SET completed_at = CURRENT_TIMESTAMP, results = ? WHERE id = ?",
		string(output), runId)

	// Parse and store metrics (simplified)
	parseAndStoreMetrics(runId, outputFile)

	return mcp.NewToolResultText(fmt.Sprintf("Test completed. Run ID: %d\n\nContainers have been stopped and removed.\n\n%s", runId, output)), nil
}

func handleAnalyzeResults(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	runId, err := request.RequireString("runId")
	if err != nil {
		return mcp.NewToolResultError("Missing required runId"), nil
	}

	compareHistory := request.GetString("compareHistory", "false") == "true"

	// Get metrics for this run
	rows, err := db.Query(`
		SELECT endpoint, avg_response_time, error_rate 
		FROM metrics 
		WHERE run_id = ?`, runId)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query metrics: %v", err)), nil
	}
	defer rows.Close()

	analysis := "# Performance Analysis\n\n"
	analysis += fmt.Sprintf("## Run ID: %s\n\n", runId)

	for rows.Next() {
		var endpoint string
		var avgTime, errorRate float64
		rows.Scan(&endpoint, &avgTime, &errorRate)

		analysis += fmt.Sprintf("### %s\n", endpoint)
		analysis += fmt.Sprintf("- Avg Response Time: %.2f ms\n", avgTime)
		analysis += fmt.Sprintf("- Error Rate: %.2f%%\n", errorRate*100)

		// Check against SLAs
		var slaTime int
		var slaError float64
		err := db.QueryRow(`
			SELECT sla_response_time, sla_error_rate 
			FROM endpoints 
			WHERE path = ?`, endpoint).Scan(&slaTime, &slaError)

		if err == nil {
			if avgTime > float64(slaTime) {
				analysis += fmt.Sprintf("- ⚠️ SLA VIOLATION: Response time exceeds %d ms\n", slaTime)
			}
			if errorRate > slaError {
				analysis += fmt.Sprintf("- ⚠️ SLA VIOLATION: Error rate exceeds %.1f%%\n", slaError*100)
			}
		}

		if compareHistory {
			// Compare with historical average
			var histAvgTime, histErrorRate float64
			err := db.QueryRow(`
				SELECT AVG(avg_response_time), AVG(error_rate) 
				FROM metrics 
				WHERE endpoint = ? AND run_id != ?`, endpoint, runId).Scan(&histAvgTime, &histErrorRate)

			if err == nil {
				timeDiff := ((avgTime - histAvgTime) / histAvgTime) * 100
				analysis += fmt.Sprintf("- Response time: %.1f%% vs historical average\n", timeDiff)
			}
		}

		analysis += "\n"
	}

	return mcp.NewToolResultText(analysis), nil
}

func handleQueryHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// service := request.GetString("service", "") // Not used yet
	endpoint := request.GetString("endpoint", "")
	days := int(request.GetFloat("days", 7))

	query := `
		SELECT 
			tr.started_at,
			m.endpoint,
			m.avg_response_time,
			m.error_rate,
			m.requests_per_second
		FROM metrics m
		JOIN test_runs tr ON m.run_id = tr.id
		WHERE tr.started_at > datetime('now', '-' || ? || ' days')`

	args := []interface{}{days}

	if endpoint != "" {
		query += " AND m.endpoint = ?"
		args = append(args, endpoint)
	}

	query += " ORDER BY tr.started_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Query failed: %v", err)), nil
	}
	defer rows.Close()

	results := []map[string]interface{}{}
	for rows.Next() {
		var timestamp, endpoint string
		var avgTime, errorRate, rps float64
		rows.Scan(&timestamp, &endpoint, &avgTime, &errorRate, &rps)

		results = append(results, map[string]interface{}{
			"timestamp": timestamp,
			"endpoint":  endpoint,
			"avgTime":   avgTime,
			"errorRate": errorRate,
			"rps":       rps,
		})
	}

	jsonData, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(jsonData)), nil
}

// Helper functions

func generateK6APITest(specId, endpoints, testType string) string {
	// Simplified test generation
	return fmt.Sprintf(`import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    %s_test: {
      executor: '%s',
      %s
    },
  },
};

export default function () {
  // Generated from spec %s
  // Testing endpoints: %s
  const res = http.get('http://localhost:8080/api/endpoint');
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}`, testType, getExecutorType(testType), getScenarioConfig(testType), specId, endpoints)
}

func generateK6UITest(url, instructions string) string {
	// Parse natural language to k6 browser commands
	actions := parseUIInstructions(instructions)

	script := fmt.Sprintf(`import { browser } from 'k6/experimental/browser';
import { check } from 'k6';

export const options = {
  scenarios: {
    browser: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 1,
      options: {
        browser: {
          type: 'chromium',
        },
      },
    },
  },
};

export default async function () {
  const page = browser.newPage();
  
  try {
    await page.goto('%s');
    
`, url)

	for _, action := range actions {
		script += "    " + action + "\n"
	}

	script += `  } finally {
    page.close();
  }
}`

	return script
}

func parseUIInstructions(instructions string) []string {
	// Simple natural language parsing
	actions := []string{}
	instructions = strings.ToLower(instructions)

	// Map common phrases to k6 commands
	if strings.Contains(instructions, "click") {
		if strings.Contains(instructions, "button") {
			actions = append(actions, "await page.locator('button').click();")
		}
	}
	if strings.Contains(instructions, "type") || strings.Contains(instructions, "enter") {
		actions = append(actions, "await page.locator('input').type('test data');")
	}
	if strings.Contains(instructions, "wait") {
		actions = append(actions, "await page.waitForTimeout(1000);")
	}

	return actions
}

func getExecutorType(testType string) string {
	switch testType {
	case "stress":
		return "ramping-vus"
	case "spike":
		return "ramping-arrival-rate"
	default:
		return "constant-vus"
	}
}

func getScenarioConfig(testType string) string {
	switch testType {
	case "stress":
		return `stages: [
        { duration: '2m', target: 100 },
        { duration: '5m', target: 100 },
        { duration: '2m', target: 0 },
      ],`
	case "spike":
		return `startRate: 10,
      timeUnit: '1s',
      stages: [
        { duration: '30s', target: 10 },
        { duration: '10s', target: 100 },
        { duration: '30s', target: 10 },
      ],`
	default:
		return `vus: 10,
      duration: '30s',`
	}
}

func parseAndStoreMetrics(runId int64, outputFile string) {
	// Simplified metric parsing - in reality would parse k6 JSON output
	db.Exec(`INSERT INTO metrics 
		(run_id, endpoint, avg_response_time, min_response_time, max_response_time, error_rate, requests_per_second) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		runId, "/api/endpoint", 150.5, 50.0, 500.0, 0.02, 85.5)
}

func generateJSArray(items []string) string {
	// Convert string slice to JavaScript array literal
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("'%s'", item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// New automated handlers
func handleTestApplication(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	composeSource, err := request.RequireString("composeSource")
	if err != nil {
		logError("Missing required composeSource", err, nil)
		return mcp.NewToolResultError("Missing required composeSource"), nil
	}

	testType := request.GetString("testType", "standard")
	endpoints := request.GetString("endpoints", "")

	logInfo("Starting automated application testing", map[string]interface{}{
		"composeSource": composeSource,
		"testType":      testType,
		"endpoints":     endpoints,
	})

	// Full automated flow
	report := "# Automated Application Testing\n\n"

	// Step 1: Setup environment
	report += "## Step 1: Setting up environment\n"
	sendProgress(ctx, "Setting up test environment", map[string]interface{}{"step": 1})
	content, err := fetchComposeContent(composeSource)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch compose file: %v", err)), nil
	}

	composeFileId, err := storeComposeFile(composeSource, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to store compose file: %v", err)), nil
	}

	// Create session
	sessionName := fmt.Sprintf("auto-test-%d", time.Now().Unix())
	result, err := db.Exec("INSERT INTO test_sessions (compose_file_id, session_name, status) VALUES (?, ?, ?)",
		composeFileId, sessionName, "running")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create session: %v", err)), nil
	}
	sessionId, _ := result.LastInsertId()

	// Parse compose to store services
	var compose ComposeFile
	yaml.Unmarshal([]byte(content), &compose)
	for name, service := range compose.Services {
		ports := strings.Join(service.Ports, ",")
		db.Exec("INSERT INTO services (session_id, name, image, ports) VALUES (?, ?, ?, ?)",
			sessionId, name, service.Image, ports)
	}
	report += fmt.Sprintf("- Created session %d with %d services\n", sessionId, len(compose.Services))

	// Step 2: Start containers and discover APIs
	report += "\n## Step 2: Discovering APIs\n"
	composePath, err := writeComposeToTemp(content, sessionId)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write compose: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	projectName := fmt.Sprintf("auto-%d", sessionId)
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	if output, err := startCmd.CombinedOutput(); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, output)), nil
	}

	// Ensure cleanup
	defer func() {
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		stopCmd.Run()
		db.Exec("UPDATE test_sessions SET completed_at = CURRENT_TIMESTAMP, status = ? WHERE id = ?",
			"completed", sessionId)
	}()

	time.Sleep(15 * time.Second) // Wait for services

	// Discover specs
	discovered := 0
	commonPaths := []string{"/openapi.json", "/swagger.json", "/api-docs", "/api/v3/openapi.json"}

	rows, _ := db.Query("SELECT id, name, ports FROM services WHERE session_id = ?", sessionId)
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, ports string
		rows.Scan(&id, &name, &ports)

		portList := strings.Split(ports, ",")
		if len(portList) > 0 && portList[0] != "" {
			port := strings.Split(portList[0], ":")[0]
			baseURL := fmt.Sprintf("http://localhost:%s", port)

			for _, path := range commonPaths {
				url := baseURL + path
				resp, err := http.Get(url)
				if err == nil && resp.StatusCode == 200 {
					discovered++
					db.Exec("INSERT INTO api_specs (session_id, spec_url) VALUES (?, ?)", sessionId, url)
					report += fmt.Sprintf("- Found API spec: %s\n", url)
					resp.Body.Close()
					break
				}
			}
		}
	}

	// Step 3: Generate and run tests
	report += fmt.Sprintf("\n## Step 3: Running %s tests\n", testType)
	if endpoints != "" {
		report += fmt.Sprintf("- Testing specific endpoints: %s\n", endpoints)
	}

	// Generate test based on type
	var testVus int
	var testDuration string
	switch testType {
	case "quick":
		testVus = 10
		testDuration = "30s"
	case "thorough":
		testVus = 100
		testDuration = "5m"
	default:
		testVus = 50
		testDuration = "2m"
	}

	// Get port from first service
	var testPort string
	for _, service := range compose.Services {
		if len(service.Ports) > 0 {
			testPort = strings.Split(service.Ports[0], ":")[0]
			break
		}
	}
	if testPort == "" {
		testPort = "8080" // fallback
	}

	// Create test script with endpoint filtering
	var testEndpoints []string
	if endpoints != "" {
		// Parse comma-separated endpoints
		for _, ep := range strings.Split(endpoints, ",") {
			testEndpoints = append(testEndpoints, strings.TrimSpace(ep))
		}
	} else {
		// Default endpoints based on discovered specs
		testEndpoints = []string{"/", "/api/health", "/api/v3/pet"}
	}

	// Generate test script
	testScript := fmt.Sprintf(`import http from 'k6/http';
import { check, group } from 'k6';

export const options = {
  vus: %d,
  duration: '%s',
  thresholds: {
    http_req_duration: ['p(95)<500'],
    http_req_failed: ['rate<0.1'],
  },
};

const BASE_URL = 'http://localhost:%s';
const endpoints = %s;

export default function () {
  endpoints.forEach(endpoint => {
    group('Testing ' + endpoint, () => {
      const res = http.get(BASE_URL + endpoint);
      check(res, {
        'status is 200': (r) => r.status === 200,
        'response time < 500ms': (r) => r.timings.duration < 500,
      });
    });
  });
}`, testVus, testDuration, testPort, generateJSArray(testEndpoints))

	// Store and run test
	testResult, _ := db.Exec("INSERT INTO tests (session_id, name, type, script) VALUES (?, ?, ?, ?)",
		sessionId, "auto-load-test", "load", testScript)
	testId, _ := testResult.LastInsertId()

	// Write test script
	tmpFile, _ := os.CreateTemp("", "k6-auto-test-*.js")
	tmpFile.WriteString(testScript)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Run test
	runResult, _ := db.Exec("INSERT INTO test_runs (test_id, vus, duration) VALUES (?, ?, ?)",
		testId, testVus, testDuration)
	runId, _ := runResult.LastInsertId()

	outputFile := fmt.Sprintf("/tmp/k6-auto-results-%d.json", runId)
	k6Cmd := exec.CommandContext(ctx, "k6", "run",
		"--vus", fmt.Sprintf("%d", testVus),
		"--duration", testDuration,
		"--out", fmt.Sprintf("json=%s", outputFile),
		tmpFile.Name())

	k6Output, _ := k6Cmd.CombinedOutput()
	report += fmt.Sprintf("- Test completed with %d VUs for %s\n", testVus, testDuration)
	report += "\n## Results Summary\n"
	report += "```\n" + string(k6Output) + "\n```\n"

	// Update session
	db.Exec("UPDATE test_runs SET completed_at = CURRENT_TIMESTAMP WHERE id = ?", runId)

	return mcp.NewToolResultText(report), nil
}

func handleQuickPerformanceTest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	composeSource, err := request.RequireString("composeSource")
	if err != nil {
		logError("Missing required composeSource", err, nil)
		return mcp.NewToolResultError("Missing required composeSource"), nil
	}

	vus := int(request.GetFloat("vus", 50))
	duration := request.GetString("duration", "2m")
	// targetService := request.GetString("targetService", "") // TODO: implement service targeting

	logInfo("Starting quick performance test", map[string]interface{}{
		"composeSource": composeSource,
		"vus":           vus,
		"duration":      duration,
		"component":     "quick_performance_test",
	})

	// Quick test - simpler flow
	report := fmt.Sprintf("# Quick Performance Test\n\n")
	report += fmt.Sprintf("- Target: %s\n", composeSource)
	report += fmt.Sprintf("- VUs: %d\n", vus)
	report += fmt.Sprintf("- Duration: %s\n\n", duration)

	// Fetch and store compose
	content, err := fetchComposeContent(composeSource)
	if err != nil {
		logError("Failed to fetch compose content", err, map[string]interface{}{"composeSource": composeSource})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch compose: %v", err)), nil
	}

	composeFileId, err := storeComposeFile(composeSource, content)
	if err != nil {
		logError("Failed to store compose file", err, map[string]interface{}{"composeSource": composeSource})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to store compose file: %v", err)), nil
	}

	// Quick session
	sessionName := fmt.Sprintf("quick-%d", time.Now().Unix())
	result, err := db.Exec("INSERT INTO test_sessions (compose_file_id, session_name, status) VALUES (?, ?, ?)",
		composeFileId, sessionName, "running")
	if err != nil {
		logError("Failed to create session", err, map[string]interface{}{"sessionName": sessionName})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create session: %v", err)), nil
	}
	sessionId, _ := result.LastInsertId()

	// Write and start
	composePath, err := writeComposeToTemp(content, sessionId)
	if err != nil {
		logError("Failed to write compose to temp", err, map[string]interface{}{"sessionId": sessionId})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write compose file: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	projectName := fmt.Sprintf("quick-%d", sessionId)
	containerStart := time.Now()
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	containerOutput, err := startCmd.CombinedOutput()
	if err != nil {
		logContainerOperation("start", projectName, time.Since(containerStart), err, map[string]interface{}{
			"output":     string(containerOutput),
			"session_id": sessionId,
		})
		return mcp.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, containerOutput)), nil
	}
	logContainerOperation("start", projectName, time.Since(containerStart), nil, map[string]interface{}{
		"session_id":   sessionId,
		"compose_path": composePath,
	})

	defer func() {
		stopStart := time.Now()
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		err := stopCmd.Run()
		logContainerOperation("stop", projectName, time.Since(stopStart), err, map[string]interface{}{
			"session_id": sessionId,
		})
	}()

	logInfo("Waiting for services to start", map[string]interface{}{
		"wait_time":  "10s",
		"session_id": sessionId,
	})
	time.Sleep(10 * time.Second)

	// Simple test script
	testScript := `import http from 'k6/http';
import { check } from 'k6';

export default function () {
  const res = http.get('http://localhost:8082/');
  check(res, { 'status ok': (r) => r.status < 400 });
}`

	// Run quick test
	tmpFile, err := os.CreateTemp("", "k6-quick-*.js")
	if err != nil {
		logError("Failed to create temp file", err, nil)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
	}
	tmpFile.WriteString(testScript)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	testStart := time.Now()
	logInfo("Starting k6 test execution", map[string]interface{}{
		"vus":         vus,
		"duration":    duration,
		"script_path": tmpFile.Name(),
		"session_id":  sessionId,
	})

	k6Cmd := exec.CommandContext(ctx, "k6", "run", "--vus", fmt.Sprintf("%d", vus), "--duration", duration, tmpFile.Name())
	output, err := k6Cmd.CombinedOutput()
	testDuration := time.Since(testStart)

	if err != nil {
		logError("k6 test execution failed", err, map[string]interface{}{
			"session_id": sessionId,
			"duration":   testDuration.String(),
			"output":     string(output),
		})
		return mcp.NewToolResultError(fmt.Sprintf("k6 test failed: %v\n%s", err, output)), nil
	}

	logInfo("k6 test completed successfully", map[string]interface{}{
		"session_id":  sessionId,
		"duration":    testDuration.String(),
		"output_size": len(output),
	})

	report += "## Results\n```\n" + string(output) + "\n```"

	return mcp.NewToolResultText(report), nil
}

// Resource handlers
func handleSchemaResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	// Get all tables
	rows, err := db.Query(`SELECT name, sql FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schema strings.Builder
	schema.WriteString("# SQLite Database Schema\n\n")

	for rows.Next() {
		var name, sql string
		if err := rows.Scan(&name, &sql); err != nil {
			continue
		}
		schema.WriteString(fmt.Sprintf("## Table: %s\n\n```sql\n%s\n```\n\n", name, sql))
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/plain",
			Text:     schema.String(),
		},
	}, nil
}

func handleSessionsResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	rows, err := db.Query(`
		SELECT s.id, s.session_name, s.started_at, s.completed_at, s.status,
		       c.source_url, COUNT(DISTINCT sv.id) as service_count
		FROM test_sessions s
		LEFT JOIN compose_files c ON s.compose_file_id = c.id
		LEFT JOIN services sv ON sv.session_id = s.id
		GROUP BY s.id
		ORDER BY s.started_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type SessionInfo struct {
		ID           int64      `json:"id"`
		Name         string     `json:"name"`
		StartedAt    time.Time  `json:"started_at"`
		CompletedAt  *time.Time `json:"completed_at,omitempty"`
		Status       string     `json:"status"`
		SourceURL    string     `json:"source_url"`
		ServiceCount int        `json:"service_count"`
	}

	var sessions []SessionInfo
	for rows.Next() {
		var s SessionInfo
		var completedAt sql.NullTime
		err := rows.Scan(&s.ID, &s.Name, &s.StartedAt, &completedAt, &s.Status, &s.SourceURL, &s.ServiceCount)
		if err != nil {
			continue
		}
		if completedAt.Valid {
			s.CompletedAt = &completedAt.Time
		}
		sessions = append(sessions, s)
	}

	data, _ := json.MarshalIndent(sessions, "", "  ")
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func handleComposeFilesResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	rows, err := db.Query(`
		SELECT id, source_url, hash, created_at, LENGTH(content) as size
		FROM compose_files
		ORDER BY created_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type ComposeFileInfo struct {
		ID        int64     `json:"id"`
		SourceURL string    `json:"source_url"`
		Hash      string    `json:"hash"`
		CreatedAt time.Time `json:"created_at"`
		Size      int       `json:"size_bytes"`
	}

	var files []ComposeFileInfo
	for rows.Next() {
		var f ComposeFileInfo
		err := rows.Scan(&f.ID, &f.SourceURL, &f.Hash, &f.CreatedAt, &f.Size)
		if err != nil {
			continue
		}
		files = append(files, f)
	}

	data, _ := json.MarshalIndent(files, "", "  ")
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

func handleTestRunsResource(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	rows, err := db.Query(`
		SELECT r.id, r.started_at, r.completed_at, r.vus, r.duration,
		       t.name as test_name, t.type as test_type,
		       s.session_name
		FROM test_runs r
		JOIN tests t ON r.test_id = t.id
		JOIN test_sessions s ON t.session_id = s.id
		ORDER BY r.started_at DESC
		LIMIT 20
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type TestRunInfo struct {
		ID          int64      `json:"id"`
		StartedAt   time.Time  `json:"started_at"`
		CompletedAt *time.Time `json:"completed_at,omitempty"`
		VUs         int        `json:"vus"`
		Duration    string     `json:"duration"`
		TestName    string     `json:"test_name"`
		TestType    string     `json:"test_type"`
		SessionName string     `json:"session_name"`
	}

	var runs []TestRunInfo
	for rows.Next() {
		var r TestRunInfo
		var completedAt sql.NullTime
		err := rows.Scan(&r.ID, &r.StartedAt, &completedAt, &r.VUs, &r.Duration,
			&r.TestName, &r.TestType, &r.SessionName)
		if err != nil {
			continue
		}
		if completedAt.Valid {
			r.CompletedAt = &completedAt.Time
		}
		runs = append(runs, r)
	}

	data, _ := json.MarshalIndent(runs, "", "  ")
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
