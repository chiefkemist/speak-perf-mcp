package tools

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
	"gopkg.in/yaml.v3"
)

// TestApplicationTool handles the test_application tool
type TestApplicationTool struct {
	deps *SharedDependencies
}

// NewTestApplicationTool creates a new instance of TestApplicationTool
func NewTestApplicationTool(deps *SharedDependencies) *TestApplicationTool {
	return &TestApplicationTool{deps: deps}
}

// Handle processes the test_application request
func (t *TestApplicationTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	composeSource, err := request.RequireString("composeSource")
	if err != nil {
		t.deps.Logger.LogError("Missing required composeSource", err, nil)
		return mcpgolang.NewToolResultError("Missing required composeSource"), nil
	}

	testType := request.GetString("testType", "standard")
	endpoints := request.GetString("endpoints", "")

	t.deps.Logger.LogInfo("Starting automated application testing", map[string]interface{}{
		"composeSource": composeSource,
		"testType":      testType,
		"endpoints":     endpoints,
	})

	// Full automated flow
	report := "# Automated Application Testing\n\n"

	// Step 1: Setup environment
	report += "## Step 1: Setting up environment\n"
	t.sendProgress(ctx, "Setting up test environment", map[string]interface{}{"step": 1})
	content, err := FetchComposeContent(composeSource)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to fetch compose file: %v", err)), nil
	}

	composeFileId, err := StoreComposeFile(t.deps.DB, composeSource, content)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to store compose file: %v", err)), nil
	}

	// Create session
	sessionName := fmt.Sprintf("auto-test-%d", time.Now().Unix())
	result, err := t.deps.DB.Exec("INSERT INTO test_sessions (compose_file_id, session_name, status) VALUES (?, ?, ?)",
		composeFileId, sessionName, "running")
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to create session: %v", err)), nil
	}
	sessionId, _ := result.LastInsertId()

	// Parse compose to store services
	var compose ComposeFile
	yaml.Unmarshal([]byte(content), &compose)
	for name, service := range compose.Services {
		ports := strings.Join(service.Ports, ",")
		t.deps.DB.Exec("INSERT INTO services (session_id, name, image, ports) VALUES (?, ?, ?, ?)",
			sessionId, name, service.Image, ports)
	}
	report += fmt.Sprintf("- Created session %d with %d services\n", sessionId, len(compose.Services))

	// Step 2: Start containers and discover APIs
	report += "\n## Step 2: Discovering APIs\n"
	composePath, err := WriteComposeToTemp(content, sessionId)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to write compose: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	projectName := fmt.Sprintf("auto-%d", sessionId)
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	if output, err := startCmd.CombinedOutput(); err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, output)), nil
	}

	// Ensure cleanup
	defer func() {
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		stopCmd.Run()
		t.deps.DB.Exec("UPDATE test_sessions SET completed_at = CURRENT_TIMESTAMP, status = ? WHERE id = ?",
			"completed", sessionId)
	}()

	time.Sleep(15 * time.Second) // Wait for services

	// Discover specs
	discovered := 0
	commonPaths := []string{"/openapi.json", "/swagger.json", "/api-docs", "/api/v3/openapi.json"}

	rows, _ := t.deps.DB.Query("SELECT id, name, ports FROM services WHERE session_id = ?", sessionId)
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
					t.deps.DB.Exec("INSERT INTO api_specs (session_id, spec_url) VALUES (?, ?)", sessionId, url)
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
}`, testVus, testDuration, testPort, GenerateJSArray(testEndpoints))

	// Store and run test
	testResult, _ := t.deps.DB.Exec("INSERT INTO tests (session_id, name, type, script) VALUES (?, ?, ?, ?)",
		sessionId, "auto-load-test", "load", testScript)
	testId, _ := testResult.LastInsertId()

	// Write test script
	tmpFile, _ := os.CreateTemp("", "k6-auto-test-*.js")
	tmpFile.WriteString(testScript)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Run test
	runResult, _ := t.deps.DB.Exec("INSERT INTO test_runs (test_id, vus, duration) VALUES (?, ?, ?)",
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
	t.deps.DB.Exec("UPDATE test_runs SET completed_at = CURRENT_TIMESTAMP WHERE id = ?", runId)

	return mcpgolang.NewToolResultText(report), nil
}

func (t *TestApplicationTool) sendProgress(ctx context.Context, progress string, data map[string]interface{}) {
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

