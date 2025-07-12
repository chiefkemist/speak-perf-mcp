# Step 1 Usage Guide

## Quick Start

1. **Build the MCP server:**
   ```bash
   make build-step1
   # or
   air
   ```

2. **Run the MCP server:**
   ```bash
   ./bin/mcp-server-step1
   ```

## Automated Testing (Recommended)

### Option A: Complete Automated Test
```
"Test the application at https://raw.githubusercontent.com/example/repo/main/docker-compose.yml"
```

This single command will:
- Download the Docker Compose file
- Store it in SQLite with a unique session
- Start all services in an isolated environment
- Discover API specifications
- Generate and run performance tests
- Provide a comprehensive report
- Clean up all resources

### Option A2: Test Specific Endpoints
```
"Test the application at ./docker-compose.yml with endpoints /users,/products,/orders"
```

This will:
- Test only the specified endpoints
- Skip default endpoint discovery
- Focus performance testing on critical paths

### Option B: Quick Performance Test
```
"Run a quick performance test on ./example-compose.yml with 100 users for 5 minutes"
```

This will:
- Use your custom parameters
- Run a focused performance test
- Provide quick results

## Traditional Workflow (Step-by-Step)

### 1. Setup Test Environment
```
"Setup the test environment using https://example.com/docker-compose.yml"
```

This will:
- Download the Docker Compose file from the URL
- Store the complete content in SQLite
- Create a unique session for this environment
- Parse and validate the compose structure
- Services will be started automatically when needed

### 2. Discover API Specifications
```
"Discover API specifications from the running services"
```

This will:
- Temporarily start the Docker Compose environment
- Search for OpenAPI/Swagger specs at common endpoints
- Store discovered specs in the database
- Automatically stop containers when done

### 3. Generate API Tests
```
"Generate load tests for the pet endpoints from spec ID 1"
```

This creates k6 test scripts based on the OpenAPI specification.

### 4. Create UI Test
```
"Create a UI test for http://localhost:8081 that clicks the login button, enters 'testuser' as username and 'password123' as password, then submits the form"
```

This converts natural language to k6 browser test script.

### 5. Run Performance Test
```
"Run test ID 1 with 50 virtual users for 2 minutes"
```

This will:
- Start the Docker Compose environment
- Execute the k6 test against running services
- Store results in the database
- Automatically stop and remove all containers when done

### 6. Analyze Results
```
"Analyze the results from run ID 1 and compare with historical data"
```

Checks performance against SLAs and historical trends.

### 7. Query History
```
"Show me the performance history for the last 7 days"
```

Retrieves historical data for trend analysis.

## Natural Language UI Test Examples

### Simple Form Test
```
"Click the login button, type username, type password, click submit"
```

### Navigation Test
```
"Click the products link, wait 2 seconds, click the first product, click add to cart"
```

### Search Test
```
"Type 'laptop' in the search box, click search button, wait for results"
```

## Docker Compose Requirements

Your Docker Compose file should:
- Use only Docker images (no build contexts)
- Include port mappings for endpoint discovery
- Have all environment values embedded (no external .env files)
- Not reference local volumes or files

Example:
```yaml
version: '3.8'
services:
  my-api:
    image: my-api:latest
    ports:
      - "3000:3000"
    environment:
      - API_KEY=test-key-123
      - DATABASE_URL=postgres://db/myapp
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
```

## OpenAPI/Swagger Paths

If your API doesn't expose specs at common paths, provide them explicitly:
```
"Discover API specs at /my-api/docs/swagger.json"
```

## Database Access

### MCP Resources

The MCP server exposes database contents through resources:

- **sqlite://schema** - View the complete database schema
- **sqlite://sessions** - Recent test sessions with metadata
- **sqlite://compose-files** - Stored Docker Compose files info
- **sqlite://test-runs** - Recent test execution results

Example usage in your MCP client:
```
"Show me the database schema"
"List recent test sessions"
"What compose files have been tested?"
"Show recent test run results"
```

### Direct Database Access

The SQLite database is created at `./perf_test.db` in the working directory.

To view data:
```bash
sqlite3 perf_test.db
.tables
SELECT * FROM compose_files;
SELECT * FROM test_sessions ORDER BY created_at DESC;
SELECT * FROM test_runs ORDER BY started_at DESC LIMIT 10;
```

Key tables:
- `compose_files`: Stores Docker Compose content with hashes
- `test_sessions`: Groups related operations
- `services`: Extracted from compose files
- `test_runs`: Individual test executions
- `metrics`: Performance measurements

## Troubleshooting

**Docker Compose not found:**
- Ensure Docker and Docker Compose are installed
- For URLs, check network connectivity
- Verify the URL is publicly accessible

**Container startup fails:**
- Check Docker daemon is running
- Ensure no port conflicts with existing containers
- Verify all images in compose file are accessible

**API spec discovery fails:**
- The MCP server starts containers automatically
- Check the compose file has correct port mappings
- Try providing explicit spec paths

**UI tests fail:**
- Ensure k6 browser module is available
- Check that the target URL is accessible
- Verify selectors match the page structure

**SQLite errors:**
- Check write permissions in the current directory
- Ensure the database isn't locked by another process

**Cleanup issues:**
- Containers are automatically cleaned up
- Check for orphaned containers: `docker ps -a`
- Remove manually if needed: `docker-compose down`