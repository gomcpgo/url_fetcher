package test

import (
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
