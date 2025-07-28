package fetcher

import (
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gomcpgo/url_fetcher/pkg/config"
	"github.com/gomcpgo/url_fetcher/pkg/types"
)

// HTTPEngine handles HTTP-based URL fetching
type HTTPEngine struct {
	client *http.Client
	config *config.Config
}

// NewHTTPEngine creates a new HTTP engine
func NewHTTPEngine(cfg *config.Config) *HTTPEngine {
	transport := &http.Transport{
		DisableCompression:    false,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	return &HTTPEngine{
		client: client,
		config: cfg,
	}
}

// Fetch retrieves content from a URL using HTTP
func (e *HTTPEngine) Fetch(fetchURL string, maxContentLength int) (*types.FetchResponse, error) {
	startTime := time.Now()

	// Validate URL
	if err := e.validateURL(fetchURL); err != nil {
		return types.ErrorResponse(fetchURL, types.EngineHTTP, err, time.Since(startTime)), err
	}

	// Create request
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return types.ErrorResponse(fetchURL, types.EngineHTTP, err, time.Since(startTime)), err
	}

	// Set browser-like headers
	req.Header.Set("User-Agent", types.DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")

	// Execute request with retry logic for server errors
	var resp *http.Response
	maxRetries := 2

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Add small delay between retries (except first attempt)
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		// Create new request for each attempt (in case body was consumed)
		if attempt > 0 {
			req, err = http.NewRequest("GET", fetchURL, nil)
			if err != nil {
				return types.ErrorResponse(fetchURL, types.EngineHTTP, err, time.Since(startTime)), err
			}

			// Re-set headers for retry attempts
			req.Header.Set("User-Agent", types.DefaultUserAgent)
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			req.Header.Set("Accept-Encoding", "gzip, deflate, br")
			req.Header.Set("DNT", "1")
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("Upgrade-Insecure-Requests", "1")
			req.Header.Set("Sec-Fetch-Dest", "document")
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			req.Header.Set("Sec-Fetch-Site", "none")
			req.Header.Set("Sec-Fetch-User", "?1")
			req.Header.Set("Cache-Control", "max-age=0")
		}

		resp, err = e.client.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return types.ErrorResponse(fetchURL, types.EngineHTTP, err, time.Since(startTime)), err
			}
			continue
		}

		// If we get a server error (5xx), retry
		if resp.StatusCode >= 500 && attempt < maxRetries {
			resp.Body.Close()
			continue
		}

		// Success or non-retryable error, break out of retry loop
		break
	}
	defer resp.Body.Close()

	// Check for server errors and provide helpful messages
	if resp.StatusCode >= 500 {
		return types.ErrorResponse(fetchURL, types.EngineHTTP,
			fmt.Errorf("server error (status %d) after %d retries. try using engine='chrome'", resp.StatusCode, maxRetries),
			time.Since(startTime)), fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	if resp.StatusCode >= 400 {
		return types.ErrorResponse(fetchURL, types.EngineHTTP,
			fmt.Errorf("client error (status %d): %s", resp.StatusCode, resp.Status),
			time.Since(startTime)), fmt.Errorf("client error: %s", resp.Status)
	}

	// Read response body
	body, err := e.readResponseBody(resp, maxContentLength)
	if err != nil {
		return types.ErrorResponse(fetchURL, types.EngineHTTP, err, time.Since(startTime)), err
	}

	// Create response
	response := &types.FetchResponse{
		URL:             fetchURL,
		Engine:          types.EngineHTTP,
		StatusCode:      resp.StatusCode,
		ContentType:     resp.Header.Get("Content-Type"),
		Content:         string(body),
		Format:          types.FormatHTML, // Will be processed later
		FetchTimeMs:     time.Since(startTime).Milliseconds(),
		ChromeAvailable: false, // Will be set by main fetcher
	}

	return response, nil
}

// validateURL validates the URL and checks for security issues
func (e *HTTPEngine) validateURL(fetchURL string) error {
	parsedURL, err := url.Parse(fetchURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}

	// Check for local/private IPs if blocking is enabled
	if e.config.BlockLocal {
		host := parsedURL.Hostname()
		if isLocalOrPrivateIP(host) {
			return fmt.Errorf("access to local/private IP addresses is blocked")
		}
	}

	return nil
}

// readResponseBody reads the response body with size limits and decompression
func (e *HTTPEngine) readResponseBody(resp *http.Response, maxContentLength int) ([]byte, error) {
	var reader io.Reader = resp.Body

	// Handle gzip compression
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	// Read with size limit
	limitedReader := io.LimitReader(reader, int64(maxContentLength)+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check if content was truncated
	if len(body) > maxContentLength {
		return body[:maxContentLength], fmt.Errorf("content exceeds maximum length of %d bytes", maxContentLength)
	}

	return body, nil
}

// isLocalOrPrivateIP checks if the given host is a local or private IP
func isLocalOrPrivateIP(host string) bool {
	// Check for localhost variations
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Parse IP
	ip := net.ParseIP(host)
	if ip == nil {
		// Try to resolve hostname
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return false
		}
		ip = ips[0]
	}

	// Check for private IP ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // Link-local
		"fc00::/7",       // IPv6 private
		"fe80::/10",      // IPv6 link-local
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
