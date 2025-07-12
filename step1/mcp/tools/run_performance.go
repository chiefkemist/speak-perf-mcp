package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
)

// RunPerformanceTestTool handles the run_performance_test tool
type RunPerformanceTestTool struct {
	deps *SharedDependencies
}

// NewRunPerformanceTestTool creates a new instance of RunPerformanceTestTool
func NewRunPerformanceTestTool(deps *SharedDependencies) *RunPerformanceTestTool {
	return &RunPerformanceTestTool{deps: deps}
}

// Handle processes the run_performance_test request
func (t *RunPerformanceTestTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	testId, err := request.RequireString("testId")
	if err != nil {
		return mcpgolang.NewToolResultError("Missing required testId"), nil
	}

	vus := int(request.GetFloat("vus", 10))
	duration := request.GetString("duration", "30s")

	// Get test script and session
	var script string
	var sessionId int64
	err = t.deps.DB.QueryRow("SELECT script, session_id FROM tests WHERE id = ?", testId).Scan(&script, &sessionId)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Test not found: %v", err)), nil
	}

	// Get compose file content
	var content string
	err = t.deps.DB.QueryRow(`
		SELECT cf.content 
		FROM compose_files cf
		JOIN test_sessions ts ON ts.compose_file_id = cf.id
		WHERE ts.id = ?`, sessionId).Scan(&content)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Compose file not found: %v", err)), nil
	}

	// Write compose to temp location
	composePath, err := WriteComposeToTemp(content, sessionId)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to write compose file: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	// Start Docker Compose environment
	projectName := fmt.Sprintf("perftest-%d", time.Now().Unix())
	containerStart := time.Now()
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	containerOutput, err := startCmd.CombinedOutput()
	if err != nil {
		t.deps.Logger.LogContainerOperation("start", projectName, time.Since(containerStart), err, map[string]interface{}{
			"output":  string(containerOutput),
			"test_id": testId,
		})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, containerOutput)), nil
	}
	t.deps.Logger.LogContainerOperation("start", projectName, time.Since(containerStart), nil, map[string]interface{}{
		"test_id":      testId,
		"compose_path": composePath,
	})

	// Ensure we clean up containers at the end
	defer func() {
		stopStart := time.Now()
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		err := stopCmd.Run()
		t.deps.Logger.LogContainerOperation("stop", projectName, time.Since(stopStart), err, map[string]interface{}{
			"test_id": testId,
		})
	}()

	// Wait for services to be ready
	time.Sleep(10 * time.Second)

	// Write script to temp file
	tmpFile, err := os.CreateTemp("", "k6-test-*.js")
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(script)
	tmpFile.Close()

	// Create test run record
	result, _ := t.deps.DB.Exec("INSERT INTO test_runs (test_id, vus, duration) VALUES (?, ?, ?)",
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
	t.deps.Logger.LogInfo("Starting k6 test execution", map[string]interface{}{
		"test_id":     testId,
		"run_id":      runId,
		"vus":         vus,
		"duration":    duration,
		"output_file": outputFile,
	})

	output, err := cmd.CombinedOutput()
	testDuration := time.Since(testStart)

	if err != nil {
		t.deps.Logger.LogError("k6 test execution failed", err, map[string]interface{}{
			"test_id":  testId,
			"run_id":   runId,
			"duration": testDuration.String(),
			"output":   string(output),
		})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Test execution failed: %v\n%s", err, output)), nil
	}

	// Convert testId string to int64 for logging
	testIdInt, _ := strconv.ParseInt(testId, 10, 64)
	t.deps.Logger.LogTestExecution("performance", testIdInt, testDuration, map[string]interface{}{
		"run_id":      runId,
		"vus":         vus,
		"duration":    duration,
		"output_size": len(output),
	})

	// Update test run
	t.deps.DB.Exec("UPDATE test_runs SET completed_at = CURRENT_TIMESTAMP, results = ? WHERE id = ?",
		string(output), runId)

	// Parse and store metrics (simplified)
	ParseAndStoreMetrics(t.deps.DB, runId, outputFile)

	return mcpgolang.NewToolResultText(fmt.Sprintf("Test completed. Run ID: %d\n\nContainers have been stopped and removed.\n\n%s", runId, output)), nil
}
