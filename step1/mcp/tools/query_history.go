package tools

import (
	"context"
	"encoding/json"
	"fmt"

	mcpgolang "github.com/mark3labs/mcp-go/mcp"
)

// QueryHistoryTool handles the query_test_history tool
type QueryHistoryTool struct {
	deps *SharedDependencies
}

// NewQueryHistoryTool creates a new instance of QueryHistoryTool
func NewQueryHistoryTool(deps *SharedDependencies) *QueryHistoryTool {
	return &QueryHistoryTool{deps: deps}
}

// Handle processes the query_test_history request
func (t *QueryHistoryTool) Handle(ctx context.Context, request mcpgolang.CallToolRequest) (*mcpgolang.CallToolResult, error) {
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

	rows, err := t.deps.DB.Query(query, args...)
	if err != nil {
		return mcpgolang.NewToolResultError(fmt.Sprintf("Query failed: %v", err)), nil
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
	return mcpgolang.NewToolResultText(string(jsonData)), nil
}

