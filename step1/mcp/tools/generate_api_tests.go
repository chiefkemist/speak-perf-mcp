package tools

import (
	"context"
	"fmt"
	"time"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
)

// GenerateAPITestsTool handles the generate_api_tests tool
type GenerateAPITestsTool struct {
	deps *SharedDependencies
}

// NewGenerateAPITestsTool creates a new instance of GenerateAPITestsTool
func NewGenerateAPITestsTool(deps *SharedDependencies) *GenerateAPITestsTool {
	return &GenerateAPITestsTool{deps: deps}
}

// Handle processes the generate_api_tests request
func (t *GenerateAPITestsTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	specId, err := request.RequireString("specId")
	if err != nil {
		return mcpgolang.NewToolResultError("Missing required specId"), nil
	}

	endpoints := request.GetString("endpoints", "")
	testType := request.GetString("testType", "load")

	// Get session ID from spec
	var sessionId int64
	err = t.deps.DB.QueryRow("SELECT session_id FROM api_specs WHERE id = ?", specId).Scan(&sessionId)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Spec not found: %v", err)), nil
	}

	// Generate k6 test script
	script := t.generateK6APITest(specId, endpoints, testType)

	// Store test with session
	result, err := t.deps.DB.Exec("INSERT INTO tests (session_id, name, type, script) VALUES (?, ?, ?, ?)",
		sessionId, fmt.Sprintf("api-test-%s", time.Now().Format("20060102-150405")), testType, script)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to store test: %v", err)), nil
	}

	testId, _ := result.LastInsertId()

	return mcpgolang.NewToolResultText(fmt.Sprintf("Generated %s test with ID: %d\n\nScript preview:\n%s...",
		testType, testId, script[:200])), nil
}

func (t *GenerateAPITestsTool) generateK6APITest(specId, endpoints, testType string) string {
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
}`, testType, GetExecutorType(testType), GetScenarioConfig(testType), specId, endpoints)
}

