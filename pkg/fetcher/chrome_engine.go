package fetcher

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/gomcpgo/url_fetcher/pkg/config"
	"github.com/gomcpgo/url_fetcher/pkg/types"
)

// ChromeEngine handles Chrome-based URL fetching with a browser pool
type ChromeEngine struct {
	config      *config.Config
	pool        *BrowserPool
	isAvailable bool
}

// NewChromeEngine creates a new Chrome engine. No Chrome process is started
// here: browsers are launched by the pool on the first Chrome fetch.
func NewChromeEngine(cfg *config.Config) *ChromeEngine {
	engine := &ChromeEngine{
		config:      cfg,
		isAvailable: checkChromeAvailable(),
	}

	if engine.isAvailable {
		engine.pool = newBrowserPool(cfg.ChromePoolSize, cfg.ChromeIdleTimeout)
	}

	// Clean up after earlier processes that died without shutting down.
	go sweepStaleProfiles()

	return engine
}

// IsAvailable returns whether Chrome is available on the system
func (e *ChromeEngine) IsAvailable() bool {
	return e.isAvailable
}

// Fetch retrieves content from a URL using Chrome
func (e *ChromeEngine) Fetch(fetchURL string, maxContentLength int) (*types.FetchResponse, error) {
	startTime := time.Now()

	if !e.isAvailable {
		return nil, fmt.Errorf("Chrome is not available on this system")
	}

	// Get a browser from the pool, launching Chrome if none is idle
	b, err := e.pool.acquire()
	if err != nil {
		return types.ErrorResponse(fetchURL, types.EngineChrome, err, time.Since(startTime)), err
	}
	defer e.pool.release(b)

	// Create a new tab context with timeout
	tabCtx, cancel := chromedp.NewContext(b.ctx)
	defer cancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(tabCtx, e.config.Timeout)
	defer timeoutCancel()

	var htmlContent string
	var statusCode int64
	contentType := "text/html"

	// Set up network monitoring
	chromedp.ListenTarget(timeoutCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:
			if ev.Type == network.ResourceTypeDocument {
				statusCode = ev.Response.Status
				if ct, ok := ev.Response.Headers["content-type"].(string); ok {
					contentType = ct
				}
			}
		}
	})

	// Navigate and wait with smart strategy
	err = chromedp.Run(timeoutCtx,
		// Enable network events
		network.Enable(),

		// Set up request interception to block resources
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable fetch domain
			if err := network.Enable().Do(ctx); err != nil {
				return err
			}

			// Note: Request interception patterns were removed as they're not available
			// in the current chromedp version. Resource blocking is handled by
			// browser flags instead.

			// Use SetCacheDisabled to improve performance
			return network.SetCacheDisabled(true).Do(ctx)
		}),

		// Navigate to URL
		chromedp.Navigate(fetchURL),

		// Smart wait strategy
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Wait for initial page load
			if err := chromedp.WaitReady("body", chromedp.ByQuery).Do(ctx); err != nil {
				return err
			}

			// Smart wait: monitor network and DOM changes
			return waitForPageStability(ctx, 15*time.Second)
		}),

		// Get the HTML content
		chromedp.OuterHTML("html", &htmlContent),
	)

	if err != nil {
		return types.ErrorResponse(fetchURL, types.EngineChrome, err, time.Since(startTime)), err
	}

	// Truncate content if needed
	if len(htmlContent) > maxContentLength {
		htmlContent = htmlContent[:maxContentLength]
	}

	response := &types.FetchResponse{
		URL:             fetchURL,
		Engine:          types.EngineChrome,
		StatusCode:      int(statusCode),
		ContentType:     contentType,
		Content:         htmlContent,
		Format:          types.FormatHTML, // Will be processed later
		FetchTimeMs:     time.Since(startTime).Milliseconds(),
		ChromeAvailable: true,
	}

	return response, nil
}

// Close kills every Chrome instance and removes their profile directories.
// It is safe to call more than once.
func (e *ChromeEngine) Close() {
	if e.pool != nil {
		e.pool.Close()
	}
}

// checkChromeAvailable checks if Chrome/Chromium is available on the system
func checkChromeAvailable() bool {
	// Check common Chrome/Chromium paths
	chromePaths := []string{
		"google-chrome",
		"chromium",
		"chromium-browser",
		"chrome",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}

	for _, path := range chromePaths {
		if _, err := exec.LookPath(path); err == nil {
			return true
		}
	}

	// Try to execute chrome with version flag
	cmd := exec.Command("google-chrome", "--version")
	if err := cmd.Run(); err == nil {
		return true
	}

	cmd = exec.Command("chromium", "--version")
	if err := cmd.Run(); err == nil {
		return true
	}

	return false
}

// waitForPageStability implements smart wait strategy
func waitForPageStability(ctx context.Context, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	networkIdleTime := 500 * time.Millisecond
	domStableTime := 500 * time.Millisecond

	var lastNetworkActivity time.Time
	networkIdle := false
	domStable := false

	// Monitor network activity
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev.(type) {
		case *network.EventRequestWillBeSent,
			*network.EventResponseReceived,
			*network.EventLoadingFinished:
			lastNetworkActivity = time.Now()
			networkIdle = false
		}
	})

	// Check for stability
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			now := time.Now()

			// Check if we've exceeded max wait time
			if now.After(deadline) {
				return nil
			}

			// Check network idle
			if !networkIdle && now.Sub(lastNetworkActivity) > networkIdleTime {
				networkIdle = true
			}

			// For simplicity, assume DOM is stable after network is idle
			// In a more sophisticated implementation, we would monitor DOM mutations
			if networkIdle && now.Sub(lastNetworkActivity) > domStableTime {
				domStable = true
			}

			// If both network and DOM are stable, we're done
			if networkIdle && domStable {
				// Wait a bit more to be sure
				time.Sleep(200 * time.Millisecond)
				return nil
			}
		}
	}
}
