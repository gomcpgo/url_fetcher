package processor

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
	"github.com/gomcpgo/url_fetcher/pkg/types"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// Processor handles content processing for different formats
type Processor struct {
	policy *bluemonday.Policy
}

// NewProcessor creates a new content processor
func NewProcessor() *Processor {
	// Create a strict policy that removes all HTML
	policy := bluemonday.StrictPolicy()

	return &Processor{
		policy: policy,
	}
}

// Process converts content to the requested format
func (p *Processor) Process(response *types.FetchResponse) error {
	// Extract title first if not already set
	if response.Title == "" {
		response.Title = p.extractTitle(response.Content)
	}

	switch response.Format {
	case types.FormatText:
		text, err := p.extractText(response.Content, response.URL)
		if err != nil {
			return fmt.Errorf("failed to extract text: %w", err)
		}
		response.Content = text

	case types.FormatHTML:
		// Clean HTML but keep structure
		cleaned := p.cleanHTML(response.Content)
		response.Content = cleaned

	case types.FormatMarkdown:
		// First extract readable content, then convert to markdown
		markdown, err := p.convertToMarkdown(response.Content, response.URL)
		if err != nil {
			return fmt.Errorf("failed to convert to markdown: %w", err)
		}
		response.Content = markdown

	default:
		return fmt.Errorf("unsupported format: %s", response.Format)
	}

	return nil
}

// extractTitle extracts the title from HTML content
func (p *Processor) extractTitle(htmlContent string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	// Try to get title from <title> tag
	title := doc.Find("title").First().Text()
	return strings.TrimSpace(title)
}

// extractText extracts clean text from HTML using go-readability
func (p *Processor) extractText(htmlContent, urlStr string) (string, error) {
	// Parse URL for readability
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		// If URL parsing fails, use simple extraction
		return p.simpleTextExtraction(htmlContent), nil
	}

	// Use go-readability for better content extraction
	article, err := readability.FromReader(strings.NewReader(htmlContent), parsedURL)
	if err != nil {
		// Fallback to simple text extraction
		return p.simpleTextExtraction(htmlContent), nil
	}

	// Get the text content
	text := article.TextContent

	// Clean up whitespace
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.Join(cleanedLines, "\n\n"), nil
}

// simpleTextExtraction performs basic text extraction from HTML
func (p *Processor) simpleTextExtraction(htmlContent string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		// Last resort: strip all HTML tags
		return p.policy.Sanitize(htmlContent)
	}

	// Remove script and style elements
	doc.Find("script, style, noscript, iframe, svg").Remove()

	// Get text content
	text := doc.Text()

	// Clean up whitespace
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.Join(cleanedLines, "\n")
}

// cleanHTML removes dangerous elements but preserves structure
func (p *Processor) cleanHTML(htmlContent string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	// Remove unwanted elements
	doc.Find("script, style, noscript, iframe, object, embed, applet").Remove()

	// Remove all attributes except href and src
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		node := s.Get(0)
		if node.Type == html.ElementNode {
			var newAttrs []html.Attribute
			for _, attr := range node.Attr {
				if attr.Key == "href" || attr.Key == "src" {
					newAttrs = append(newAttrs, attr)
				}
			}
			node.Attr = newAttrs
		}
	})

	// Get cleaned HTML
	result, err := doc.Html()
	if err != nil {
		return htmlContent
	}

	return result
}

// convertToMarkdown converts HTML to Markdown format
func (p *Processor) convertToMarkdown(htmlContent, urlStr string) (string, error) {
	// Parse URL for readability
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		// If URL parsing fails, convert the original HTML
		return p.htmlToMarkdown(htmlContent), nil
	}

	// First, try to extract the main content using readability
	article, err := readability.FromReader(strings.NewReader(htmlContent), parsedURL)
	if err != nil {
		// If readability fails, use the original HTML
		return p.htmlToMarkdown(htmlContent), nil
	}

	// Convert the extracted content to markdown
	return p.htmlToMarkdown(article.Content), nil
}

// htmlToMarkdown converts HTML to Markdown using a simple approach
func (p *Processor) htmlToMarkdown(htmlContent string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	var markdown strings.Builder

	// Process the document
	p.processNode(doc.Selection, &markdown, 0)

	// Clean up excessive newlines
	result := markdown.String()
	result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	result = strings.TrimSpace(result)

	return result
}

// processNode recursively processes HTML nodes to generate Markdown
func (p *Processor) processNode(s *goquery.Selection, markdown *strings.Builder, listDepth int) {
	s.Contents().Each(func(i int, sel *goquery.Selection) {
		node := sel.Get(0)

		if node.Type == html.TextNode {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				markdown.WriteString(text)
			}
		} else if node.Type == html.ElementNode {
			switch node.Data {
			case "h1":
				markdown.WriteString("\n\n# ")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n\n")
			case "h2":
				markdown.WriteString("\n\n## ")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n\n")
			case "h3":
				markdown.WriteString("\n\n### ")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n\n")
			case "h4":
				markdown.WriteString("\n\n#### ")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n\n")
			case "h5":
				markdown.WriteString("\n\n##### ")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n\n")
			case "h6":
				markdown.WriteString("\n\n###### ")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n\n")
			case "p":
				markdown.WriteString("\n\n")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n\n")
			case "br":
				markdown.WriteString("\n")
			case "strong", "b":
				markdown.WriteString("**")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("**")
			case "em", "i":
				markdown.WriteString("*")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("*")
			case "code":
				markdown.WriteString("`")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("`")
			case "pre":
				markdown.WriteString("\n\n```\n")
				p.processNode(sel, markdown, listDepth)
				markdown.WriteString("\n```\n\n")
			case "a":
				href, exists := sel.Attr("href")
				if exists && href != "" {
					markdown.WriteString("[")
					p.processNode(sel, markdown, listDepth)
					markdown.WriteString("](")
					markdown.WriteString(href)
					markdown.WriteString(")")
				} else {
					p.processNode(sel, markdown, listDepth)
				}
			case "ul":
				markdown.WriteString("\n")
				p.processNode(sel, markdown, listDepth+1)
			case "ol":
				markdown.WriteString("\n")
				p.processNode(sel, markdown, listDepth+1)
			case "li":
				markdown.WriteString("\n")
				for i := 0; i < listDepth; i++ {
					markdown.WriteString("  ")
				}
				parent := sel.Parent()
				if parent.Is("ol") {
					markdown.WriteString("1. ")
				} else {
					markdown.WriteString("- ")
				}
				p.processNode(sel, markdown, listDepth)
			case "blockquote":
				lines := strings.Split(sel.Text(), "\n")
				for _, line := range lines {
					if strings.TrimSpace(line) != "" {
						markdown.WriteString("\n> ")
						markdown.WriteString(strings.TrimSpace(line))
					}
				}
				markdown.WriteString("\n")
			case "hr":
				markdown.WriteString("\n\n---\n\n")
			case "img":
				alt, _ := sel.Attr("alt")
				src, exists := sel.Attr("src")
				if exists && src != "" {
					markdown.WriteString("![")
					markdown.WriteString(alt)
					markdown.WriteString("](")
					markdown.WriteString(src)
					markdown.WriteString(")")
				}
			default:
				// For other elements, just process their children
				p.processNode(sel, markdown, listDepth)
			}
		}
	})
}
