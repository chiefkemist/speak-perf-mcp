package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// SetupEnvironmentTool handles the setup_test_environment tool
type SetupEnvironmentTool struct {
	deps *SharedDependencies
}

// NewSetupEnvironmentTool creates a new instance of SetupEnvironmentTool
func NewSetupEnvironmentTool(deps *SharedDependencies) *SetupEnvironmentTool {
	return &SetupEnvironmentTool{deps: deps}
}

// Handle processes the setup_test_environment request
func (t *SetupEnvironmentTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	composePath, err := request.RequireString("composePath")
	if err != nil {
		t.deps.Logger.LogError("Missing required composePath", err, nil)
		return mcpgolang.NewToolResultError("Missing required composePath"), nil
	}

	t.deps.Logger.LogInfo("Setting up test environment", map[string]interface{}{
		"composePath": composePath,
		"component":   "setup_environment",
	})
	t.sendProgress(ctx, "Fetching compose file", map[string]interface{}{"composePath": composePath})

	// Fetch compose content
	content, err := FetchComposeContent(composePath)
	if err != nil {
		t.deps.Logger.LogError("Failed to fetch compose content", err, map[string]interface{}{"composePath": composePath})
		return mcpgolang.NewToolResultError(err.Error()), nil
	}

	// Parse to validate
	var compose ComposeFile
	if err := yaml.Unmarshal([]byte(content), &compose); err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Invalid compose file: %v", err)), nil
	}

	// Store in database
	dbStart := time.Now()
	composeFileId, err := StoreComposeFile(t.deps.DB, composePath, content)
	if err != nil {
		t.deps.Logger.LogError("Failed to store compose file", err, map[string]interface{}{"composePath": composePath})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to store compose file: %v", err)), nil
	}
	t.deps.Logger.LogDatabaseOperation("store_compose_file", time.Since(dbStart), nil, map[string]interface{}{
		"compose_file_id": composeFileId,
		"source":          composePath,
	})

	// Create test session
	sessionName := fmt.Sprintf("session-%d", time.Now().Unix())
	dbStart = time.Now()
	result, err := t.deps.DB.Exec("INSERT INTO test_sessions (compose_file_id, session_name, status) VALUES (?, ?, ?)",
		composeFileId, sessionName, "initialized")
	if err != nil {
		t.deps.Logger.LogError("Failed to create session", err, map[string]interface{}{"sessionName": sessionName})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to create session: %v", err)), nil
	}
	sessionId, _ := result.LastInsertId()
	t.deps.Logger.LogDatabaseOperation("create_session", time.Since(dbStart), nil, map[string]interface{}{
		"session_id":   sessionId,
		"session_name": sessionName,
	})

	// Store services metadata
	servicesStored := 0
	for name, service := range compose.Services {
		ports := strings.Join(service.Ports, ",")
		dbStart = time.Now()
		_, err := t.deps.DB.Exec("INSERT INTO services (session_id, name, image, ports) VALUES (?, ?, ?, ?)",
			sessionId, name, service.Image, ports)
		if err != nil {
			t.deps.Logger.LogError("Failed to store service", err, map[string]interface{}{
				"service_name": name,
				"session_id":   sessionId,
			})
		} else {
			servicesStored++
			t.deps.Logger.LogDatabaseOperation("store_service", time.Since(dbStart), nil, map[string]interface{}{
				"service_name": name,
				"session_id":   sessionId,
				"image":        service.Image,
			})
		}
	}

	t.deps.Logger.LogInfo("Services stored successfully", map[string]interface{}{
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

	return mcpgolang.NewToolResultText(response), nil
}

func (t *SetupEnvironmentTool) sendProgress(ctx context.Context, progress string, data map[string]interface{}) {
	// Log the progress
	t.deps.Logger.LogInfo("Progress update", map[string]interface{}{
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
	t.deps.Logger.LogDebug("Progress notification prepared", progressData)
}

