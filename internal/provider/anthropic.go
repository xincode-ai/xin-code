package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// generateSessionID 生成 CC 风格的会话 UUID
func generateSessionID() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// AnthropicProvider Claude API Provider
type AnthropicProvider struct {
	client *anthropic.Client
	model  string
}

// NewAnthropicProvider 创建 Anthropic Provider
// authSource 标识认证来源，"cc-oauth" 时使用 Bearer token + beta header
func NewAnthropicProvider(apiKey, model, baseURL, authSource string) (*AnthropicProvider, error) {
	var opts []option.RequestOption

	if authSource == "cc-oauth" {
		// CC OAuth 用户：完全模拟 Claude Code 的请求特征
		opts = append(opts,
			option.WithHeader("Authorization", "Bearer "+apiKey),
			option.WithHeader("anthropic-beta", "oauth-2025-04-20"),
			option.WithHeader("x-app", "cli"),
			option.WithHeader("User-Agent", "claude-code/2.1.88"),
			option.WithHeader("X-Claude-Code-Session-Id", generateSessionID()),
			option.WithAPIKey("placeholder"),
		)
	} else {
		opts = append(opts, option.WithAPIKey(apiKey))
	}

	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)
	return &AnthropicProvider{
		client: &client,
		model:  model,
	}, nil
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Capabilities() Capabilities {
	return Capabilities{
		Thinking:   strings.Contains(p.model, "opus") || strings.Contains(p.model, "sonnet"),
		Vision:     true,
		ToolUse:    true,
		Streaming:  true,
		MaxContext: 200000,
	}
}

func (p *AnthropicProvider) Stream(ctx context.Context, req *Request) (<-chan Event, error) {
	ch := make(chan Event, 64)

	// 转换消息格式
	messages := convertToAnthropicMessages(req.Messages)

	// 构建 API 参数
	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: int64(req.MaxTokens),
		Messages:  messages,
	}

	// 构建 system prompt（支持 cache_control 分块）
	sysBlocks := req.EffectiveSystemBlocks()
	if len(sysBlocks) > 0 {
		var systemParams []anthropic.TextBlockParam
		for i, block := range sysBlocks {
			tb := anthropic.TextBlockParam{Text: block.Text}
			// 为最后一个 block 或显式标记 ephemeral 的 block 添加 cache_control
			if i == len(sysBlocks)-1 || block.CacheControl == "ephemeral" {
				tb.CacheControl = anthropic.NewCacheControlEphemeralParam()
			}
			systemParams = append(systemParams, tb)
		}
		params.System = systemParams
	}

	// 转换工具定义
	if len(req.Tools) > 0 {
		params.Tools = convertToAnthropicTools(req.Tools)
	}

	go func() {
		defer close(ch)

		stream := p.client.Messages.NewStreaming(ctx, params)
		defer stream.Close()

		// 跟踪当前工具调用的 JSON 输入
		var currentToolCall *ToolCall
		var toolInputBuilder strings.Builder

		for stream.Next() {
			evt := stream.Current()

			switch variant := evt.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				// 内容块开始
				switch cb := variant.ContentBlock.AsAny().(type) {
				case anthropic.ToolUseBlock:
					currentToolCall = &ToolCall{
						ID:   cb.ID,
						Name: cb.Name,
					}
					toolInputBuilder.Reset()
				case anthropic.ThinkingBlock:
					ch <- Event{
						Type:     EventThinking,
						Thinking: &ThinkingBlock{Text: cb.Thinking},
					}
				}

			case anthropic.ContentBlockDeltaEvent:
				// 内容块增量
				switch delta := variant.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					ch <- Event{Type: EventTextDelta, Text: delta.Text}
				case anthropic.InputJSONDelta:
					// 累积工具输入 JSON
					toolInputBuilder.WriteString(delta.PartialJSON)
				case anthropic.ThinkingDelta:
					ch <- Event{
						Type:     EventThinking,
						Thinking: &ThinkingBlock{Text: delta.Thinking},
					}
				}

			case anthropic.ContentBlockStopEvent:
				// 内容块结束，如果有工具调用则发送
				if currentToolCall != nil {
					currentToolCall.Input = toolInputBuilder.String()
					ch <- Event{
						Type:     EventToolUse,
						ToolCall: currentToolCall,
					}
					currentToolCall = nil
					toolInputBuilder.Reset()
				}

			case anthropic.MessageDeltaEvent:
				// 消息结束，包含 usage 信息
				ch <- Event{
					Type: EventUsage,
					Usage: &Usage{
						InputTokens:              int(variant.Usage.InputTokens),
						OutputTokens:             int(variant.Usage.OutputTokens),
						CacheCreationInputTokens: int(variant.Usage.CacheCreationInputTokens),
						CacheReadInputTokens:     int(variant.Usage.CacheReadInputTokens),
					},
				}

			case anthropic.MessageStopEvent:
				// 不做额外处理，循环结束后发送 EventDone
			}
		}

		if err := stream.Err(); err != nil {
			ch <- Event{Type: EventError, Error: fmt.Errorf("anthropic stream error: %w", err)}
			return
		}

		ch <- Event{Type: EventDone}
	}()

	return ch, nil
}

func convertToAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	for _, msg := range msgs {
		var blocks []anthropic.ContentBlockParamUnion
		for _, block := range msg.Content {
			switch block.Type {
			case BlockText:
				blocks = append(blocks, anthropic.NewTextBlock(block.Text))
			case BlockToolUse:
				// 工具调用需要作为 assistant 消息的一部分传回
				if block.ToolCall != nil {
					// 解析 input JSON
					var input any
					if err := json.Unmarshal([]byte(block.ToolCall.Input), &input); err != nil {
						input = map[string]any{}
					}
					blocks = append(blocks, anthropic.NewToolUseBlock(
						block.ToolCall.ID,
						input,
						block.ToolCall.Name,
					))
				}
			case BlockToolResult:
				if block.ToolResult != nil {
					blocks = append(blocks, anthropic.NewToolResultBlock(
						block.ToolResult.ToolUseID,
						block.ToolResult.Content,
						block.ToolResult.IsError,
					))
				}
			}
		}
		if len(blocks) > 0 {
			result = append(result, anthropic.MessageParam{
				Role:    anthropic.MessageParamRole(msg.Role),
				Content: blocks,
			})
		}
	}
	return result
}

func convertToAnthropicTools(tools []ToolDef) []anthropic.ToolUnionParam {
	var result []anthropic.ToolUnionParam
	for _, t := range tools {
		// 从 map[string]any 提取 properties 和 required
		schema := anthropic.ToolInputSchemaParam{}
		if props, ok := t.InputSchema["properties"]; ok {
			schema.Properties = props
		}
		if req, ok := t.InputSchema["required"]; ok {
			if reqSlice, ok := req.([]string); ok {
				schema.Required = reqSlice
			} else if reqSlice, ok := req.([]any); ok {
				// 处理 []any 到 []string 的转换
				strs := make([]string, 0, len(reqSlice))
				for _, v := range reqSlice {
					if s, ok := v.(string); ok {
						strs = append(strs, s)
					}
				}
				schema.Required = strs
			}
		}

		tool := anthropic.ToolUnionParamOfTool(schema, t.Name)
		if tool.OfTool != nil && t.Description != "" {
			tool.OfTool.Description = anthropic.String(t.Description)
		}
		result = append(result, tool)
	}
	return result
}
