package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xincode-ai/xin-code/internal/tool"
)

// ServerConfig MCP 服务器配置
type ServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPTool MCP 发现的工具
type MCPTool struct {
	ServerName  string         `json:"server_name"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// Client MCP 客户端管理器
type Client struct {
	mu          sync.RWMutex
	servers     map[string]*StdioTransport
	tools       map[string]*MCPTool // 全局工具表: "mcp__<server>__<tool>" -> MCPTool
	configs     []ServerConfig
}

// NewClient 创建 MCP 客户端
func NewClient() *Client {
	return &Client{
		servers: make(map[string]*StdioTransport),
		tools:   make(map[string]*MCPTool),
	}
}

// LoadConfigs 从配置加载 MCP 服务器列表
func (c *Client) LoadConfigs(configs []ServerConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configs = configs
}

// ConnectAll 连接所有配置的服务器
func (c *Client) ConnectAll(ctx context.Context) error {
	c.mu.Lock()
	configs := make([]ServerConfig, len(c.configs))
	copy(configs, c.configs)
	c.mu.Unlock()

	for _, cfg := range configs {
		if err := c.Connect(ctx, cfg); err != nil {
			// 单个服务器连接失败不影响其他
			fmt.Printf("MCP 服务器 %s 连接失败: %v\n", cfg.Name, err)
		}
	}
	return nil
}

// Connect 连接单个 MCP 服务器
func (c *Client) Connect(ctx context.Context, cfg ServerConfig) error {
	transport := NewStdioTransport(cfg.Command, cfg.Args, cfg.Env)

	if err := transport.Start(ctx); err != nil {
		return fmt.Errorf("启动 MCP 服务器 %s 失败: %w", cfg.Name, err)
	}

	// 发送 initialize 请求
	initResult, err := transport.SendRequest(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "xin-code",
			"version": "0.1.0",
		},
	})
	if err != nil {
		transport.Close()
		return fmt.Errorf("MCP initialize 失败: %w", err)
	}

	_ = initResult // 可以解析服务器能力

	// 发送 initialized 通知
	if err := transport.SendNotification("initialized", nil); err != nil {
		transport.Close()
		return fmt.Errorf("MCP initialized 通知失败: %w", err)
	}

	c.mu.Lock()
	c.servers[cfg.Name] = transport
	c.mu.Unlock()

	// 发现工具
	if err := c.discoverTools(ctx, cfg.Name, transport); err != nil {
		fmt.Printf("MCP 工具发现失败 (%s): %v\n", cfg.Name, err)
	}

	return nil
}

// discoverTools 发现服务器的工具
func (c *Client) discoverTools(ctx context.Context, serverName string, transport *StdioTransport) error {
	result, err := transport.SendRequest(ctx, "tools/list", nil)
	if err != nil {
		return err
	}

	// 解析工具列表
	var toolsResult struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(resultBytes, &toolsResult); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, t := range toolsResult.Tools {
		key := fmt.Sprintf("mcp__%s__%s", serverName, t.Name)
		c.tools[key] = &MCPTool{
			ServerName:  serverName,
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	return nil
}

// CallTool 调用 MCP 工具
func (c *Client) CallTool(ctx context.Context, serverName, toolName string, args json.RawMessage) (string, error) {
	c.mu.RLock()
	transport, ok := c.servers[serverName]
	c.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("MCP 服务器 %s 未连接", serverName)
	}

	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return "", fmt.Errorf("解析工具参数失败: %w", err)
		}
	}

	result, err := transport.SendRequest(ctx, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": arguments,
	})
	if err != nil {
		return "", err
	}

	// 解析结果
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	var callResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resultBytes, &callResult); err != nil {
		// 如果解析失败，直接返回原始 JSON
		return string(resultBytes), nil
	}

	// 拼接文本内容
	var texts []string
	for _, c := range callResult.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}

	if callResult.IsError {
		return "", fmt.Errorf("MCP 工具错误: %s", joinTexts(texts))
	}

	return joinTexts(texts), nil
}

// RegisterToRegistry 将 MCP 工具注册到 Xin Code 工具注册表
func (c *Client) RegisterToRegistry(registry *tool.Registry) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for key, mcpTool := range c.tools {
		registry.Register(&mcpToolAdapter{
			client:     c,
			key:        key,
			mcpTool:    mcpTool,
		})
	}
}

// ToolCount 返回已发现的 MCP 工具数量
func (c *Client) ToolCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.tools)
}

// Close 关闭所有连接
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, transport := range c.servers {
		transport.Close()
	}
	c.servers = make(map[string]*StdioTransport)
	c.tools = make(map[string]*MCPTool)
}

func joinTexts(texts []string) string {
	if len(texts) == 0 {
		return ""
	}
	result := texts[0]
	for _, t := range texts[1:] {
		result += "\n" + t
	}
	return result
}
