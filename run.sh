#!/bin/bash

# URL Fetcher MCP Server - Build and Run Script
# Environment Variables:
# - FETCH_URL_BLOCK_LOCAL: Block local/private IPs (default: true)
# - FETCH_URL_CHROME_POOL_SIZE: Chrome browser pool size (default: 3)
# - FETCH_URL_CACHE_TTL: Cache TTL in seconds (default: 3600)
# - FETCH_URL_TIMEOUT: Request timeout in seconds (default: 30)

set -e

# Change to the directory of this script
cd "$(dirname "$0")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Show usage if no command provided
function show_usage() {
    echo "Usage: ./run.sh [command] [options]"
    echo ""
    echo "Commands:"
    echo "  build        Build the URL Fetcher MCP server binary"
    echo "  run          Run the URL Fetcher MCP server"
    echo "  test         Run the server in test mode with sample URLs"
    echo "  test-unit    Run unit tests only (fast)"
    echo "  test-full    Run full test suite including real websites"
    echo "  test-suite   Run comprehensive test suite with reporting"
    echo "  clean        Remove built binaries and temporary files"
    echo "  dev          Development mode with auto-restart (requires 'air')"
    echo "  version      Show version information"
    echo "  help         Show this help message"
    echo ""
    echo "Examples:"
    echo "  ./run.sh build                    # Build the server"
    echo "  ./run.sh run                      # Run the server"
    echo "  ./run.sh test                     # Test with sample URLs"
    echo "  ./run.sh test-full                # Run all tests"
    echo ""
    echo "Environment Variables:"
    echo "  FETCH_URL_BLOCK_LOCAL=true        # Block local/private IPs"
    echo "  FETCH_URL_CHROME_POOL_SIZE=5      # Use 5 Chrome instances"
    echo "  FETCH_URL_CACHE_TTL=1800          # 30-minute cache TTL"
    echo "  FETCH_URL_TIMEOUT=45              # 45-second timeout"
    echo ""
    echo "Configuration Examples:"
    echo "  FETCH_URL_BLOCK_LOCAL=false ./run.sh test"
    echo "  FETCH_URL_CHROME_POOL_SIZE=5 ./run.sh run"
    exit 1
}

function echo_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

function echo_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

function echo_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

function echo_error() {
    echo -e "${RED}❌ $1${NC}"
}

function create_bin_dir() {
    if [ ! -d "bin" ]; then
        echo_info "Creating bin directory..."
        mkdir -p bin
    fi
}

function check_dependencies() {
    if ! command -v go &> /dev/null; then
        echo_error "Go is not installed or not in PATH"
        exit 1
    fi
    
    echo_info "Go version: $(go version)"
}

function build_server() {
    echo_info "Building URL Fetcher MCP server..."
    create_bin_dir
    check_dependencies
    
    # Build with optimizations and version info
    local version=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    local build_time=$(date -u '+%Y-%m-%d_%H:%M:%S')
    local ldflags="-X main.Version=${version} -X main.BuildTime=${build_time}"
    
    go build -ldflags "${ldflags}" -o bin/url-fetcher ./cmd
    
    if [ -f "bin/url-fetcher" ]; then
        echo_success "Built bin/url-fetcher ($(ls -lh bin/url-fetcher | awk '{print $5}'))"
        echo_info "Binary location: $(pwd)/bin/url-fetcher"
    else
        echo_error "Build failed - binary not created"
        exit 1
    fi
}

function run_server() {
    echo_info "Running URL Fetcher MCP server..."
    check_dependencies
    
    # Show configuration
    echo_info "Configuration:"
    echo "  FETCH_URL_BLOCK_LOCAL: ${FETCH_URL_BLOCK_LOCAL:-true (default)}"
    echo "  FETCH_URL_CHROME_POOL_SIZE: ${FETCH_URL_CHROME_POOL_SIZE:-3 (default)}"
    echo "  FETCH_URL_CACHE_TTL: ${FETCH_URL_CACHE_TTL:-3600 (default)}"
    echo "  FETCH_URL_TIMEOUT: ${FETCH_URL_TIMEOUT:-30 (default)}"
    echo ""
    
    # Check if Chrome is available
    if command -v google-chrome &> /dev/null || command -v chromium &> /dev/null || command -v chromium-browser &> /dev/null; then
        echo_success "Chrome/Chromium detected - Chrome engine available"
    else
        echo_warning "Chrome/Chromium not found - will fall back to HTTP engine"
    fi
    
    echo_info "Starting server... (Press Ctrl+C to stop)"
    echo ""
    
    go run ./cmd/main.go
}

