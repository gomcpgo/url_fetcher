package test

import (
	"strings"
	"testing"
	"time"

	"github.com/gomcpgo/url_fetcher/pkg/cache"
	"github.com/gomcpgo/url_fetcher/pkg/config"
	"github.com/gomcpgo/url_fetcher/pkg/fetcher"
	"github.com/gomcpgo/url_fetcher/pkg/processor"
	"github.com/gomcpgo/url_fetcher/pkg/types"
)

func TestHTTPEngine(t *testing.T) {
	cfg := &config.Config{
		BlockLocal:     false,
		ChromePoolSize: 3,
		CacheTTL:       time.Hour,
		Timeout:        30 * time.Second,
	}
	
	f := fetcher.NewFetcher(cfg)
	defer f.Close()
	
	req := &types.FetchRequest{
		URL:              "https://example.com",
		Engine:           types.EngineHTTP,
		Format:           types.FormatText,
		MaxContentLength: 1024 * 1024, // 1MB
	}
	
	resp, err := f.Fetch(req)
	if err != nil {
		t.Fatalf("Failed to fetch URL: %v", err)
	}
	
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
	
	if resp.Engine != types.EngineHTTP {
		t.Errorf("Expected engine %s, got %s", types.EngineHTTP, resp.Engine)
	}
}

