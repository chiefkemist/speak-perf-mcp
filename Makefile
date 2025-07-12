.PHONY: all build run-web run-mcp run-all dev test clean logs tail-log

# Build all components
all: build

# Build all servers with step numbers
build:
	@echo "Creating logs directory..."
	@mkdir -p logs
	@echo "Building step0..."
	@cd step0/mcp && go build -o ../../bin/mcp-server-step0 .
	@cd step0/web && go build -o ../../bin/web-server-step0 .
	@echo "Building step1..."
	@cd step1/mcp && go build -o ../../bin/mcp-server-step1 .
	@echo "Build complete!"

# Build specific step
build-step%:
	@echo "Building step$*..."
	@if [ -d "step$*/mcp" ]; then cd step$*/mcp && go build -o ../../bin/mcp-server-step$* .; fi
	@if [ -d "step$*/web" ]; then cd step$*/web && go build -o ../../bin/web-server-step$* .; fi
	@echo "Step$* build complete!"

# Run web server only
run-web: build
	@echo "Starting web server on :8080..."
	@./bin/web-server-step0

# Run MCP server only
run-mcp: build
	@echo "Starting MCP server..."
	@MCP_LOG_DIR=./logs ./bin/mcp-server-step0

# Run both servers in parallel (requires GNU parallel or similar)
run-all: build
	@echo "Starting both servers..."
	@echo "Web server on :8080"
	@echo "MCP server on stdio"
	@trap 'kill %1 %2' INT; \
	./bin/web-server-step0 & \
	./bin/mcp-server-step0 & \
	wait

# Development mode with hot reload using air
dev:
	@echo "Starting development mode with hot reload..."
	@air

# Run k6 tests
test-basic:
	k6 run step0/k6-scripts/basic-test.js

test-api:
	k6 run -e BASE_URL=http://localhost:8080 step0/k6-scripts/api-test.js

test-spike:
	k6 run step0/k6-scripts/spike-test.js

test-all: test-basic test-api test-spike

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/ tmp/ logs/
	@echo "Clean complete!"

# View logs
logs:
	@if [ -d logs ]; then \
		echo "Available log files:"; \
		ls -la logs/; \
		echo ""; \
		echo "To tail a log file, run: tail -f logs/<filename>"; \
	else \
		echo "No logs directory found. Run 'make build' first."; \
	fi

# Tail the latest log file
tail-log:
	@if [ -d logs ] && [ "$$(ls -A logs 2>/dev/null)" ]; then \
		tail -f logs/$$(ls -t logs/ | head -1); \
	else \
		echo "No log files found. Run the MCP server first."; \
	fi

# Install dependencies
deps:
	@echo "Installing Go dependencies..."
	@go work sync
	@cd step0/mcp && go mod tidy
	@cd step0/web && go mod tidy
	@echo "Dependencies installed!"

# Check if k6 is installed
check-k6:
	@which k6 > /dev/null || (echo "k6 is not installed. Install with: brew install k6" && exit 1)

# Check if air is installed
check-air:
	@which air > /dev/null || (echo "air is not installed. Install with: go install github.com/cosmtrek/air@latest" && exit 1)

# Setup development environment
setup: check-k6 check-air deps
	@echo "Development environment setup complete!"