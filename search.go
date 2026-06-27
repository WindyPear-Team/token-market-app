package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	webSearchTimeout     = 20 * time.Second
	webSearchMaxResponse = 2 << 20
	webSearchUserAgent   = "WindyPearConnector/0.1 (+https://windypear.ai)"
)

var (
	duckHTMLResultRe  = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*\bresult__a\b[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	duckHTMLSnippetRe = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*\bresult__snippet\b[^"]*"[^>]*>(.*?)</a>`)
	htmlTagRe         = regexp.MustCompile(`(?is)<[^>]+>`)
	htmlWhitespaceRe  = regexp.MustCompile(`\s+`)
)

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func webSearch(query string, maxResults int, language string, region string, timeRange string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", errors.New("query is required")
	}
	maxResults = clampInt(maxResults, 1, 10, 5)
	results, err := duckDuckGoInstantSearch(query, maxResults, language, region)
	if err == nil && len(results) > 0 {
		return formatSearchResults(query, results, err), nil
	}
	htmlResults, htmlErr := duckDuckGoHTMLSearch(query, maxResults, language, region, timeRange)
	if len(htmlResults) > 0 {
		return formatSearchResults(query, htmlResults, htmlErr), nil
	}
	if htmlErr != nil {
		return "", htmlErr
	}
	if err == nil {
		return "No search results found.", nil
	}
	return "", err
}

func duckDuckGoInstantSearch(query string, maxResults int, language string, region string) ([]searchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("no_html", "1")
	params.Set("skip_disambig", "1")
	if language = strings.TrimSpace(language); language != "" {
		params.Set("kl", language)
	}
	if region = strings.TrimSpace(region); region != "" {
		params.Set("kl", region)
	}
	raw, err := fetchSearchURL("https://api.duckduckgo.com/?" + params.Encode())
	if err != nil {
		return nil, err
	}
	var payload struct {
		Abstract       string `json:"AbstractText"`
		AbstractSource string `json:"AbstractSource"`
		AbstractURL    string `json:"AbstractURL"`
		Heading        string `json:"Heading"`
		Results        []struct {
			FirstURL string `json:"FirstURL"`
			Text     string `json:"Text"`
		} `json:"Results"`
		RelatedTopics []json.RawMessage `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	results := []searchResult{}
	if payload.AbstractURL != "" && payload.Abstract != "" {
		title := payload.Heading
		if title == "" {
			title = payload.AbstractSource
		}
		results = append(results, searchResult{Title: title, URL: payload.AbstractURL, Snippet: payload.Abstract})
	}
	for _, result := range payload.Results {
		results = append(results, searchResultFromDuckText(result.Text, result.FirstURL))
	}
	for _, rawTopic := range payload.RelatedTopics {
		collectDuckRelatedTopic(rawTopic, &results, maxResults)
		if len(results) >= maxResults {
			break
		}
	}
	return dedupeSearchResults(results, maxResults), nil
}

func collectDuckRelatedTopic(raw json.RawMessage, results *[]searchResult, maxResults int) {
	if len(*results) >= maxResults {
		return
	}
	var topic struct {
		FirstURL string            `json:"FirstURL"`
		Text     string            `json:"Text"`
		Topics   []json.RawMessage `json:"Topics"`
	}
	if err := json.Unmarshal(raw, &topic); err != nil {
		return
	}
	if topic.FirstURL != "" && topic.Text != "" {
		*results = append(*results, searchResultFromDuckText(topic.Text, topic.FirstURL))
	}
	for _, nested := range topic.Topics {
		collectDuckRelatedTopic(nested, results, maxResults)
		if len(*results) >= maxResults {
			return
		}
	}
}

func searchResultFromDuckText(text string, rawURL string) searchResult {
	title, snippet, _ := strings.Cut(strings.TrimSpace(text), " - ")
	if strings.TrimSpace(snippet) == "" {
		snippet = title
	}
	return searchResult{Title: title, URL: rawURL, Snippet: snippet}
}

func duckDuckGoHTMLSearch(query string, maxResults int, language string, region string, timeRange string) ([]searchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	if kl := duckDuckGoLocale(language, region); kl != "" {
		params.Set("kl", kl)
	}
	if df := duckDuckGoTimeRange(timeRange); df != "" {
		params.Set("df", df)
	}
	raw, err := fetchSearchURL("https://duckduckgo.com/html/?" + params.Encode())
	if err != nil {
		return nil, err
	}
	body := string(raw)
	matches := duckHTMLResultRe.FindAllStringSubmatch(body, maxResults)
	snippets := duckHTMLSnippetRe.FindAllStringSubmatch(body, maxResults)
	results := make([]searchResult, 0, len(matches))
	for index, match := range matches {
		result := searchResult{
			Title: cleanHTMLText(match[2]),
			URL:   normalizeDuckDuckGoResultURL(match[1]),
		}
		if index < len(snippets) {
			result.Snippet = cleanHTMLText(snippets[index][1])
		}
		results = append(results, result)
	}
	return dedupeSearchResults(results, maxResults), nil
}

func fetchSearchURL(rawURL string) ([]byte, error) {
	client := http.Client{Timeout: webSearchTimeout}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json,text/html;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", webSearchUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("search provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(preview)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, webSearchMaxResponse))
}

