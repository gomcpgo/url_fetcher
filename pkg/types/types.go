package types

import "time"

// Engine types
const (
	EngineHTTP   = "http"
	EngineChrome = "chrome"
)

// Format types
const (
	FormatText     = "text"
	FormatHTML     = "html"
	FormatMarkdown = "markdown"
)

// Default values
const (
	DefaultEngine          = EngineHTTP
	DefaultFormat          = FormatText
	DefaultMaxContentLength = 10 * 1024 * 1024 // 10MB
	DefaultUserAgent       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// FetchRequest represents a request to fetch a URL
type FetchRequest struct {
	URL              string `json:"url"`
	Engine           string `json:"engine,omitempty"`
	Format           string `json:"format,omitempty"`
	MaxContentLength int    `json:"max_content_length,omitempty"`
}

// FetchResponse represents the response from fetching a URL
type FetchResponse struct {
	URL             string   `json:"url"`
	Engine          string   `json:"engine"`
	StatusCode      int      `json:"status_code"`
	ContentType     string   `json:"content_type"`
	Content         string   `json:"content"`
	Format          string   `json:"format"`
	Title           string   `json:"title,omitempty"`
	FetchTimeMs     int64    `json:"fetch_time_ms"`
	Warnings        []string `json:"warnings,omitempty"`
	ChromeAvailable bool     `json:"chrome_available"`
}

// CacheEntry represents a cached response
type CacheEntry struct {
	Response  *FetchResponse
	ExpiresAt time.Time
}

// Error response helper
func ErrorResponse(url string, engine string, err error, fetchTime time.Duration) *FetchResponse {
	return &FetchResponse{
		URL:         url,
		Engine:      engine,
		StatusCode:  0,
		Content:     err.Error(),
		Format:      FormatText,
		FetchTimeMs: fetchTime.Milliseconds(),
	}
}