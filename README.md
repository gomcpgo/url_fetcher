# URL Fetcher MCP Server

An internal MCP server for Savant that provides web content fetching capabilities with support for both static and JavaScript-rendered content.

## Features

- **Dual Engine Architecture**:
  - **HTTP Engine**: Fast fetching for static content using standard HTTP
  - **Chrome Engine**: Full browser rendering for JavaScript-heavy sites

- **Multiple Output Formats**:
  - **Text**: Clean text extraction (default)
  - **HTML**: Cleaned HTML with dangerous elements removed
  - **Markdown**: Converted markdown format

- **Smart Features**:
  - In-memory caching with configurable TTL
  - Chrome browser pool for performance
  - Smart wait strategies for dynamic content
  - Security features (SSRF protection, content size limits)

## Configuration

Configure via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `FETCH_URL_BLOCK_LOCAL` | `true` | Block requests to local/private IPs |
| `FETCH_URL_CHROME_POOL_SIZE` | `3` | Number of Chrome instances in pool |
| `FETCH_URL_CACHE_TTL` | `3600` | Cache TTL in seconds (1 hour) |
| `FETCH_URL_TIMEOUT` | `30` | Request timeout in seconds |

## Usage

### Running the Server

```bash
# Build and run
./run.sh

# Run in test mode
./run.sh -test
```

### MCP Tool Interface

#### fetch_url

Fetches content from a URL with various options.

**Parameters:**
- `url` (required): URL to fetch
- `engine`: "http" (default) or "chrome"
- `format`: "text" (default), "html", or "markdown"
- `max_content_length`: Maximum content length in bytes (default: 10MB)

**Example Request:**
```json
{
  "tool": "fetch_url",
  "arguments": {
    "url": "https://example.com",
    "engine": "chrome",
    "format": "markdown"
  }
}
```

**Example Response:**
```json
{
  "url": "https://example.com",
  "engine": "chrome",
  "status_code": 200,
  "content_type": "text/html",
  "content": "# Example Domain\n\nThis domain is for use in illustrative examples...",
  "format": "markdown",
  "title": "Example Domain",
  "fetch_time_ms": 1234,
  "chrome_available": true
}
```

## Chrome Engine Notes

- Automatically detects Chrome/Chromium availability
- Falls back to HTTP engine if Chrome is not available
- Blocks unnecessary resources (images, fonts, CSS) for performance
- Uses smart wait strategy:
  - Waits for network idle (500ms)
  - Waits for DOM stability (500ms)
  - Maximum wait time: 15 seconds

## Security Features

- URL validation prevents SSRF attacks
- Configurable blocking of local/private IPs
- Content size limits (default 10MB)
- No cookie/session persistence
- Safe default headers

## Development

### Running Tests

```bash
go test ./test/... -v
```

### Project Structure

```
url_fetcher/
├── cmd/
│   └── main.go              # MCP server implementation
├── pkg/
│   ├── cache/               # In-memory caching
│   ├── config/              # Configuration management
│   ├── fetcher/             # HTTP and Chrome engines
│   ├── processor/           # Content processing (text, HTML, markdown)
│   └── types/               # Common types and constants
└── test/                    # Integration tests
```