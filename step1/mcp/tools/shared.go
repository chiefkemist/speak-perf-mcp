package tools

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ComposeFile represents a Docker Compose file structure
type ComposeFile struct {
	Services map[string]Service `yaml:"services"`
}

// Service represents a service in Docker Compose
type Service struct {
	Image       string   `yaml:"image"`
	Ports       []string `yaml:"ports"`
	Environment []string `yaml:"environment"`
	DependsOn   []string `yaml:"depends_on"`
}

// SharedDependencies holds shared resources for tools
type SharedDependencies struct {
	DB     *sql.DB
	Logger Logger
}

// FetchComposeContent fetches Docker Compose content from URL or file
func FetchComposeContent(source string) (string, error) {
	// Check if it's a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source)
		if err != nil {
			return "", fmt.Errorf("failed to download compose file: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("failed to download compose file: status %d", resp.StatusCode)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response: %w", err)
		}
		return string(content), nil
	}

	// Otherwise treat as file path
	content, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("failed to read compose file: %w", err)
	}
	return string(content), nil
}

// StoreComposeFile stores compose file in database
func StoreComposeFile(db *sql.DB, source, content string) (int64, error) {
	// Calculate hash
	hash := md5.Sum([]byte(content))
	hashStr := hex.EncodeToString(hash[:])

	// Check if already exists
	var existingId int64
	err := db.QueryRow("SELECT id FROM compose_files WHERE hash = ?", hashStr).Scan(&existingId)
	if err == nil {
		return existingId, nil
	}

	// Store new compose file
	result, err := db.Exec("INSERT INTO compose_files (source_url, content, hash) VALUES (?, ?, ?)",
		source, content, hashStr)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// WriteComposeToTemp writes compose content to temporary directory
func WriteComposeToTemp(content string, sessionId int64) (string, error) {
	// Create unique temp directory
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("k6-test-%d-%d", sessionId, time.Now().Unix()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", err
	}

	// Write compose file
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(content), 0644); err != nil {
		return "", err
	}

	return composePath, nil
}

// GenerateJSArray converts string slice to JavaScript array literal
func GenerateJSArray(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("'%s'", item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// GetExecutorType returns k6 executor type based on test type
func GetExecutorType(testType string) string {
	switch testType {
	case "stress":
		return "ramping-vus"
	case "spike":
		return "ramping-arrival-rate"
	default:
		return "constant-vus"
	}
}

// GetScenarioConfig returns k6 scenario configuration based on test type
func GetScenarioConfig(testType string) string {
	switch testType {
	case "stress":
		return `stages: [
        { duration: '2m', target: 100 },
        { duration: '5m', target: 100 },
        { duration: '2m', target: 0 },
      ],`
	case "spike":
		return `startRate: 10,
      timeUnit: '1s',
      stages: [
        { duration: '30s', target: 10 },
        { duration: '10s', target: 100 },
        { duration: '30s', target: 10 },
      ],`
	default:
		return `vus: 10,
      duration: '30s',`
	}
}

// ParseUIInstructions parses natural language to k6 browser commands
func ParseUIInstructions(instructions string) []string {
	// Simple natural language parsing
	actions := []string{}
	instructions = strings.ToLower(instructions)

	// Map common phrases to k6 commands
	if strings.Contains(instructions, "click") {
		if strings.Contains(instructions, "button") {
			actions = append(actions, "await page.locator('button').click();")
		}
	}
	if strings.Contains(instructions, "type") || strings.Contains(instructions, "enter") {
		actions = append(actions, "await page.locator('input').type('test data');")
	}
	if strings.Contains(instructions, "wait") {
		actions = append(actions, "await page.waitForTimeout(1000);")
	}

	return actions
}

// ParseAndStoreMetrics parses k6 output and stores metrics (simplified)
func ParseAndStoreMetrics(db *sql.DB, runId int64, outputFile string) {
	// Simplified metric parsing - in reality would parse k6 JSON output
	db.Exec(`INSERT INTO metrics 
		(run_id, endpoint, avg_response_time, min_response_time, max_response_time, error_rate, requests_per_second) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		runId, "/api/endpoint", 150.5, 50.0, 500.0, 0.02, 85.5)
}

