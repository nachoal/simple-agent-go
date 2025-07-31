package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// GoogleSearchParams now uses generic input like Ruby
// The input string is the search query directly
type GoogleSearchParams = base.GenericParams

// GoogleSearchTool performs Google searches using the Custom Search API
type GoogleSearchTool struct {
	base.BaseTool
	client         *http.Client
	apiKey         string
	searchEngineID string
}

// Parameters returns the parameters struct
func (t *GoogleSearchTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute performs a Google search and returns formatted results
func (t *GoogleSearchTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args base.GenericParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// In Ruby style, the input is the query directly
	query := strings.TrimSpace(args.Input)
	if query == "" {
		return "", NewToolError("VALIDATION_FAILED", "Query cannot be empty")
	}

	// Check if API credentials are configured
	if t.apiKey == "" || t.searchEngineID == "" {
		return "", NewToolError("NOT_CONFIGURED", "Google Search API credentials not configured").
			WithDetail("help", "Set GOOGLE_API_KEY and GOOGLE_CX environment variables")
	}

	// Default to 10 results (Ruby behavior)
	num := 10

	// Prepare the request
	baseURL := "https://www.googleapis.com/customsearch/v1"
	queryParams := url.Values{}
	queryParams.Add("key", t.apiKey)
	queryParams.Add("cx", t.searchEngineID)
	queryParams.Add("q", query)
	queryParams.Add("num", fmt.Sprintf("%d", num))

	requestURL := fmt.Sprintf("%s?%s", baseURL, queryParams.Encode())

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return "", NewToolError("REQUEST_ERROR", "Failed to create request").
			WithDetail("error", err.Error())
	}

	// Execute request
	resp, err := t.client.Do(req)
	if err != nil {
		return "", NewToolError("HTTP_ERROR", "Failed to perform Google search").
			WithDetail("error", err.Error())
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", NewToolError("API_ERROR", fmt.Sprintf("Google API returned status %d", resp.StatusCode)).
			WithDetail("response", string(body))
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", NewToolError("READ_ERROR", "Failed to read response").
			WithDetail("error", err.Error())
	}

	// Parse response
	var result struct {
		SearchInformation struct {
			FormattedTotalResults string `json:"formattedTotalResults"`
			FormattedSearchTime   string `json:"formattedSearchTime"`
		} `json:"searchInformation"`
		Items []struct {
			Title       string `json:"title"`
			Link        string `json:"link"`
			DisplayLink string `json:"displayLink"`
			Snippet     string `json:"snippet"`
			FileFormat  string `json:"fileFormat,omitempty"`
			Pagemap     struct {
				Metatags []map[string]string `json:"metatags,omitempty"`
			} `json:"pagemap,omitempty"`
		} `json:"items"`
		Error struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", NewToolError("PARSE_ERROR", "Failed to parse Google search response").
			WithDetail("error", err.Error())
	}

	// Check for API errors
	if result.Error.Message != "" {
		return "", NewToolError("API_ERROR", "Google API error").
			WithDetail("message", result.Error.Message)
	}

	// Check if we have results
	if len(result.Items) == 0 {
		return fmt.Sprintf("No results found for query: %s", query), nil
	}

	// Format results
	var output strings.Builder
	
	// Header with search information
	output.WriteString(fmt.Sprintf("Found %s results in %s seconds\n\n",
		result.SearchInformation.FormattedTotalResults,
		result.SearchInformation.FormattedSearchTime))

	// Format each result
	for i, item := range result.Items {
		output.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, item.Title))
		output.WriteString(fmt.Sprintf("   URL: %s\n", item.Link))
		output.WriteString(fmt.Sprintf("   Description: %s\n", item.Snippet))
		
		// Add optional fields if present
		if item.FileFormat != "" {
			output.WriteString(fmt.Sprintf("   File Format: %s\n", item.FileFormat))
		}
		if item.DisplayLink != "" {
			output.WriteString(fmt.Sprintf("   Site Name: %s\n", item.DisplayLink))
		}
		
		// Add meta description if available
		if len(item.Pagemap.Metatags) > 0 {
			metatags := item.Pagemap.Metatags[0]
			if desc, ok := metatags["og:description"]; ok && desc != "" {
				output.WriteString(fmt.Sprintf("   Description (meta): %s\n", desc))
			}
		}
		
		if i < len(result.Items)-1 {
			output.WriteString("\n")
		}
	}

	return output.String(), nil
}