function run_test_mode() {
    echo_info "Running URL Fetcher in test mode..."
    check_dependencies
    
    echo_info "Testing with various websites and formats..."
    echo ""
    
    go run ./cmd/main.go -test
}

function run_unit_tests() {
    echo_info "Running unit tests..."
    check_dependencies
    
    go test ./test/... -v -short
    
    if [ $? -eq 0 ]; then
        echo_success "All unit tests passed!"
    else
        echo_error "Some unit tests failed"
        exit 1
    fi
}

function run_full_tests() {
    echo_info "Running full test suite (including real websites)..."
    echo_warning "This may take several minutes and requires internet connectivity"
    check_dependencies
    
    go test ./test/... -v -timeout=10m
    
    if [ $? -eq 0 ]; then
        echo_success "All tests passed!"
    else
        echo_warning "Some tests failed (may be due to network issues)"
    fi
}

function run_test_suite() {
    echo_info "Running comprehensive test suite..."
    check_dependencies
    
    if [ -f "test_suite.sh" ]; then
        ./test_suite.sh
    else
        echo_error "test_suite.sh not found"
        exit 1
    fi
}

function clean_build() {
    echo_info "Cleaning build artifacts..."
    
    if [ -d "bin" ]; then
        rm -rf bin
        echo_success "Removed bin directory"
    fi
    
    # Clean Go build cache
    go clean -cache
    echo_success "Cleaned Go build cache"
    
    # Remove any test binaries
    find . -name "*.test" -delete 2>/dev/null || true
    echo_success "Removed test binaries"
}

function dev_mode() {
    echo_info "Starting development mode..."
    
    # Check if air is installed
    if ! command -v air &> /dev/null; then
        echo_warning "Air not found. Installing..."
        go install github.com/cosmtrek/air@latest
        
        if ! command -v air &> /dev/null; then
            echo_error "Failed to install air. Please install manually:"
            echo "  go install github.com/cosmtrek/air@latest"
            exit 1
        fi
    fi
    
    # Create air config if it doesn't exist
    if [ ! -f ".air.toml" ]; then
        echo_info "Creating air configuration..."
        cat > .air.toml << 'EOF'
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ./cmd"
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata", "bin"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  kill_delay = "0s"
  log = "build-errors.log"
  send_interrupt = false
  stop_on_root = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
EOF
    fi
    
    echo_success "Starting development server with auto-restart..."
    air
}

function show_version() {
    echo_info "URL Fetcher MCP Server Version Information"
    
    if [ -f "bin/url-fetcher" ]; then
        echo_info "Built binary version:"
        ./bin/url-fetcher -version
    else
        echo_info "Source version (binary not built):"
        go run ./cmd/main.go -version
    fi
    
    echo ""
    echo_info "Build information:"
    echo "  Go version: $(go version)"
    if command -v git &> /dev/null; then
        echo "  Git commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'not available')"
        echo "  Git status: $(git status --porcelain 2>/dev/null | wc -l | tr -d ' ') uncommitted changes"
    fi
}

# Handle different commands
case "$1" in
  build)
    build_server
    ;;
  run)
    run_server
    ;;
  test)
    run_test_mode
    ;;
  test-unit)
    run_unit_tests
    ;;
  test-full)
    run_full_tests
    ;;
  test-suite)
    run_test_suite
    ;;
  clean)
    clean_build
    ;;
  dev)
    dev_mode
    ;;
  version)
    show_version
    ;;
  help|--help|-h)
    show_usage
    ;;
  *)
    if [ -z "$1" ]; then
        echo_error "No command specified"
    else
        echo_error "Unknown command: $1"
    fi
    echo ""
    show_usage
    ;;
esac