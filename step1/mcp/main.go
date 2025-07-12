package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chiefkemist/speak-perf/step1/mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "github.com/mattn/go-sqlite3"
)

var (
	db        *sql.DB
	mcpServer *server.MCPServer
)

func init() {
	// Initialize logging
	InitializeLogging()
}

func sendProgress(ctx context.Context, progress string, data map[string]interface{}) {
	if mcpServer != nil {
		// Log the progress
		LogInfo("Progress update", map[string]interface{}{
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
		LogDebug("Progress notification prepared", progressData)
	}
}

func main() {
	startTime := time.Now()

	// Initialize database
	LogInfo("Initializing database", nil)
	dbStart := time.Now()
	initDB()
	LogPerformanceMetrics("database_init", time.Since(dbStart), nil)
	defer func() {
		if err := db.Close(); err != nil {
			LogError("Failed to close database", err, nil)
		} else {
			LogInfo("Database closed successfully", nil)
		}
	}()

	LogInfo("MCP Server Step1 starting", map[string]interface{}{
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
	LogPerformanceMetrics("mcp_server_init", time.Since(serverStart), nil)

	// Register tools with enhanced logging
	registerTools(s)

	// Add resources with enhanced logging
	registerResources(s)

	// Start server
	LogInfo("Starting stdio server", map[string]interface{}{
		"startup_duration": time.Since(startTime).String(),
	})

	if err := server.ServeStdio(s); err != nil {
		LogFatal("Failed to start stdio server", err, nil)
		log.Fatal(err)
	}
}

func registerTools(s *server.MCPServer) {
	LogInfo("Registering MCP tools", nil)

	// Create shared dependencies with logger adapter
	deps := &tools.SharedDependencies{
		DB:     db,
		Logger: &LoggerAdapter{},
	}

	// Create tool instances
	setupTool := tools.NewSetupEnvironmentTool(deps)
	discoverTool := tools.NewDiscoverSpecsTool(deps)
	generateAPITool := tools.NewGenerateAPITestsTool(deps)
	createUITool := tools.NewCreateUITestTool(deps)
	runPerfTool := tools.NewRunPerformanceTestTool(deps)
	analyzeTool := tools.NewAnalyzeResultsTool(deps)
	queryTool := tools.NewQueryHistoryTool(deps)
	testAppTool := tools.NewTestApplicationTool(deps)
	quickTestTool := tools.NewQuickPerformanceTestTool(deps)

	// Register tools
	s.AddTool(mcp.NewTool(
		"setup_test_environment",
		mcp.WithDescription("Initialize testing environment from Docker Compose file"),
		mcp.WithString("composePath", mcp.Required(), mcp.Description("Path to docker-compose.yml")),
		mcp.WithString("projectName", mcp.Description("Project name for containers")),
	), enhanceToolHandler("setup_test_environment", setupTool.Handle))

	s.AddTool(mcp.NewTool(
		"discover_api_specs",
		mcp.WithDescription("Find and parse OpenAPI/Swagger specifications"),
		mcp.WithString("specPaths", mcp.Description("Comma-separated paths to API specs")),
		mcp.WithString("autoDiscover", mcp.Description("Auto-discover specs from running services (true/false)")),
	), enhanceToolHandler("discover_api_specs", discoverTool.Handle))

	s.AddTool(mcp.NewTool(
		"generate_api_tests",
		mcp.WithDescription("Generate k6 tests from API specifications"),
		mcp.WithString("specId", mcp.Required(), mcp.Description("ID of discovered spec")),
		mcp.WithString("endpoints", mcp.Description("Comma-separated endpoints to test")),
		mcp.WithString("testType", mcp.Description("Test type: load, stress, spike")),
	), enhanceToolHandler("generate_api_tests", generateAPITool.Handle))

	s.AddTool(mcp.NewTool(
		"create_ui_test",
		mcp.WithDescription("Generate k6 browser test from natural language"),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL")),
		mcp.WithString("instructions", mcp.Required(), mcp.Description("Natural language test instructions")),
		mcp.WithString("testName", mcp.Description("Name for the test")),
	), enhanceToolHandler("create_ui_test", createUITool.Handle))

	s.AddTool(mcp.NewTool(
		"run_performance_test",
		mcp.WithDescription("Execute generated performance tests"),
		mcp.WithString("testId", mcp.Required(), mcp.Description("ID of test to run")),
		mcp.WithNumber("vus", mcp.Description("Virtual users")),
		mcp.WithString("duration", mcp.Description("Test duration")),
	), enhanceToolHandler("run_performance_test", runPerfTool.Handle))

	s.AddTool(mcp.NewTool(
		"analyze_results",
		mcp.WithDescription("Analyze test results against SLAs"),
		mcp.WithString("runId", mcp.Required(), mcp.Description("Test run ID")),
		mcp.WithString("compareHistory", mcp.Description("Compare with historical data (true/false)")),
	), enhanceToolHandler("analyze_results", analyzeTool.Handle))

	s.AddTool(mcp.NewTool(
		"query_test_history",
		mcp.WithDescription("Query historical test data"),
		mcp.WithString("service", mcp.Description("Filter by service name")),
		mcp.WithString("endpoint", mcp.Description("Filter by endpoint")),
		mcp.WithNumber("days", mcp.Description("Number of days to look back")),
	), enhanceToolHandler("query_test_history", queryTool.Handle))

	// Add automated tools
	s.AddTool(mcp.NewTool(
		"test_application",
		mcp.WithDescription("Complete automated testing of a Docker Compose application"),
		mcp.WithString("composeSource", mcp.Required(), mcp.Description("Path or URL to docker-compose.yml")),
		mcp.WithString("testType", mcp.Description("Test type: quick, standard, thorough (default: standard)")),
		mcp.WithString("endpoints", mcp.Description("Specific endpoints to test (comma-separated)")),
	), enhanceToolHandler("test_application", testAppTool.Handle))

	s.AddTool(mcp.NewTool(
		"quick_performance_test",
		mcp.WithDescription("Run quick performance test with custom parameters"),
		mcp.WithString("composeSource", mcp.Required(), mcp.Description("Path or URL to docker-compose.yml")),
		mcp.WithNumber("vus", mcp.Description("Virtual users (default: 50)")),
		mcp.WithString("duration", mcp.Description("Test duration (default: 2m)")),
		mcp.WithString("targetService", mcp.Description("Specific service to test")),
	), enhanceToolHandler("quick_performance_test", quickTestTool.Handle))

	LogInfo("MCP tools registered successfully", map[string]interface{}{
		"tool_count": 9,
	})
}

func registerResources(s *server.MCPServer) {
	LogInfo("Registering MCP resources", nil)

	// Add resources
	s.AddResource(mcp.NewResource("sqlite://schema", "Database Schema",
		mcp.WithResourceDescription("View the SQLite database schema")), handleSchemaResource)
	s.AddResource(mcp.NewResource("sqlite://sessions", "Test Sessions",
		mcp.WithResourceDescription("List recent test sessions")), handleSessionsResource)
	s.AddResource(mcp.NewResource("sqlite://compose-files", "Compose Files",
		mcp.WithResourceDescription("List stored Docker Compose files")), handleComposeFilesResource)
	s.AddResource(mcp.NewResource("sqlite://test-runs", "Test Runs",
		mcp.WithResourceDescription("List recent performance test runs")), handleTestRunsResource)

	LogInfo("MCP resources registered successfully", map[string]interface{}{
		"resource_count": 4,
	})
}

// enhanceToolHandler wraps tool handlers with comprehensive logging
func enhanceToolHandler(toolName string, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		requestID := GenerateRequestID()
		startTime := time.Now()

		// Extract parameters for logging - using a simpler approach
		params := make(map[string]interface{})
		// Note: We can't easily access request.Params.Arguments due to interface{} type
		// So we'll log the tool name and let individual handlers log their specific params

		LogToolStart(toolName, requestID, params)

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

		LogToolEnd(toolName, requestID, duration, success, logData)

		return result, err
	}
}

func initDB() {
	start := time.Now()
	var err error

	LogInfo("Opening SQLite database", map[string]interface{}{
		"database_path": "./perf_test.db",
	})

	db, err = sql.Open("sqlite3", "./perf_test.db")
	if err != nil {
		LogFatal("Failed to open database", err, nil)
		log.Fatal(err)
	}

	LogDatabaseOperation("open", time.Since(start), err, map[string]interface{}{
		"database_path": "./perf_test.db",
	})

	// Test connection
	start = time.Now()
	if err := db.Ping(); err != nil {
		LogFatal("Failed to ping database", err, nil)
		log.Fatal(err)
	}
	LogDatabaseOperation("ping", time.Since(start), nil, nil)

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
		LogFatal("Failed to create database schema", err, nil)
		log.Fatal(err)
	}

	LogDatabaseOperation("create_schema", time.Since(start), nil, map[string]interface{}{
		"tables_created": 8,
	})

	LogInfo("Database initialized successfully", map[string]interface{}{
		"total_duration": time.Since(start).String(),
	})
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