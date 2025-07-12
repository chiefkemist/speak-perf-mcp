# Step 0: Foundation - K6 Performance Testing MCP Server

## Overview

Step 0 establishes the foundation for AI-driven performance testing. This step provides:
- A basic MCP server that can execute k6 performance tests
- Dynamic k6 script generation for common test patterns
- A sample web application with realistic API endpoints for testing

## Architecture

```mermaid
graph TB
    subgraph "Claude AI"
        A[Natural Language Request]
    end
    
    subgraph "MCP Server"
        B[MCP Protocol Handler]
        C[Tool Registry]
        D[K6 Script Generator]
        E[Command Executor]
    end
    
    subgraph "Testing Infrastructure"
        F[k6 Engine]
        G[Test Scripts]
        H[Results Collector]
    end
    
    subgraph "Target Application"
        I[Web Server :8080]
        J[API Endpoints]
        K[Static Pages]
    end
    
    A -->|MCP Protocol| B
    B --> C
    C -->|Tool Selection| D
    D --> E
    E -->|Execute| F
    F -->|HTTP Requests| I
    I --> J
    I --> K
    F --> H
    H -->|Results| B
    B -->|Response| A
```

## Capabilities

### 1. **Execute K6 Test** (`execute_k6_test`)
Run custom k6 scripts with configurable parameters.

```mermaid
sequenceDiagram
    participant User
    participant MCP
    participant K6
    participant WebApp
    
    User->>MCP: "Run the basic test script"
    MCP->>MCP: Validate script path
    MCP->>K6: Execute script with parameters
    K6->>WebApp: Send HTTP requests
    WebApp-->>K6: Return responses
    K6-->>MCP: Test results & metrics
    MCP-->>User: Formatted report
```

**Parameters:**
- `script` (required): Path to k6 test script
- `vus`: Virtual users (default: 10)
- `duration`: Test duration (default: 30s)

### 2. **Run Load Test** (`run_load_test`)
Generate and execute constant-rate load tests dynamically.

```mermaid
graph LR
    subgraph "Load Test Configuration"
        A[URL Target]
        B[Requests/Second]
        C[Duration]
        D[HTTP Method]
        E[Payload]
    end
    
    subgraph "Generated Script"
        F[Import k6 modules]
        G[Configure scenarios]
        H[Set thresholds]
        I[HTTP requests]
        J[Response checks]
    end
    
    A --> F
    B --> G
    C --> G
    D --> I
    E --> I
    
    style G fill:#f9f,stroke:#333,stroke-width:2px
    style I fill:#bbf,stroke:#333,stroke-width:2px
```

**Features:**
- Constant arrival rate executor
- Automatic VU scaling
- Response time validation
- Error rate monitoring

### 3. **Run Stress Test** (`run_stress_test`)
Find system breaking points through progressive load increase.

```mermaid
graph TD
    subgraph "Stress Test Stages"
        A[Start: 1 VU] -->|Ramp Up| B[Target: Max VUs]
        B -->|Sustain| C[Hold at Peak]
        C -->|Ramp Down| D[End: 0 VUs]
    end
    
    subgraph "Metrics Monitored"
        E[Response Time]
        F[Error Rate]
        G[Throughput]
        H[System Resources]
    end
    
    B --> E
    B --> F
    B --> G
    B --> H
    
    style B fill:#f66,stroke:#333,stroke-width:2px
```

The `generateStressTestScript` function creates a k6 script with:
- Progressive load stages
- Configurable ramp-up duration
- Peak load sustain period
- Graceful ramp-down
- Performance thresholds

### 4. **Generate Report** (`generate_report`)
Transform k6 results into readable reports.

```mermaid
flowchart LR
    A[JSON Results] --> B{Format Type}
    B -->|Markdown| C[MD Report]
    B -->|HTML| D[HTML Report]
    B -->|JSON| E[JSON Summary]
    
    C --> F[Summary Stats]
    C --> G[Threshold Results]
    C --> H[Recommendations]
```

## Step 0 Components

### Web Server (`web/main.go`)
A Go HTTP server running on port 8080 that provides realistic endpoints for testing:

```mermaid
graph TD
    subgraph "API Endpoints"
        A[GET /api/health]
        B[GET /api/users]
        C[POST /api/users]
        D[GET /api/data]
    end
    
    subgraph "Behaviors"
        E[Instant Response]
        F[Variable Latency]
        G[Simulated Errors]
        H[JSON Payloads]
    end
    
    A --> E
    B --> F
    C --> F
    D --> F
    D --> G
    
    style G fill:#faa,stroke:#333,stroke-width:2px
```

### MCP Server (`mcp/main.go`)
The MCP server exposes four tools that can be called via the Model Context Protocol:

1. **execute_k6_test** - Runs existing k6 scripts with JSON output collection
2. **run_load_test** - Generates and runs constant-rate load tests with metrics
3. **run_stress_test** - Generates and runs progressive stress tests with analysis
4. **generate_report** - Parses k6 JSON output and creates formatted reports (markdown/html/json)

## Usage Workflow

```mermaid
stateDiagram-v2
    [*] --> Start: User asks Claude
    Start --> Parse: MCP receives request
    Parse --> Select: Choose appropriate tool
    
    Select --> Execute: execute_k6_test
    Select --> Generate: run_load_test
    Select --> Stress: run_stress_test
    
    Execute --> Run: Run existing script
    Generate --> Create: Generate k6 script
    Stress --> Create: Generate stress script
    
    Create --> Run: Execute generated script
    Run --> Collect: Gather metrics
    Collect --> Format: Process results
    Format --> Report: Return to user
    Report --> [*]
```

## What Step 0 Provides

### Foundation Elements
1. **Basic MCP Integration** - Simple request/response pattern with k6
2. **Script Generation** - Two patterns: constant load and progressive stress
3. **Test Target** - A working web application to test against
4. **Error Handling** - Basic error reporting from k6 execution
5. **Report Generation** - Parses k6 JSON output to extract key metrics:
   - Response time statistics (avg, min, max)
   - Success/failure rates
   - Data transfer metrics
   - Virtual user counts

### Current Limitations (To Be Addressed in Future Steps)
- No script discovery or management
- No real-time monitoring during tests
- Single-node execution only
- No test history or comparison
- Limited script customization options

## Step 0 Implementation Details

### Script Generation Functions

**`generateLoadTestScript`**
- Creates k6 script with constant-arrival-rate executor
- Configurable RPS, duration, HTTP method, and payload
- Includes basic error rate tracking
- Sets performance thresholds

**`generateStressTestScript`**
- Creates k6 script with staged load increases
- Three stages: ramp-up, sustain, ramp-down
- Monitors for breaking point detection
- Simple but effective stress pattern

## Running Step 0

1. **Build the servers:**
   ```bash
   make build-step0
   # or use air for hot reload:
   air
   ```

2. **Start the web server:**
   ```bash
   ./bin/web-server-step0
   # Web server runs on :8080
   ```

3. **Configure and run the MCP server:**
   ```bash
   ./bin/mcp-server-step0
   ```

## Step 0 Summary

This foundational step demonstrates:
- Basic MCP server implementation with k6 integration
- Two script generation patterns (load and stress)
- A functional test target application
- Simple tool-based architecture

It's intentionally minimal to establish the core pattern of:
1. Receiving natural language requests via MCP
2. Translating them into k6 test executions
3. Returning results in a readable format

Future steps will build upon this foundation to add more sophisticated testing capabilities, monitoring, and analysis features.