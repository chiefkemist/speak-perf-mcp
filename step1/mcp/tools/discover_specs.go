package tools

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
)

// DiscoverSpecsTool handles the discover_api_specs tool
type DiscoverSpecsTool struct {
	deps *SharedDependencies
}

// NewDiscoverSpecsTool creates a new instance of DiscoverSpecsTool
func NewDiscoverSpecsTool(deps *SharedDependencies) *DiscoverSpecsTool {
	return &DiscoverSpecsTool{deps: deps}
}

// Handle processes the discover_api_specs request
func (t *DiscoverSpecsTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	specPaths := request.GetString("specPaths", "")
	autoDiscover := request.GetString("autoDiscover", "true") == "true"

	// Get the most recent session
	var sessionId int64
	var composeFileId int64
	err := t.deps.DB.QueryRow(`
		SELECT id, compose_file_id 
		FROM test_sessions 
		ORDER BY created_at DESC 
		LIMIT 1`).Scan(&sessionId, &composeFileId)
	if err != nil {
		return mcpgolang.NewToolResultError("No environment configured. Run setup_test_environment first."), nil
	}

	// Get compose content
	var content string
	err = t.deps.DB.QueryRow("SELECT content FROM compose_files WHERE id = ?", composeFileId).Scan(&content)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to get compose file: %v", err)), nil
	}

	// Write to temp location
	composePath, err := WriteComposeToTemp(content, sessionId)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to write compose file: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	// Start containers temporarily for discovery
	projectName := fmt.Sprintf("discover-%d", sessionId)
	containerStart := time.Now()
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	output, err := startCmd.CombinedOutput()
	if err != nil {
		t.deps.Logger.LogContainerOperation("start", projectName, time.Since(containerStart), err, map[string]interface{}{
			"output":     string(output),
			"session_id": sessionId,
		})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, output)), nil
	}
	t.deps.Logger.LogContainerOperation("start", projectName, time.Since(containerStart), nil, map[string]interface{}{
		"session_id":   sessionId,
		"compose_path": composePath,
	})

	// Ensure cleanup
	defer func() {
		stopStart := time.Now()
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		err := stopCmd.Run()
		t.deps.Logger.LogContainerOperation("stop", projectName, time.Since(stopStart), err, map[string]interface{}{
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
		rows, err := t.deps.DB.Query("SELECT id, name, ports FROM services WHERE session_id = ?", sessionId)
		if err != nil {
			return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to query services: %v", err)), nil
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
		_, err := t.deps.DB.Exec("INSERT INTO api_specs (session_id, spec_url) VALUES (?, ?)", sessionId, spec)
		if err != nil {
			log.Printf("Failed to store spec: %v", err)
		}
	}

	return mcpgolang.NewToolResultText(result + "\nContainers have been stopped."), nil
}

