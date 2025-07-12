package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Create mcp server
	s := server.NewMCPServer(
		"k6-mcp", "1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithRecovery(),
		server.WithLogging(),
	)

	// Add tools to control infra and k6
	tool := mcp.NewTool(
		"execute_k6_test",
		mcp.WithDescription("Execute k6 performance test"),
		mcp.WithString("script", mcp.Required(), mcp.Description("Path to k6 test script")),
		mcp.WithNumber("vus", mcp.Description("Virtual users")),
		mcp.WithString("duration", mcp.Description("Test duration")),
	)

	s.AddTool(tool, handleK6Test)

	// Add load test tool
	loadTool := mcp.NewTool(
		"run_load_test",
		mcp.WithDescription("Run a load test with custom parameters"),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL to test")),
		mcp.WithNumber("rps", mcp.Description("Requests per second")),
		mcp.WithString("duration", mcp.Description("Test duration")),
		mcp.WithString("method", mcp.Description("HTTP method (GET, POST, etc.)")),
		mcp.WithString("payload", mcp.Description("Request payload for POST/PUT")),
	)
	s.AddTool(loadTool, handleLoadTest)

	// Add stress test tool
	stressTool := mcp.NewTool(
		"run_stress_test",
		mcp.WithDescription("Run a stress test to find breaking point"),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL to test")),
		mcp.WithNumber("startVus", mcp.Description("Starting virtual users")),
		mcp.WithNumber("maxVus", mcp.Description("Maximum virtual users")),
		mcp.WithString("rampDuration", mcp.Description("Duration to ramp up users")),
	)
	s.AddTool(stressTool, handleStressTest)

	// Add performance report tool
	reportTool := mcp.NewTool(
		"generate_report",
		mcp.WithDescription("Generate performance test report"),
		mcp.WithString("resultFile", mcp.Required(), mcp.Description("Path to k6 results JSON file")),
		mcp.WithString("format", mcp.Description("Report format (html, json, markdown)")),
	)
	s.AddTool(reportTool, handleGenerateReport)

	// Start server with stdio transport
	if err := server.ServeStdio(s); err != nil {
		log.Fatal(err)
	}
}

func handleK6Test(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	script, err := request.RequireString("script")
	if err != nil {
		return mcp.NewToolResultError("Missing required script parameter"), nil
	}

	// Get optional parameters
	vus := request.GetInt("vus", 10)
	duration := request.GetString("duration", "30s")

	// Execute k6 test with JSON output
	resultFile := fmt.Sprintf("/tmp/k6-results-%d.json", time.Now().Unix())
	defer os.Remove(resultFile)
	
	result, err := executeK6TestWithJSON(ctx, script, vus, duration, resultFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Test execution failed: %+v", err)), nil
	}
	
	// Parse and format results
	report := parseK6Results(resultFile)
	return mcp.NewToolResultText(result + "\n\n" + report), nil
}


func handleLoadTest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := request.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError("Missing required url parameter"), nil
	}

	// Get optional parameters
	rps := request.GetFloat("rps", 100.0)
	duration := request.GetString("duration", "60s")
	method := request.GetString("method", "GET")
	payload := request.GetString("payload", "")

	// Create a temporary k6 script
	script := generateLoadTestScript(url, rps, duration, method, payload)
	
	// Write script to temp file
	tmpFile, err := os.CreateTemp("", "k6-load-test-*.js")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write script: %v", err)), nil
	}
	tmpFile.Close()

	// Execute the test with JSON output
	resultFile := fmt.Sprintf("/tmp/k6-load-results-%d.json", time.Now().Unix())
	defer os.Remove(resultFile)
	
	result, err := executeK6TestWithJSON(ctx, tmpFile.Name(), 10, duration, resultFile)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Load test failed: %v", err)), nil
	}
	
	// Parse and format results
	report := parseK6Results(resultFile)
	return mcp.NewToolResultText(result + "\n\n" + report), nil
}

