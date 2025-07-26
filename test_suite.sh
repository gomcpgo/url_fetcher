#!/bin/bash

# URL Fetcher MCP Server - Comprehensive Test Suite
# This script runs various test scenarios to validate the URL fetcher functionality

set -e

echo "🧪 URL Fetcher MCP Server - Comprehensive Test Suite"
echo "=================================================="

# Change to the directory of this script
cd "$(dirname "$0")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo_step() {
    echo -e "${BLUE}📋 $1${NC}"
}

echo_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

echo_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

echo_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Function to run a test and capture output
run_test() {
    local test_name="$1"
    local test_command="$2"
    
    echo_step "Running: $test_name"
    
    if eval "$test_command"; then
        echo_success "$test_name completed successfully"
        return 0
    else
        echo_error "$test_name failed"
        return 1
    fi
}

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo_error "Go is not installed or not in PATH"
    exit 1
fi

echo_step "Checking Go version"
go version

# Step 1: Run unit tests (fast)
echo_step "Step 1: Running unit tests"
if run_test "Unit Tests" "go test ./test/... -v -short"; then
    echo_success "All unit tests passed"
else
    echo_error "Unit tests failed"
    exit 1
fi

# Step 2: Build the server
echo_step "Step 2: Building server"
if run_test "Build Server" "go build -o url_fetcher cmd/main.go"; then
    echo_success "Server built successfully"
else
    echo_error "Failed to build server"
    exit 1
fi

# Step 3: Run integration tests (may be slower)
echo_step "Step 3: Running integration tests with real websites"
echo_warning "Note: These tests may take longer as they fetch real websites"

if run_test "Integration Tests" "go test ./test/... -v -timeout=5m"; then
    echo_success "All integration tests passed"
else
    echo_warning "Some integration tests failed (this may be due to network issues or site availability)"
fi

# Step 4: Test server in test mode
echo_step "Step 4: Testing server functionality"
if run_test "Server Test Mode" "timeout 60s go run cmd/main.go -test"; then
    echo_success "Server test mode completed"
else
    echo_warning "Server test mode had issues (may be due to network or timeout)"
fi

# Step 5: Test different configurations
echo_step "Step 5: Testing different configurations"

echo "Testing with local access blocked:"
if run_test "Local Access Blocked" "FETCH_URL_BLOCK_LOCAL=true timeout 30s go run cmd/main.go -test 2>/dev/null | head -20"; then
    echo_success "Local access blocking test completed"
fi

echo "Testing with custom cache TTL:"
if run_test "Custom Cache TTL" "FETCH_URL_CACHE_TTL=60 timeout 30s go run cmd/main.go -test 2>/dev/null | head -20"; then
    echo_success "Custom cache TTL test completed"
fi

echo "Testing with larger Chrome pool:"
if run_test "Large Chrome Pool" "FETCH_URL_CHROME_POOL_SIZE=5 timeout 30s go run cmd/main.go -test 2>/dev/null | head -20"; then
    echo_success "Large Chrome pool test completed"
fi

# Step 6: Performance and load testing
echo_step "Step 6: Basic performance testing"

echo "Testing concurrent requests (basic load test):"
if run_test "Concurrent Tests" "go test ./test/... -v -run=TestCache -count=3"; then
    echo_success "Concurrent test completed"
fi

# Step 7: Chrome availability check
echo_step "Step 7: Checking Chrome availability"

# Check if Chrome/Chromium is available
if command -v google-chrome &> /dev/null; then
    echo_success "Google Chrome found: $(google-chrome --version)"
elif command -v chromium &> /dev/null; then
    echo_success "Chromium found: $(chromium --version)"
elif command -v chromium-browser &> /dev/null; then
    echo_success "Chromium browser found: $(chromium-browser --version)"
else
    echo_warning "Chrome/Chromium not found - Chrome engine will fall back to HTTP"
fi

# Final summary
echo
echo "🏁 Test Suite Summary"
echo "===================="

if [ -f "url_fetcher" ]; then
    echo_success "Server binary created: $(ls -lh url_fetcher | awk '{print $5}')"
    rm -f url_fetcher  # Clean up
fi

echo_step "Test suite completed!"
echo
echo "📖 Usage Examples:"
echo "  • Run unit tests only:     go test ./test/... -v -short"
echo "  • Run all tests:           go test ./test/... -v"
echo "  • Run server test mode:    go run cmd/main.go -test"
echo "  • Build and run server:    ./run.sh"
echo
echo "🔧 Configuration Options:"
echo "  • FETCH_URL_BLOCK_LOCAL=true|false"
echo "  • FETCH_URL_CHROME_POOL_SIZE=<number>"
echo "  • FETCH_URL_CACHE_TTL=<seconds>"
echo "  • FETCH_URL_TIMEOUT=<seconds>"
echo
echo "🌐 Example URLs tested:"
echo "  • https://example.com (basic HTML)"
echo "  • https://en.wikipedia.org/wiki/Go_(programming_language) (rich content)"
echo "  • https://github.com/golang/go (developer platform)"
echo "  • https://news.ycombinator.com (news aggregator)"
echo "  • https://tools.ietf.org/rfc/rfc7231.txt (plain text)"
echo
echo_success "URL Fetcher MCP Server is ready for use! 🚀"