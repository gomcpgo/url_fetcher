package fetcher

import (
	"fmt"
	"strings"

	"github.com/gomcpgo/url_fetcher/pkg/config"
	"github.com/gomcpgo/url_fetcher/pkg/types"
)

// Engine interface defines methods for fetching URLs
type Engine interface {
	Fetch(url string, maxContentLength int) (*types.FetchResponse, error)
}

// Fetcher manages URL fetching with multiple engines
type Fetcher struct {
	config       *config.Config
	httpEngine   *HTTPEngine
	chromeEngine *ChromeEngine
}

// NewFetcher creates a new fetcher instance
func NewFetcher(cfg *config.Config) *Fetcher {
	return &Fetcher{
		config:       cfg,
		httpEngine:   NewHTTPEngine(cfg),
		chromeEngine: NewChromeEngine(cfg),
	}
}

// Fetch retrieves content from a URL using the specified engine
func (f *Fetcher) Fetch(req *types.FetchRequest) (*types.FetchResponse, error) {
	// Set defaults
	if req.Engine == "" {
		req.Engine = types.DefaultEngine
	}
	if req.Format == "" {
		req.Format = types.DefaultFormat
	}
	if req.MaxContentLength == 0 {
		req.MaxContentLength = types.DefaultMaxContentLength
	}

	// Normalize engine name
	req.Engine = strings.ToLower(req.Engine)

	var response *types.FetchResponse
	var err error

	// Check Chrome availability
	chromeAvailable := f.chromeEngine.IsAvailable()

	// Select engine and fetch
	switch req.Engine {
	case types.EngineHTTP:
		response, err = f.httpEngine.Fetch(req.URL, req.MaxContentLength)

	case types.EngineChrome:
		if !chromeAvailable {
			// Fall back to HTTP with warning
			response, err = f.httpEngine.Fetch(req.URL, req.MaxContentLength)
			if response != nil {
				response.Engine = types.EngineHTTP
				response.Warnings = append(response.Warnings,
					"Chrome not available, falling back to HTTP engine")
			}
		} else {
			response, err = f.chromeEngine.Fetch(req.URL, req.MaxContentLength)
		}

	default:
		return nil, fmt.Errorf("unsupported engine: %s", req.Engine)
	}

	if err != nil {
		return response, err
	}

	// Set Chrome availability in response
	response.ChromeAvailable = chromeAvailable

	// Set the requested format (processing will be done by the processor)
	response.Format = req.Format

	return response, nil
}

// Close shuts down the fetcher and its engines
func (f *Fetcher) Close() {
	if f.chromeEngine != nil {
		f.chromeEngine.Close()
	}
}
