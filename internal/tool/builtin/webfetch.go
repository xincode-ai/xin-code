package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/xincode-ai/xin-code/internal/tool"
)

// 私有/保留 IP 段（SSRF 防护）
var privateNetworks = []net.IPNet{
	{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},     // 127.0.0.0/8
	{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},      // 10.0.0.0/8
	{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},   // 172.16.0.0/12
	{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},  // 192.168.0.0/16
	{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)},  // 169.254.0.0/16 (link-local + 云元数据)
	{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(8, 32)},       // 0.0.0.0/8
}

// isPrivateIP 检查 IP 是否为私有/保留地址
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, network := range privateNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// validateURL 校验 URL，拒绝私有 IP 和危险协议
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	// 只允许 http/https
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme: %s (only http/https allowed)", u.Scheme)
	}

	// 解析 hostname
	hostname := u.Hostname()
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("access denied: %s resolves to private IP %s", hostname, ip)
		}
	}

	return nil
}

// WebFetchTool 网页抓取工具
type WebFetchTool struct{}

type webFetchInput struct {
	URL     string `json:"url"`
	Timeout int    `json:"timeout,omitempty"` // 秒
}

func (t *WebFetchTool) Name() string        { return "WebFetch" }
func (t *WebFetchTool) Description() string { return "抓取网页内容，返回纯文本正文。" }
func (t *WebFetchTool) IsReadOnly() bool    { return true }
func (t *WebFetchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":     map[string]any{"type": "string", "description": "要抓取的 URL"},
			"timeout": map[string]any{"type": "integer", "description": "超时秒数，默认 30"},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// SSRF 防护：校验 URL
	if err := validateURL(in.URL); err != nil {
		return &tool.Result{Content: fmt.Sprintf("url validation failed: %s", err), IsError: true}, nil
	}

	timeout := 30 * time.Second
	if in.Timeout > 0 {
		timeout = time.Duration(in.Timeout) * time.Second
	}

	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("invalid url: %s", err), IsError: true}, nil
	}
	req.Header.Set("User-Agent", "XinCode/1.0 (CLI AI Agent)")

	resp, err := client.Do(req)
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("fetch error: %s", err), IsError: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &tool.Result{
			Content: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status),
			IsError: true,
		}, nil
	}

	// 限制读取大小
	const maxBody = 512 * 1024 // 512KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return &tool.Result{Content: fmt.Sprintf("read error: %s", err), IsError: true}, nil
	}

	content := string(body)

	// 如果是 HTML，提取正文
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		content = extractHTMLText(content)
	}

	// 限制输出
	const maxOutput = 50 * 1024
	if len(content) > maxOutput {
		content = content[:maxOutput] + "\n... (content truncated)"
	}

	return &tool.Result{Content: content}, nil
}

// extractHTMLText 简单的 HTML 正文提取（去标签、去脚本/样式）
func extractHTMLText(html string) string {
	// 移除 script 和 style
	reScript := regexp.MustCompile(`(?is)<script.*?</script>`)
	html = reScript.ReplaceAllString(html, "")
	reStyle := regexp.MustCompile(`(?is)<style.*?</style>`)
	html = reStyle.ReplaceAllString(html, "")

	// 移除 HTML 注释
	reComment := regexp.MustCompile(`(?s)<!--.*?-->`)
	html = reComment.ReplaceAllString(html, "")

	// 替换常见块级标签为换行
	reBlock := regexp.MustCompile(`(?i)</(p|div|br|h[1-6]|li|tr)>`)
	html = reBlock.ReplaceAllString(html, "\n")
	reBR := regexp.MustCompile(`(?i)<br\s*/?>`)
	html = reBR.ReplaceAllString(html, "\n")

	// 移除所有标签
	reTag := regexp.MustCompile(`<[^>]+>`)
	html = reTag.ReplaceAllString(html, "")

	// HTML 实体解码（常见的几个）
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// 压缩空白
	lines := strings.Split(html, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
