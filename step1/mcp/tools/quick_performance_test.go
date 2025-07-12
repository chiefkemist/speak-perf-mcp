package tools

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
)

// QuickPerformanceTestTool handles the quick_performance_test tool
type QuickPerformanceTestTool struct {
	deps *SharedDependencies
}

// NewQuickPerformanceTestTool creates a new instance of QuickPerformanceTestTool
func NewQuickPerformanceTestTool(deps *SharedDependencies) *QuickPerformanceTestTool {
	return &QuickPerformanceTestTool{deps: deps}
}

// Handle processes the quick_performance_test request
func (t *QuickPerformanceTestTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	composeSource, err := request.RequireString("composeSource")
	if err != nil {
		t.deps.Logger.LogError("Missing required composeSource", err, nil)
		return mcpgolang.NewToolResultError("Missing required composeSource"), nil
	}

	vus := int(request.GetFloat("vus", 50))
	duration := request.GetString("duration", "2m")
	// targetService := request.GetString("targetService", "") // TODO: implement service targeting

	t.deps.Logger.LogInfo("Starting quick performance test", map[string]interface{}{
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
	content, err := FetchComposeContent(composeSource)
	if err != nil {
		t.deps.Logger.LogError("Failed to fetch compose content", err, map[string]interface{}{"composeSource": composeSource})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to fetch compose: %v", err)), nil
	}

	composeFileId, err := StoreComposeFile(t.deps.DB, composeSource, content)
	if err != nil {
		t.deps.Logger.LogError("Failed to store compose file", err, map[string]interface{}{"composeSource": composeSource})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to store compose file: %v", err)), nil
	}

	// Quick session
	sessionName := fmt.Sprintf("quick-%d", time.Now().Unix())
	result, err := t.deps.DB.Exec("INSERT INTO test_sessions (compose_file_id, session_name, status) VALUES (?, ?, ?)",
		composeFileId, sessionName, "running")
	if err != nil {
		t.deps.Logger.LogError("Failed to create session", err, map[string]interface{}{"sessionName": sessionName})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to create session: %v", err)), nil
	}
	sessionId, _ := result.LastInsertId()

	// Write and start
	composePath, err := WriteComposeToTemp(content, sessionId)
	if err != nil {
		t.deps.Logger.LogError("Failed to write compose to temp", err, map[string]interface{}{"sessionId": sessionId})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to write compose file: %v", err)), nil
	}
	defer os.RemoveAll(filepath.Dir(composePath))

	projectName := fmt.Sprintf("quick-%d", sessionId)
	containerStart := time.Now()
	startCmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", projectName, "up", "-d")
	containerOutput, err := startCmd.CombinedOutput()
	if err != nil {
		t.deps.Logger.LogContainerOperation("start", projectName, time.Since(containerStart), err, map[string]interface{}{
			"output":     string(containerOutput),
			"session_id": sessionId,
		})
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to start containers: %v\n%s", err, containerOutput)), nil
	}
	t.deps.Logger.LogContainerOperation("start", projectName, time.Since(containerStart), nil, map[string]interface{}{
		"session_id":   sessionId,
		"compose_path": composePath,
	})

	defer func() {
		stopStart := time.Now()
		stopCmd := exec.Command("docker", "compose", "-f", composePath, "-p", projectName, "down", "-v")
		err := stopCmd.Run()
		t.deps.Logger.LogContainerOperation("stop", projectName, time.Since(stopStart), err, map[string]interface{}{
			"session_id": sessionId,
		})
	}()

	t.deps.Logger.LogInfo("Waiting for services to start", map[string]interface{}{
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
		t.deps.Logger.LogError("Failed to create temp file", err, nil)
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
	}
	tmpFile.WriteString(testScript)
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	testStart := time.Now()
	t.deps.Logger.LogInfo("Starting k6 test execution", map[string]interface{}{
		"vus":         vus,
		"duration":    duration,
		"script_path": tmpFile.Name(),
		"session_id":  sessionId,
	})

	k6Cmd := exec.CommandContext(ctx, "k6", "run", "--vus", fmt.Sprintf("%d", vus), "--duration", duration, tmpFile.Name())
	output, err := k6Cmd.CombinedOutput()
	testDuration := time.Since(testStart)

	if err != nil {
		t.deps.Logger.LogError("k6 test execution failed", err, map[string]interface{}{
			"session_id": sessionId,
			"duration":   testDuration.String(),
			"output":     string(output),
		})
		return mcpgolang.NewToolResultError(fmt.Sprintf("k6 test failed: %v\n%s", err, output)), nil
	}

	t.deps.Logger.LogInfo("k6 test completed successfully", map[string]interface{}{
		"session_id":  sessionId,
		"duration":    testDuration.String(),
		"output_size": len(output),
	})

	report += "## Results\n```\n" + string(output) + "\n```"

	return mcpgolang.NewToolResultText(report), nil
}