func handleStressTest(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	url, err := request.RequireString("url")
	if err != nil {
		return mcp.NewToolResultError("Missing required url parameter"), nil
	}

	// Get optional parameters
	startVus := request.GetInt("startVus", 1)
	maxVus := request.GetInt("maxVus", 100)
	rampDuration := request.GetString("rampDuration", "5m")

	// Create stress test script
	script := generateStressTestScript(url, startVus, maxVus, rampDuration)
	
	// Write script to temp file
	tmpFile, err := os.CreateTemp("", "k6-stress-test-*.js")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create temp file: %v", err)), nil
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write script: %v", err)), nil
	}
	tmpFile.Close()

	// Execute with stages and JSON output
	resultFile := fmt.Sprintf("/tmp/k6-stress-results-%d.json", time.Now().Unix())
	defer os.Remove(resultFile)
	
	args := []string{"run", "--out", fmt.Sprintf("json=%s", resultFile), tmpFile.Name()}
	cmd := exec.CommandContext(ctx, "k6", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n\nErrors:\n" + stderr.String()
	}

	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Stress test failed: %v\n%s", err, output)), nil
	}
	
	// Parse and format results
	report := parseK6Results(resultFile)
	return mcp.NewToolResultText(output + "\n\n" + report), nil
}

func handleGenerateReport(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resultFile, err := request.RequireString("resultFile")
	if err != nil {
		return mcp.NewToolResultError("Missing required resultFile parameter"), nil
	}

	format := request.GetString("format", "markdown")

	// Check if file exists
	if _, err := os.Stat(resultFile); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Result file not found: %s", resultFile)), nil
	}

	// Generate report based on format
	var report string
	switch format {
	case "json":
		report = generateJSONReport(resultFile)
	case "html":
		report = generateHTMLReport(resultFile)
	default:
		report = parseK6Results(resultFile)
	}

	return mcp.NewToolResultText(report), nil
}

func generateLoadTestScript(url string, rps float64, duration string, method string, payload string) string {
	script := fmt.Sprintf(`import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  scenarios: {
    constant_request_rate: {
      executor: 'constant-arrival-rate',
      rate: %d,
      timeUnit: '1s',
      duration: '%s',
      preAllocatedVUs: 10,
      maxVUs: 100,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<500'],
    errors: ['rate<0.1'],
  },
};

export default function () {
  const params = {
    headers: { 'Content-Type': 'application/json' },
  };
  
`, int(rps), duration)

	if method == "GET" {
		script += fmt.Sprintf(`  const res = http.get('%s', params);`, url)
	} else if payload != "" {
		script += fmt.Sprintf(`  const res = http.%s('%s', '%s', params);`, strings.ToLower(method), url, payload)
	} else {
		script += fmt.Sprintf(`  const res = http.%s('%s', null, params);`, strings.ToLower(method), url)
	}

	script += `
  
  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 500ms': (r) => r.timings.duration < 500,
  });
  
  errorRate.add(!success);
}
`
	return script
}

func generateStressTestScript(url string, _ int, maxVus int, rampDuration string) string {
	return fmt.Sprintf(`import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '%s', target: %d },
    { duration: '10s', target: %d },
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    http_req_failed: ['rate<0.5'],
  },
};

export default function () {
  const res = http.get('%s');
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
  sleep(1);
}
`, rampDuration, maxVus, maxVus, url)
}

func executeK6TestWithJSON(ctx context.Context, scriptPath string, vus int, duration string, outputFile string) (string, error) {
	// Check if script exists
	if _, err := os.Stat(scriptPath); err != nil {
		return "", fmt.Errorf("script not found: %s", scriptPath)
	}

	// Build k6 command with JSON output
	args := []string{"run"}
	args = append(args, "--vus", strconv.Itoa(vus))
	args = append(args, "--duration", duration)
	args = append(args, "--out", fmt.Sprintf("json=%s", outputFile))
	args = append(args, scriptPath)

	cmd := exec.CommandContext(ctx, "k6", args...)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()

	// Combine outputs
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n\nErrors:\n" + stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("k6 execution failed: %w", err)
	}

	return output, nil
}

// K6Metric represents a k6 metric data point
type K6Metric struct {
	Type   string                 `json:"type"`
	Data   map[string]interface{} `json:"data"`
	Metric string                 `json:"metric"`
}

