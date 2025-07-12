package tools

import (
	"context"
	"database/sql"
	"fmt"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
)

// CreateUITestTool handles the create_ui_test tool
type CreateUITestTool struct {
	deps *SharedDependencies
}

// NewCreateUITestTool creates a new instance of CreateUITestTool
func NewCreateUITestTool(deps *SharedDependencies) *CreateUITestTool {
	return &CreateUITestTool{deps: deps}
}

// Handle processes the create_ui_test request
func (t *CreateUITestTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	url, err := request.RequireString("url")
	if err != nil {
		return mcpgolang.NewToolResultError("Missing required url"), nil
	}

	instructions, err := request.RequireString("instructions")
	if err != nil {
		return mcpgolang.NewToolResultError("Missing required instructions"), nil
	}

	testName := request.GetString("testName", "ui-test")

	// Get most recent session
	var sessionId int64
	err = t.deps.DB.QueryRow("SELECT id FROM test_sessions ORDER BY created_at DESC LIMIT 1").Scan(&sessionId)
	if err != nil {
		return mcpgolang.NewToolResultError("No active session. Run setup_test_environment first."), nil
	}

	// Parse natural language instructions
	script := t.generateK6UITest(url, instructions)

	// Store test with session
	result, err := t.deps.DB.Exec("INSERT INTO tests (session_id, name, type, script) VALUES (?, ?, ?, ?)",
		sessionId, testName, "browser", script)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to store test: %v", err)), nil
	}

	testId, _ := result.LastInsertId()

	return mcpgolang.NewToolResultText(fmt.Sprintf("Created UI test '%s' with ID: %d\n\nInstructions parsed:\n%s",
		testName, testId, instructions)), nil
}

func (t *CreateUITestTool) generateK6UITest(url, instructions string) string {
	// Parse natural language to k6 browser commands
	actions := ParseUIInstructions(instructions)

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
