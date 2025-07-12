package tools

import (
	"context"
	"fmt"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
)

// AnalyzeResultsTool handles the analyze_results tool
type AnalyzeResultsTool struct {
	deps *SharedDependencies
}

// NewAnalyzeResultsTool creates a new instance of AnalyzeResultsTool
func NewAnalyzeResultsTool(deps *SharedDependencies) *AnalyzeResultsTool {
	return &AnalyzeResultsTool{deps: deps}
}

// Handle processes the analyze_results request
func (t *AnalyzeResultsTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
	runId, err := request.RequireString("runId")
	if err != nil {
		return mcpgolang.NewToolResultError("Missing required runId"), nil
	}

	compareHistory := request.GetString("compareHistory", "false") == "true"

	// Get metrics for this run
	rows, err := t.deps.DB.Query(`
		SELECT endpoint, avg_response_time, error_rate 
		FROM metrics 
		WHERE run_id = ?`, runId)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Failed to query metrics: %v", err)), nil
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
		err := t.deps.DB.QueryRow(`
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
			err := t.deps.DB.QueryRow(`
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

	return mcpgolang.NewToolResultText(analysis), nil
}

