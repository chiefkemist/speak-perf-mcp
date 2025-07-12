# Speak Performance MCP - k6 Testing Server

An MCP (Model Context Protocol) server that provides AI-driven performance testing capabilities using k6.

## Overview

This project is organized in progressive steps to demonstrate building an AI-driven performance testing system:

### Step 0 - Foundation
- **MCP Server**: A Go-based MCP server that exposes k6 performance testing tools
- **Web API**: A sample web application for testing
- **k6 Scripts**: Example performance test scripts

## MCP Tools Available

### 1. execute_k6_test
Execute a k6 performance test with a custom script.
- **script** (required): Path to k6 test script
- **vus**: Virtual users (default: 10)
- **duration**: Test duration (default: 30s)

### 2. run_load_test
Run a load test with constant request rate.
- **url** (required): Target URL to test
- **rps**: Requests per second (default: 100)
- **duration**: Test duration (default: 60s)
- **method**: HTTP method (default: GET)
- **payload**: Request payload for POST/PUT

### 3. run_stress_test
Run a stress test to find the breaking point.
- **url** (required): Target URL to test
- **startVus**: Starting virtual users (default: 1)
- **maxVus**: Maximum virtual users (default: 100)
- **rampDuration**: Duration to ramp up users (default: 5m)

### 4. generate_report
Generate a performance test report from k6 results.
- **resultFile** (required): Path to k6 results JSON file
- **format**: Report format - html, json, markdown (default: markdown)

## Setup

### Prerequisites
- Go 1.20+
- k6 installed (`brew install k6` on macOS)

### Running the MCP Server
```bash
cd step0/mcp
go run main.go
```

### Running the Test Web Server
```bash
cd step0/web
go run main.go
```

### Using Air for Development
```bash
# From the root directory
air
```
This will build and run the current step's servers with hot reload
The web server will start on http://localhost:8080

### Available API Endpoints
- `GET /api/health` - Health check endpoint
- `GET /api/users` - Get all users
- `POST /api/users` - Create a new user
- `GET /api/data` - Get random data (with simulated delays)
- `GET /` - Simple web UI for browser testing

## Example k6 Scripts

The `k6-scripts` directory contains example test scripts:

1. **basic-test.js** - Simple load test for the health endpoint
2. **api-test.js** - Comprehensive API testing with multiple endpoints
3. **spike-test.js** - Spike test to test sudden load increases
4. **browser-test.js** - Browser-based testing with k6 browser API

### Running Tests Manually

```bash
# Basic test
k6 run k6-scripts/basic-test.js

# API test with custom base URL
k6 run -e BASE_URL=http://localhost:8080 k6-scripts/api-test.js

# Spike test
k6 run k6-scripts/spike-test.js
```

## Usage with Claude

When using this MCP server with Claude, you can:

1. Execute existing test scripts:
   ```
   "Please run the basic k6 test script"
   ```

2. Run load tests on specific endpoints:
   ```
   "Run a load test on http://localhost:8080/api/users with 200 requests per second for 2 minutes"
   ```

3. Perform stress testing:
   ```
   "Run a stress test on the API starting with 10 users and ramping up to 500"
   ```

4. Generate test reports:
   ```
   "Generate a report from the stress test results"
   ```

## Project Structure

### Step 0: Foundation
Basic MCP server with k6 integration:
- Static test target (web server)
- Dynamic script generation for load/stress tests
- Basic report generation from k6 output
- Manual test execution

### Step 1: Dynamic Container Testing
Advanced testing with Docker Compose integration:
- Accepts any Docker Compose file
- Automatic OpenAPI/Swagger discovery
- Natural language UI test generation
- SQLite database for historical tracking
- SLA monitoring and comparison

## Architecture

The MCP server acts as a bridge between Claude and k6, allowing natural language commands to be translated into performance tests. The server:
- Generates k6 scripts dynamically based on parameters
- Executes tests using the k6 CLI
- Returns results in a readable format
- Supports various testing patterns (load, stress, spike, browser)

## Logging and Monitoring

The MCP servers write detailed JSON logs to help with debugging and monitoring:

### Log Location
- Default: `~/.speak-perf-mcp/logs/`
- Override with `MCP_LOG_DIR` environment variable
- When using Makefile: `./logs/`

### Viewing Logs
```bash
# List available log files
make logs

# Tail the latest log file
make tail-log

# Tail a specific log file
tail -f logs/mcp-server-2024-01-15-10-30-45.log
```

### Log Format
Logs are in JSON format with the following fields:
- `level`: INFO or ERROR
- `timestamp`: RFC3339 nano format
- `message`: Log message
- Additional context fields as needed

### Progress Reporting
The MCP server logs progress updates for long-running operations like:
- Test execution status
- Script generation
- Result parsing
- Environment setup

These progress updates are intended to be forwarded to MCP clients in the future.

## Development

To extend the MCP server with new tools:
1. Add the tool definition in `main.go`
2. Create a handler function
3. Implement the test logic (generate script, execute, parse results)

## Performance Tips

- Start with small loads and gradually increase
- Monitor server resources during tests
- Use thresholds to define success criteria
- Consider network latency in distributed setups
- Use k6 cloud for larger scale tests