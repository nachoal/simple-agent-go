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

// WikipediaParams now uses generic input like Ruby
// The input string is the search query directly
type WikipediaParams = base.GenericParams

// WikipediaTool searches Wikipedia for information
type WikipediaTool struct {
	base.BaseTool
	client *http.Client
}

// Parameters returns the parameters struct
func (t *WikipediaTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute searches Wikipedia and returns the snippet of the most relevant article
func (t *WikipediaTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
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

	// Prepare the request
	baseURL := "https://en.wikipedia.org/w/api.php"
	urlParams := url.Values{}
	urlParams.Add("action", "query")
	urlParams.Add("list", "search")
	urlParams.Add("srsearch", query)
	urlParams.Add("format", "json")
	urlParams.Add("srlimit", "5") // Get top 5 results

	requestURL := fmt.Sprintf("%s?%s", baseURL, urlParams.Encode())

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return "", NewToolError("REQUEST_ERROR", "Failed to create request").
			WithDetail("error", err.Error())
	}

	// Execute request
	resp, err := t.client.Do(req)
	if err != nil {
		return "", NewToolError("HTTP_ERROR", "Failed to fetch Wikipedia data").
			WithDetail("error", err.Error())
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", NewToolError("READ_ERROR", "Failed to read response").
			WithDetail("error", err.Error())
	}

	// Parse response
	var result struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
				PageID  int    `json:"pageid"`
				Size    int    `json:"size"`
			} `json:"search"`
		} `json:"query"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", NewToolError("PARSE_ERROR", "Failed to parse Wikipedia response").
			WithDetail("error", err.Error())
	}

	// Check if we have results
	if len(result.Query.Search) == 0 {
		return fmt.Sprintf("No Wikipedia results found for query: %s", query), nil
	}

	// Format results
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Wikipedia search results for '%s':\n\n", query))

	for i, item := range result.Query.Search {
		if i > 0 {
			output.WriteString("\n---\n\n")
		}
		
		// Clean up the snippet (remove HTML tags)
		snippet := strings.ReplaceAll(item.Snippet, "<span class=\"searchmatch\">", "**")
		snippet = strings.ReplaceAll(snippet, "</span>", "**")
		snippet = strings.ReplaceAll(snippet, "&quot;", "\"")
		
		output.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, item.Title))
		output.WriteString(fmt.Sprintf("   %s\n", snippet))
		output.WriteString(fmt.Sprintf("   (Page ID: %d, Size: %d bytes)\n", item.PageID, item.Size))
		
		// For the first result, also fetch the page extract
		if i == 0 {
			extract, err := t.fetchPageExtract(ctx, item.PageID)
			if err == nil && extract != "" {
				output.WriteString(fmt.Sprintf("\n   **Extract:**\n   %s\n", extract))
			}
		}
	}

	return output.String(), nil
}

// fetchPageExtract gets the introduction extract for a specific page
func (t *WikipediaTool) fetchPageExtract(ctx context.Context, pageID int) (string, error) {
	baseURL := "https://en.wikipedia.org/w/api.php"
	urlParams := url.Values{}
	urlParams.Add("action", "query")
	urlParams.Add("pageids", fmt.Sprintf("%d", pageID))
	urlParams.Add("prop", "extracts")
	urlParams.Add("exintro", "true")
	urlParams.Add("explaintext", "true")
	urlParams.Add("exsentences", "3")
	urlParams.Add("format", "json")

	requestURL := fmt.Sprintf("%s?%s", baseURL, urlParams.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Query struct {
			Pages map[string]struct {
				Extract string `json:"extract"`
			} `json:"pages"`
		} `json:"query"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	// Get the extract from the first (and only) page
	for _, page := range result.Query.Pages {
		return strings.TrimSpace(page.Extract), nil
	}

	return "", nil
}