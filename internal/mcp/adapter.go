package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xincode-ai/xin-code/internal/tool"
)

// mcpToolAdapter 将 MCP 工具适配为 Xin Code Tool 接口
type mcpToolAdapter struct {
	client  *Client
	key     string   // "mcp__<server>__<tool>"
	mcpTool *MCPTool
}

func (a *mcpToolAdapter) Name() string {
	return a.key
}

func (a *mcpToolAdapter) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", a.mcpTool.ServerName, a.mcpTool.Description)
}

func (a *mcpToolAdapter) InputSchema() map[string]any {
	if a.mcpTool.InputSchema != nil {
		return a.mcpTool.InputSchema
	}
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (a *mcpToolAdapter) IsReadOnly() bool {
	// MCP 工具默认需要权限确认
	return false
}

func (a *mcpToolAdapter) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	// 从 key 中提取 server 和 tool 名称
	parts := strings.SplitN(a.key, "__", 3)
	if len(parts) != 3 {
		return &tool.Result{Content: "invalid MCP tool key: " + a.key, IsError: true}, nil
	}

	serverName := parts[1]
	toolName := parts[2]

	result, err := a.client.CallTool(ctx, serverName, toolName, input)
	if err != nil {
		return &tool.Result{Content: err.Error(), IsError: true}, nil
	}

	return &tool.Result{Content: result}, nil
}
