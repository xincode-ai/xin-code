package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/xincode-ai/xin-code/internal/tool"
)

// WebSearchTool 网页搜索工具（DuckDuckGo HTML）
type WebSearchTool struct{}

type webSearchInput struct {
	Query string `json:"query"`
	Count int    `json:"count,omitempty"` // 返回结果数，默认 5
}

func (t *WebSearchTool) Name() string        { return "WebSearch" }
func (t *WebSearchTool) Description() string { return "搜索互联网，返回搜索结果摘要。使用 DuckDuckGo。" }
func (t *WebSearchTool) IsReadOnly() bool    { return true }
func (t *WebSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "搜索关键词"},
			"count": map[string]any{"type": "integer", "description": "返回结果数，默认 5"},
		},
		"required": []string{"query"},
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var in webSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	count := in.Count
	if count <= 0 {
		count = 5
	}

	// 使用 DuckDuckGo HTML 搜索
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(in.Query))

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("search error: %s", err), IsError: true}, nil
	}
	req.Header.Set("User-Agent", "XinCode/1.0 (CLI AI Agent)")

	resp, err := client.Do(req)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("search error: %s", err), IsError: true}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("read error: %s", err), IsError: true}, nil
	}

	results := parseDDGResults(string(body), count)
	if len(results) == 0 {
		return &tool.Result{Content: "未找到搜索结果"}, nil
	}

	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, r.title, r.url, r.snippet))
	}

	return &tool.Result{Content: sb.String()}, nil
}

type searchResult struct {
	title   string
	url     string
	snippet string
}

// parseDDGResults 从 DuckDuckGo HTML 结果页解析搜索结果
func parseDDGResults(html string, maxCount int) []searchResult {
	var results []searchResult

	// DuckDuckGo HTML 版本的结果在 class="result" 的 div 中
	// 标题在 <a class="result__a">，摘要在 <a class="result__snippet">
	reResult := regexp.MustCompile(`(?s)class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	reSnippet := regexp.MustCompile(`(?s)class="result__snippet"[^>]*>(.*?)</a>`)

	titleMatches := reResult.FindAllStringSubmatch(html, maxCount)
	snippetMatches := reSnippet.FindAllStringSubmatch(html, maxCount)

	for i, m := range titleMatches {
		if i >= maxCount {
			break
		}
		r := searchResult{
			title: cleanHTML(m[2]),
			url:   cleanDDGURL(m[1]),
		}
		if i < len(snippetMatches) {
			r.snippet = cleanHTML(snippetMatches[i][1])
		}
		results = append(results, r)
	}

	return results
}

func cleanHTML(s string) string {
	reTag := regexp.MustCompile(`<[^>]+>`)
	s = reTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return strings.TrimSpace(s)
}

func cleanDDGURL(rawURL string) string {
	// DuckDuckGo 的链接可能经过重定向包装
	if u, err := url.Parse(rawURL); err == nil {
		if redirect := u.Query().Get("uddg"); redirect != "" {
			return redirect
		}
	}
	return rawURL
}