func duckDuckGoLocale(language string, region string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	region = strings.ToLower(strings.TrimSpace(region))
	if language == "" && region == "" {
		return ""
	}
	if strings.Contains(language, "-") {
		return language
	}
	if language == "" {
		language = "wt-wt"
	}
	if region == "" || language == "wt-wt" {
		return language
	}
	return region + "-" + language
}

func duckDuckGoTimeRange(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "day", "d":
		return "d"
	case "week", "w":
		return "w"
	case "month", "m":
		return "m"
	case "year", "y":
		return "y"
	default:
		return ""
	}
}

func normalizeDuckDuckGoResultURL(raw string) string {
	raw = html.UnescapeString(strings.TrimSpace(raw))
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.Host == "duckduckgo.com" && parsed.Path == "/l/" {
		if target := parsed.Query().Get("uddg"); target != "" {
			return target
		}
	}
	return raw
}

func cleanHTMLText(value string) string {
	value = htmlTagRe.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	value = htmlWhitespaceRe.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func dedupeSearchResults(results []searchResult, maxResults int) []searchResult {
	filtered := make([]searchResult, 0, len(results))
	seen := map[string]bool{}
	for _, result := range results {
		result.Title = truncateRunes(strings.TrimSpace(result.Title), 160)
		result.URL = strings.TrimSpace(result.URL)
		result.Snippet = truncateRunes(strings.TrimSpace(result.Snippet), 500)
		if result.URL == "" || result.Title == "" || seen[result.URL] {
			continue
		}
		seen[result.URL] = true
		filtered = append(filtered, result)
		if len(filtered) >= maxResults {
			break
		}
	}
	return filtered
}

func formatSearchResults(query string, results []searchResult, providerErr error) string {
	if len(results) == 0 {
		if providerErr != nil {
			return "No search results found.\n\nProvider error: " + providerErr.Error()
		}
		return "No search results found."
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "Search results for %q:\n", query)
	for index, result := range results {
		fmt.Fprintf(&builder, "\n%d. %s\n%s", index+1, result.Title, result.URL)
		if result.Snippet != "" {
			fmt.Fprintf(&builder, "\n%s", result.Snippet)
		}
		builder.WriteString("\n")
	}
	if providerErr != nil {
		fmt.Fprintf(&builder, "\nNote: fallback search was used after provider error: %s\n", providerErr.Error())
	}
	return strings.TrimSpace(builder.String())
}

func clampInt(value int, min int, max int, fallback int) int {
	if value == 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}
