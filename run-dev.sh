#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Starting Speak Performance MCP Development Environment${NC}"
echo ""

# Check if air is installed
if ! command -v air &> /dev/null; then
    echo -e "${YELLOW}⚠️  Air is not installed. Installing...${NC}"
    go install github.com/cosmtrek/air@latest
fi

# Check if k6 is installed
if ! command -v k6 &> /dev/null; then
    echo -e "${RED}❌ k6 is not installed. Please install it:${NC}"
    echo "   macOS: brew install k6"
    echo "   Linux: sudo snap install k6"
    echo "   Or visit: https://k6.io/docs/getting-started/installation/"
    exit 1
fi

# Create necessary directories
mkdir -p bin tmp

# Function to cleanup on exit
cleanup() {
    echo -e "\n${YELLOW}🛑 Shutting down servers...${NC}"
    pkill -f "mcp-server" 2>/dev/null
    pkill -f "web-server" 2>/dev/null
    exit 0
}

trap cleanup INT TERM

# Start air for hot reload
echo -e "${GREEN}✨ Starting Air for hot reload...${NC}"
echo -e "${YELLOW}📝 Web server will run on http://localhost:8080${NC}"
echo -e "${YELLOW}📝 MCP server binary will be built at ./tmp/mcp-server${NC}"
echo ""
echo -e "${GREEN}To use the MCP server with Claude:${NC}"
echo "1. Run: ./tmp/mcp-server"
echo "2. Configure it in your MCP client"
echo ""
echo -e "${GREEN}Press Ctrl+C to stop${NC}"
echo ""

# Run air
air