func TestContentProcessor(t *testing.T) {
	p := processor.NewProcessor()
	
	testHTML := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Test Page</title>
		<script>console.log('test');</script>
		<style>body { color: red; }</style>
	</head>
	<body>
		<h1>Hello World</h1>
		<p>This is a <strong>test</strong> paragraph.</p>
		<ul>
			<li>Item 1</li>
			<li>Item 2</li>
		</ul>
	</body>
	</html>
	`
	
	tests := []struct {
		name   string
		format string
	}{
		{"Text extraction", types.FormatText},
		{"HTML cleaning", types.FormatHTML},
		{"Markdown conversion", types.FormatMarkdown},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &types.FetchResponse{
				Content: testHTML,
				Format:  tt.format,
			}
			
			err := p.Process(resp)
			if err != nil {
				t.Errorf("Process failed: %v", err)
			}
			
			if resp.Title != "Test Page" {
				t.Errorf("Expected title 'Test Page', got '%s'", resp.Title)
			}
			
			// Check that scripts and styles are removed
			if tt.format == types.FormatText || tt.format == types.FormatMarkdown {
				if containsString(resp.Content, "console.log") || containsString(resp.Content, "color: red") {
					t.Error("Scripts or styles were not removed")
				}
			}
		})
	}
}

func TestCache(t *testing.T) {
	c := cache.NewCache(time.Second * 2)
	
	resp := &types.FetchResponse{
		URL:        "https://example.com",
		StatusCode: 200,
		Content:    "Test content",
	}
	
	// Test Set and Get
	c.Set("https://example.com", types.EngineHTTP, types.FormatText, resp)
	
	cached, found := c.Get("https://example.com", types.EngineHTTP, types.FormatText)
	if !found {
		t.Error("Expected to find cached response")
	}
	
	if cached.Content != resp.Content {
		t.Errorf("Expected content '%s', got '%s'", resp.Content, cached.Content)
	}
	
	// Test expiration
	time.Sleep(time.Second * 3)
	
	_, found = c.Get("https://example.com", types.EngineHTTP, types.FormatText)
	if found {
		t.Error("Expected cached response to be expired")
	}
}

func TestURLValidation(t *testing.T) {
	cfg := &config.Config{
		BlockLocal:     true,
		ChromePoolSize: 3,
		CacheTTL:       time.Hour,
		Timeout:        30 * time.Second,
	}
	
	f := fetcher.NewFetcher(cfg)
	defer f.Close()
	
	tests := []struct {
		url       string
		shouldErr bool
	}{
		{"https://example.com", false},
		{"http://localhost", true},
		{"http://127.0.0.1", true},
		{"http://192.168.1.1", true},
		{"http://10.0.0.1", true},
		{"file:///etc/passwd", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			req := &types.FetchRequest{
				URL:    tt.url,
				Engine: types.EngineHTTP,
				Format: types.FormatText,
			}
			
			_, err := f.Fetch(req)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for URL %s, but got none", tt.url)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Did not expect error for URL %s, but got: %v", tt.url, err)
			}
		})
	}
}

// Enhanced integration tests with real websites
func TestRealWebsiteIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real website tests in short mode")
	}
	
	cfg := &config.Config{
		BlockLocal:     false,
		ChromePoolSize: 3,
		CacheTTL:       time.Hour,
		Timeout:        45 * time.Second, // Longer timeout for real sites
	}
	
	f := fetcher.NewFetcher(cfg)
	defer f.Close()
	
	testCases := []struct {
		name        string
		url         string
		engine      string
		expectTitle bool
		minLength   int
		description string
	}{
		{
			name:        "Wikipedia - Static Content",
			url:         "https://en.wikipedia.org/wiki/Go_(programming_language)",
			engine:      types.EngineHTTP,
			expectTitle: true,
			minLength:   1000,
			description: "Test fetching Wikipedia article with rich content",
		},
		{
			name:        "GitHub - Developer Site",
			url:         "https://github.com/golang/go",
			engine:      types.EngineHTTP,
			expectTitle: true,
			minLength:   500,
			description: "Test fetching GitHub repository page",
		},
		{
			name:        "Hacker News - News Site",
			url:         "https://news.ycombinator.com",
			engine:      types.EngineHTTP,
			expectTitle: true,
			minLength:   1000,
			description: "Test fetching news aggregation site",
		},
		{
			name:        "MDN Web Docs - Documentation",
			url:         "https://developer.mozilla.org/en-US/docs/Web/JavaScript",
			engine:      types.EngineHTTP,
			expectTitle: true,
			minLength:   2000,
			description: "Test fetching technical documentation",
		},
		{
			name:        "RFC Document - Technical Spec",
			url:         "https://tools.ietf.org/rfc/rfc7231.txt",
			engine:      types.EngineHTTP,
			expectTitle: false, // Plain text RFC
			minLength:   10000,
			description: "Test fetching plain text technical specification",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.description)
			
			req := &types.FetchRequest{
				URL:              tc.url,
				Engine:           tc.engine,
				Format:           types.FormatText,
				MaxContentLength: 2 * 1024 * 1024, // 2MB
			}
			
			resp, err := f.Fetch(req)
			if err != nil {
				t.Fatalf("Failed to fetch %s: %v", tc.url, err)
			}
			
			// Basic response validation
			if resp.StatusCode != 200 {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
			
			if len(resp.Content) < tc.minLength {
				t.Errorf("Content too short: expected at least %d chars, got %d", tc.minLength, len(resp.Content))
			}
			
			if tc.expectTitle && resp.Title == "" {
				t.Error("Expected title to be extracted")
			}
			
			t.Logf("✓ Successfully fetched %d chars from %s", len(resp.Content), tc.url)
			if resp.Title != "" {
				t.Logf("  Title: %s", resp.Title)
			}
		})
	}
}

func TestFormatConversion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping format conversion tests in short mode")
	}
	
	cfg := &config.Config{
		BlockLocal:     false,
		ChromePoolSize: 3,
		CacheTTL:       time.Hour,
		Timeout:        30 * time.Second,
	}
	
	f := fetcher.NewFetcher(cfg)
	defer f.Close()
	p := processor.NewProcessor()
	
	// Test URL with rich content for format conversion
	testURL := "https://en.wikipedia.org/wiki/Markdown"
	
	formats := []struct {
		format      string
		expectation string
		validator   func(string, *testing.T)
	}{
		{
			format:      types.FormatText,
			expectation: "clean text content",
			validator: func(content string, t *testing.T) {
				if strings.Contains(content, "<") || strings.Contains(content, ">") {
					t.Error("Text format should not contain HTML tags")
				}
				if len(content) < 1000 {
					t.Error("Text content seems too short")
				}
			},
		},
		{
			format:      types.FormatHTML,
			expectation: "cleaned HTML structure",
			validator: func(content string, t *testing.T) {
				if !strings.Contains(content, "<html") {
					t.Error("HTML format should contain HTML structure")
				}
				if strings.Contains(content, "<script") {
					t.Error("HTML format should not contain script tags")
				}
				if strings.Contains(content, "<style") {
					t.Error("HTML format should not contain style tags")
				}
			},
		},
		{
			format:      types.FormatMarkdown,
			expectation: "markdown formatted content",
			validator: func(content string, t *testing.T) {
				// Check for markdown elements
				hasHeaders := strings.Contains(content, "#")
				hasLinks := strings.Contains(content, "](")
				hasBold := strings.Contains(content, "**")
				
				if !hasHeaders && !hasLinks && !hasBold {
					t.Error("Markdown format should contain markdown elements (headers, links, or bold)")
				}
				
				if strings.Contains(content, "<script") || strings.Contains(content, "<style") {
					t.Error("Markdown format should not contain script or style tags")
				}
			},
		},
	}
	
	for _, fmt := range formats {
		t.Run("Format_"+fmt.format, func(t *testing.T) {
			req := &types.FetchRequest{
				URL:              testURL,
				Engine:           types.EngineHTTP,
				Format:           fmt.format,
				MaxContentLength: 1024 * 1024, // 1MB
			}
			
			resp, err := f.Fetch(req)
			if err != nil {
				t.Fatalf("Failed to fetch for format %s: %v", fmt.format, err)
			}
			
			// Process the content
			err = p.Process(resp)
			if err != nil {
				t.Fatalf("Failed to process content for format %s: %v", fmt.format, err)
			}
			
			t.Logf("Testing %s format (%s)", fmt.format, fmt.expectation)
			t.Logf("Content length: %d chars", len(resp.Content))
			t.Logf("Title: %s", resp.Title)
			
			// Validate format-specific requirements
			fmt.validator(resp.Content, t)
			
			// Log a sample of the content for manual verification
			sample := resp.Content
			if len(sample) > 300 {
				sample = sample[:300] + "..."
			}
			t.Logf("Content sample: %s", sample)
		})
	}
}

func TestEngineComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping engine comparison tests in short mode")
	}
	
	cfg := &config.Config{
		BlockLocal:     false,
		ChromePoolSize: 3,
		CacheTTL:       time.Hour,
		Timeout:        30 * time.Second,
	}
	
	f := fetcher.NewFetcher(cfg)
	defer f.Close()
	
	// Test a site that might have different content when rendered with JS
	testURL := "https://example.com" // Simple site for comparison
	
	engines := []string{types.EngineHTTP, types.EngineChrome}
	results := make(map[string]*types.FetchResponse)
	
	for _, engine := range engines {
		t.Run("Engine_"+engine, func(t *testing.T) {
			req := &types.FetchRequest{
				URL:              testURL,
				Engine:           engine,
				Format:           types.FormatText,
				MaxContentLength: 1024 * 1024,
			}
			
			resp, err := f.Fetch(req)
			if err != nil {
				// Chrome might not be available, check for fallback
				if engine == types.EngineChrome && strings.Contains(err.Error(), "Chrome") {
					t.Skipf("Chrome not available: %v", err)
				}
				t.Fatalf("Failed to fetch with %s engine: %v", engine, err)
			}
			
			if resp.StatusCode != 200 {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
			
			results[engine] = resp
			
			t.Logf("✓ %s engine: %d chars, took %dms", 
				engine, len(resp.Content), resp.FetchTimeMs)
			
			// Verify Chrome availability reporting
			if engine == types.EngineChrome {
				t.Logf("Chrome available: %v", resp.ChromeAvailable)
			}
		})
	}
	
	// Compare results if both engines worked
	if len(results) == 2 {
		httpResp := results[types.EngineHTTP]
		chromeResp := results[types.EngineChrome]
		
		t.Logf("Content length comparison - HTTP: %d, Chrome: %d", 
			len(httpResp.Content), len(chromeResp.Content))
		
		// For example.com, content should be very similar
		if abs(len(httpResp.Content)-len(chromeResp.Content)) > 100 {
			t.Logf("Note: Significant content length difference between engines")
		}
	}
}

func TestContentSizeLimits(t *testing.T) {
	cfg := &config.Config{
		BlockLocal:     false,
		ChromePoolSize: 3,
		CacheTTL:       time.Hour,
		Timeout:        30 * time.Second,
	}
	
	f := fetcher.NewFetcher(cfg)
	defer f.Close()
	
	req := &types.FetchRequest{
		URL:              "https://example.com",
		Engine:           types.EngineHTTP,
		Format:           types.FormatText,
		MaxContentLength: 500, // Very small limit
	}
	
	resp, err := f.Fetch(req)
	// Content size limit may cause an error or truncation
	if err != nil {
		// If there's an error, it should be due to content size limit
		if !strings.Contains(err.Error(), "content exceeds maximum length") {
			t.Fatalf("Unexpected error: %v", err)
		}
		t.Logf("Content correctly rejected due to size limit: %v", err)
		return
	}
	
	// If no error, content should be limited
	if len(resp.Content) > 500 {
		t.Errorf("Content should be limited to 500 chars, got %d", len(resp.Content))
	}
	
	t.Logf("Content successfully limited to %d chars", len(resp.Content))
}

func TestChromeEngineWithJavaScript(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Chrome engine tests in short mode")
	}
	
	cfg := &config.Config{
		BlockLocal:     false,
		ChromePoolSize: 2,
		CacheTTL:       time.Hour,
		Timeout:        45 * time.Second, // Longer for JS-heavy sites
	}
	
	f := fetcher.NewFetcher(cfg)
	defer f.Close()
	
	// Test a site that heavily relies on JavaScript
	// Note: Using a simple site for now since heavy JS sites might be flaky in tests
	req := &types.FetchRequest{
		URL:              "https://example.com",
		Engine:           types.EngineChrome,
		Format:           types.FormatText,
		MaxContentLength: 1024 * 1024,
	}
	
	resp, err := f.Fetch(req)
	if err != nil {
		if strings.Contains(err.Error(), "Chrome") {
			t.Skipf("Chrome not available: %v", err)
		}
		t.Fatalf("Failed to fetch with Chrome: %v", err)
	}
	
	if resp.Engine != types.EngineChrome && resp.Engine != types.EngineHTTP {
		t.Errorf("Unexpected engine in response: %s", resp.Engine)
	}
	
	// If Chrome wasn't available, should have warning
	if resp.Engine == types.EngineHTTP && len(resp.Warnings) == 0 {
		t.Error("Expected warning when falling back to HTTP engine")
	}
	
	t.Logf("✓ Chrome engine test completed")
	t.Logf("  Engine used: %s", resp.Engine)
	t.Logf("  Chrome available: %v", resp.ChromeAvailable)
	t.Logf("  Content length: %d", len(resp.Content))
	if len(resp.Warnings) > 0 {
		t.Logf("  Warnings: %v", resp.Warnings)
	}
}

// Helper functions
func containsString(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) && 
		(s == substr || len(s) > len(substr) && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}