#!/bin/bash

# Build and run the URL Fetcher MCP server

set -e

# Change to the directory of this script
cd "$(dirname "$0")"

# Build the server
echo "Building URL Fetcher MCP server..."
go build -o url_fetcher cmd/main.go

# Run the server
echo "Starting URL Fetcher MCP server..."
./url_fetcher "$@"