package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/gomcpgo/mcp/pkg/handler"
	"github.com/gomcpgo/mcp/pkg/protocol"
	"github.com/gomcpgo/mcp/pkg/server"
	"github.com/gomcpgo/url_fetcher/pkg/cache"
	"github.com/gomcpgo/url_fetcher/pkg/config"
	"github.com/gomcpgo/url_fetcher/pkg/fetcher"
	"github.com/gomcpgo/url_fetcher/pkg/processor"
	"github.com/gomcpgo/url_fetcher/pkg/types"
)

// URLFetcherMCPServer implements the MCP server for URL fetching
type URLFetcherMCPServer struct {
	config    *config.Config
	fetcher   *fetcher.Fetcher
	processor *processor.Processor
	cache     *cache.Cache
}

// NewURLFetcherMCPServer creates a new URL Fetcher MCP server
func NewURLFetcherMCPServer() (*URLFetcherMCPServer, error) {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return &URLFetcherMCPServer{
		config:    cfg,
		fetcher:   fetcher.NewFetcher(cfg),
		processor: processor.NewProcessor(),
		cache:     cache.NewCache(cfg.CacheTTL),
	}, nil
}

// ListTools returns the available tools
func (s *URLFetcherMCPServer) ListTools(ctx context.Context) (*protocol.ListToolsResponse, error) {
	inputSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "URL to fetch",
			},
			"engine": map[string]interface{}{
				"type":        "string",
				"description": "Fetching engine: 'http' (default) or 'chrome'",
				"enum":        []string{"http", "chrome"},
				"default":     "http",
			},
			"format": map[string]interface{}{
				"type":        "string",
				"description": "Output format: 'text' (default), 'html', or 'markdown'",
				"enum":        []string{"text", "html", "markdown"},
				"default":     "text",
			},
			"max_content_length": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum content length in bytes (default: 10MB)",
				"default":     types.DefaultMaxContentLength,
			},
		},
		"required": []string{"url"},
	}

	schemaBytes, err := json.Marshal(inputSchema)
	if err != nil {
		return nil, err
	}

	return &protocol.ListToolsResponse{
		Tools: []protocol.Tool{
			{
				Name:        "fetch_url",
				Description: "Fetch content from a URL. Use engine='chrome' for JavaScript-heavy sites that need browser rendering.",
				InputSchema: json.RawMessage(schemaBytes),
			},
		},
	}, nil
}

// CallTool executes a tool
func (s *URLFetcherMCPServer) CallTool(ctx context.Context, req *protocol.CallToolRequest) (*protocol.CallToolResponse, error) {
	switch req.Name {
	case "fetch_url":
		result, err := s.fetchURL(req.Arguments)
		if err != nil {
			return &protocol.CallToolResponse{
				Content: []protocol.ToolContent{
					{
						Type: "text",
						Text: fmt.Sprintf("Error: %v", err),
					},
				},
				IsError: true,
			}, nil
		}

		// Convert result to JSON string
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return &protocol.CallToolResponse{
				Content: []protocol.ToolContent{
					{
						Type: "text",
						Text: fmt.Sprintf("Error formatting response: %v", err),
					},
				},
				IsError: true,
			}, nil
		}

		return &protocol.CallToolResponse{
			Content: []protocol.ToolContent{
				{
					Type: "text",
					Text: string(jsonBytes),
				},
			},
		}, nil

	default:
		return &protocol.CallToolResponse{
			Content: []protocol.ToolContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Unknown tool: %s", req.Name),
				},
			},
			IsError: true,
		}, nil
	}
}

// fetchURL handles the fetch_url tool
func (s *URLFetcherMCPServer) fetchURL(params map[string]interface{}) (interface{}, error) {
	// Parse request
	req := &types.FetchRequest{}

	// URL (required)
	url, ok := params["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("url is required")
	}
	req.URL = url

	// Engine (optional)
	if engine, ok := params["engine"].(string); ok {
		req.Engine = engine
	}

	// Format (optional)
	if format, ok := params["format"].(string); ok {
		req.Format = format
	}

	// Max content length (optional)
	if maxLen, ok := params["max_content_length"].(float64); ok {
		req.MaxContentLength = int(maxLen)
	}

	// Apply defaults
	if req.Engine == "" {
		req.Engine = types.DefaultEngine
	}
	if req.Format == "" {
		req.Format = types.DefaultFormat
	}
	if req.MaxContentLength == 0 {
		req.MaxContentLength = types.DefaultMaxContentLength
	}

	// Check cache
	if cached, found := s.cache.Get(req.URL, req.Engine, req.Format); found {
		return s.formatResponse(cached), nil
	}

	// Fetch content
	response, err := s.fetcher.Fetch(req)
	if err != nil {
		// Return formatted error response
		if response != nil {
			return s.formatResponse(response), nil
		}
		return s.formatErrorResponse(req.URL, err.Error()), nil
	}

	// Process content
	if err := s.processor.Process(response); err != nil {
		// Add warning but don't fail
		response.Warnings = append(response.Warnings, fmt.Sprintf("Content processing error: %v", err))
	}

	// Cache successful responses
	s.cache.Set(req.URL, req.Engine, req.Format, response)

	return s.formatResponse(response), nil
}

// formatResponse formats the response for MCP
func (s *URLFetcherMCPServer) formatResponse(resp *types.FetchResponse) map[string]interface{} {
	result := map[string]interface{}{
		"url":              resp.URL,
		"engine":           resp.Engine,
		"status_code":      resp.StatusCode,
		"content_type":     resp.ContentType,
		"content":          resp.Content,
		"format":           resp.Format,
		"fetch_time_ms":    resp.FetchTimeMs,
		"chrome_available": resp.ChromeAvailable,
	}

	if resp.Title != "" {
		result["title"] = resp.Title
	}

	if len(resp.Warnings) > 0 {
		result["warnings"] = resp.Warnings
	}

	return result
}

// formatErrorResponse formats an error response
func (s *URLFetcherMCPServer) formatErrorResponse(url, error string) map[string]interface{} {
	return map[string]interface{}{
		"url":    url,
		"error":  error,
		"status": "failed",
	}
}

// Close shuts down the server
func (s *URLFetcherMCPServer) Close() {
	if s.fetcher != nil {
		s.fetcher.Close()
	}
}

// Test mode for the server
func runTestMode() {
	fmt.Println("URL Fetcher MCP Server - Test Mode")
	fmt.Println("==================================")

	server, err := NewURLFetcherMCPServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	// Test cases
	testCases := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "Simple HTTP fetch",
			params: map[string]interface{}{
				"url":    "https://example.com",
				"format": "text",
			},
		},
		{
			name: "Markdown conversion",
			params: map[string]interface{}{
				"url":    "https://example.com",
				"format": "markdown",
			},
		},
	}

	for _, tc := range testCases {
		fmt.Printf("\nTest: %s\n", tc.name)
		fmt.Println("-------------------")

		result, err := server.fetchURL(tc.params)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		// Pretty print result
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(jsonBytes))
	}
}

func main() {
	// Parse command line flags
	testMode := flag.Bool("test", false, "Run in test mode")
	flag.Parse()

	if *testMode {
		runTestMode()
		return
	}

	// Create server
	urlServer, err := NewURLFetcherMCPServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer urlServer.Close()

	// Create handler registry
	registry := handler.NewHandlerRegistry()

	// Register URL fetcher as a tool handler
	registry.RegisterToolHandler(urlServer)

	// Create and run MCP server
	mcpServer := server.New(server.Options{
		Name:     "URL Fetcher",
		Version:  "1.0.0",
		Registry: registry,
	})

	if err := mcpServer.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