func parseK6Results(resultFile string) string {
	// Read the JSON file
	data, err := os.ReadFile(resultFile)
	if err != nil {
		return fmt.Sprintf("Error reading results: %v", err)
	}

	// Parse metrics
	metrics := make(map[string]struct {
		count   int
		total   float64
		min     float64
		max     float64
		failed  int
		passed  int
	})

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var metric K6Metric
		if err := json.Unmarshal([]byte(line), &metric); err != nil {
			continue
		}

		if metric.Type == "Point" {
			metricName := metric.Metric
			m := metrics[metricName]
			m.count++

			if value, ok := metric.Data["value"].(float64); ok {
				m.total += value
				if m.count == 1 || value < m.min {
					m.min = value
				}
				if value > m.max {
					m.max = value
				}
			}

			// Check for passed/failed
			if tags, ok := metric.Data["tags"].(map[string]interface{}); ok {
				if status, ok := tags["status"].(string); ok {
					if status == "200" {
						m.passed++
					} else {
						m.failed++
					}
				}
			}

			metrics[metricName] = m
		}
	}

	// Generate report
	report := "# K6 Performance Test Results\n\n"
	report += "## Key Metrics\n\n"

	// HTTP Duration
	if m, ok := metrics["http_req_duration"]; ok && m.count > 0 {
		avg := m.total / float64(m.count)
		report += fmt.Sprintf("### Response Time\n")
		report += fmt.Sprintf("- Average: %.2f ms\n", avg)
		report += fmt.Sprintf("- Min: %.2f ms\n", m.min)
		report += fmt.Sprintf("- Max: %.2f ms\n", m.max)
		report += fmt.Sprintf("- Requests: %d\n\n", m.count)
	}

	// HTTP Failures
	if m, ok := metrics["http_req_failed"]; ok && m.count > 0 {
		failRate := float64(m.failed) / float64(m.count) * 100
		report += fmt.Sprintf("### Success Rate\n")
		report += fmt.Sprintf("- Total Requests: %d\n", m.count)
		report += fmt.Sprintf("- Failed: %d (%.1f%%)\n", m.failed, failRate)
		report += fmt.Sprintf("- Passed: %d (%.1f%%)\n\n", m.passed, 100-failRate)
	}

	// Data Transfer
	if m, ok := metrics["data_received"]; ok && m.count > 0 {
		totalMB := m.total / 1024 / 1024
		report += fmt.Sprintf("### Data Transfer\n")
		report += fmt.Sprintf("- Total Received: %.2f MB\n", totalMB)
	}

	if m, ok := metrics["data_sent"]; ok && m.count > 0 {
		totalMB := m.total / 1024 / 1024
		report += fmt.Sprintf("- Total Sent: %.2f MB\n\n", totalMB)
	}

	// VUs
	if m, ok := metrics["vus"]; ok && m.count > 0 {
		report += fmt.Sprintf("### Virtual Users\n")
		report += fmt.Sprintf("- Max VUs: %.0f\n\n", m.max)
	}

	return report
}

func generateJSONReport(resultFile string) string {
	// For JSON format, return a summary of parsed metrics
	data, err := os.ReadFile(resultFile)
	if err != nil {
		return fmt.Sprintf(`{"error": "Failed to read file: %v"}`, err)
	}

	// Simple aggregation for JSON output
	summary := map[string]interface{}{
		"file": resultFile,
		"timestamp": time.Now().Format(time.RFC3339),
		"lines": len(strings.Split(string(data), "\n")) - 1,
	}

	jsonData, _ := json.MarshalIndent(summary, "", "  ")
	return string(jsonData)
}

func generateHTMLReport(resultFile string) string {
	markdownReport := parseK6Results(resultFile)
	
	// Simple HTML wrapper
	html := `<!DOCTYPE html>
<html>
<head>
    <title>K6 Performance Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        h1 { color: #333; }
        h2 { color: #666; }
        h3 { color: #999; }
        ul { list-style-type: none; }
        li { margin: 5px 0; }
    </style>
</head>
<body>
`
	
	// Convert markdown to basic HTML
	lines := strings.Split(markdownReport, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			html += fmt.Sprintf("<h1>%s</h1>\n", strings.TrimPrefix(line, "# "))
		} else if strings.HasPrefix(line, "## ") {
			html += fmt.Sprintf("<h2>%s</h2>\n", strings.TrimPrefix(line, "## "))
		} else if strings.HasPrefix(line, "### ") {
			html += fmt.Sprintf("<h3>%s</h3>\n", strings.TrimPrefix(line, "### "))
		} else if strings.HasPrefix(line, "- ") {
			html += fmt.Sprintf("<li>%s</li>\n", strings.TrimPrefix(line, "- "))
		} else if line == "" {
			html += "<br>\n"
		} else {
			html += fmt.Sprintf("<p>%s</p>\n", line)
		}
	}
	
	html += `</body>
</html>`
	return html
}
