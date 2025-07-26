package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the URL Fetcher MCP server
type Config struct {
	// BlockLocal controls whether to block requests to local/private IPs
	BlockLocal bool
	
	// ChromePoolSize is the number of Chrome instances to keep in the pool
	ChromePoolSize int
	
	// CacheTTL is the time-to-live for cached responses in seconds
	CacheTTL time.Duration
	
	// Timeout is the request timeout in seconds
	Timeout time.Duration
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() (*Config, error) {
	cfg := &Config{
		BlockLocal:     true,
		ChromePoolSize: 3,
		CacheTTL:       time.Hour,
		Timeout:        30 * time.Second,
	}
	
	// FETCH_URL_BLOCK_LOCAL
	if val := os.Getenv("FETCH_URL_BLOCK_LOCAL"); val != "" {
		blockLocal, err := strconv.ParseBool(val)
		if err != nil {
			return nil, fmt.Errorf("invalid FETCH_URL_BLOCK_LOCAL value: %s", val)
		}
		cfg.BlockLocal = blockLocal
	}
	
	// FETCH_URL_CHROME_POOL_SIZE
	if val := os.Getenv("FETCH_URL_CHROME_POOL_SIZE"); val != "" {
		poolSize, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("invalid FETCH_URL_CHROME_POOL_SIZE value: %s", val)
		}
		if poolSize < 1 || poolSize > 10 {
			return nil, fmt.Errorf("FETCH_URL_CHROME_POOL_SIZE must be between 1 and 10")
		}
		cfg.ChromePoolSize = poolSize
	}
	
	// FETCH_URL_CACHE_TTL
	if val := os.Getenv("FETCH_URL_CACHE_TTL"); val != "" {
		ttlSeconds, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("invalid FETCH_URL_CACHE_TTL value: %s", val)
		}
		if ttlSeconds < 0 {
			return nil, fmt.Errorf("FETCH_URL_CACHE_TTL must be non-negative")
		}
		cfg.CacheTTL = time.Duration(ttlSeconds) * time.Second
	}
	
	// FETCH_URL_TIMEOUT
	if val := os.Getenv("FETCH_URL_TIMEOUT"); val != "" {
		timeoutSeconds, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("invalid FETCH_URL_TIMEOUT value: %s", val)
		}
		if timeoutSeconds < 1 || timeoutSeconds > 300 {
			return nil, fmt.Errorf("FETCH_URL_TIMEOUT must be between 1 and 300 seconds")
		}
		cfg.Timeout = time.Duration(timeoutSeconds) * time.Second
	}
	
	return cfg, nil
}