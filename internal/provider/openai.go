package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// OpenAIProvider OpenAI API Provider（兼容 OpenRouter / 任意兼容端点）
type OpenAIProvider struct {
	client *openai.Client
	model  string
}

// NewOpenAIProvider 创建 OpenAI Provider
func NewOpenAIProvider(apiKey, model, baseURL string) (*OpenAIProvider, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)
	return &OpenAIProvider{
		client: &client,
		model:  model,
	}, nil
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Capabilities() Capabilities {
	maxCtx := 128000
	// o 系列模型上下文更大
	if strings.HasPrefix(p.model, "o1") || strings.HasPrefix(p.model, "o3") || strings.HasPrefix(p.model, "o4") {
		maxCtx = 200000
	}
	return Capabilities{
		Thinking:   false,
		Vision:     true,
		ToolUse:    true,
		Streaming:  true,
		MaxContext: maxCtx,
	}
}

func (p *OpenAIProvider) Stream(ctx context.Context, req *Request) (<-chan Event, error) {
	ch := make(chan Event, 64)

	// 将 SystemBlocks 拼接为单个 system prompt（OpenAI 不支持 cache_control）
	systemPrompt := req.System
	if len(req.SystemBlocks) > 0 {
		var parts []string
		for _, block := range req.SystemBlocks {
			parts = append(parts, block.Text)
		}
		systemPrompt = strings.Join(parts, "\n\n")
	}

	// 转换消息格式
	messages := convertToOpenAIMessages(req.Messages, systemPrompt)

	// 构建 API 参数
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(p.model),
		Messages: messages,
	}
	if req.MaxTokens > 0 {
		params.MaxCompletionTokens = param.NewOpt(int64(req.MaxTokens))
	}
	if req.Temperature > 0 {
		params.Temperature = param.NewOpt(req.Temperature)
	}

	// 转换工具定义
	if len(req.Tools) > 0 {
		params.Tools = convertToOpenAITools(req.Tools)
	}

	go func() {
		defer close(ch)

		stream := p.client.Chat.Completions.NewStreaming(ctx, params)
		defer stream.Close()

		// 跟踪工具调用状态（OpenAI 流式工具调用按 index 累积）
		type toolCallAccum struct {
			id        string
			name      string
			arguments strings.Builder
		}
		toolCalls := make(map[int64]*toolCallAccum)

		for stream.Next() {
			chunk := stream.Current()

			// 处理 usage（通常在最后一个 chunk）
			if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
				ch <- Event{
					Type: EventUsage,
					Usage: &Usage{
						InputTokens:  int(chunk.Usage.PromptTokens),
						OutputTokens: int(chunk.Usage.CompletionTokens),
					},
				}
			}

			for _, choice := range chunk.Choices {
				delta := choice.Delta

				// 文本内容
				if delta.Content != "" {
					ch <- Event{Type: EventTextDelta, Text: delta.Content}
				}

				// 工具调用（流式累积）
				for _, tc := range delta.ToolCalls {
					accum, ok := toolCalls[tc.Index]
					if !ok {
						accum = &toolCallAccum{}
						toolCalls[tc.Index] = accum
					}
					if tc.ID != "" {
						accum.id = tc.ID
					}
					if tc.Function.Name != "" {
						accum.name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						accum.arguments.WriteString(tc.Function.Arguments)
					}
				}

				// finish_reason == "tool_calls" 或 "stop" 表示完成
				if choice.FinishReason == "tool_calls" || choice.FinishReason == "stop" {
					// 发送所有累积的工具调用
					for _, accum := range toolCalls {
						if accum.name != "" {
							ch <- Event{
								Type: EventToolUse,
								ToolCall: &ToolCall{
									ID:    accum.id,
									Name:  accum.name,
									Input: accum.arguments.String(),
								},
							}
						}
					}
					// 清空，避免重复发送
					toolCalls = make(map[int64]*toolCallAccum)
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- Event{Type: EventError, Error: fmt.Errorf("openai stream error: %w", err)}
			return
		}

		ch <- Event{Type: EventDone}
	}()

	return ch, nil
}

// convertToOpenAIMessages 将统一消息格式转换为 OpenAI 格式
func convertToOpenAIMessages(msgs []Message, systemPrompt string) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion

	// 系统消息
	if systemPrompt != "" {
		result = append(result, openai.SystemMessage(systemPrompt))
	}

	for _, msg := range msgs {
		switch msg.Role {
		case RoleUser:
			// 用户消息可能包含纯文本或工具结果
			for _, block := range msg.Content {
				switch block.Type {
				case BlockText:
					result = append(result, openai.UserMessage(block.Text))
				case BlockToolResult:
					if block.ToolResult != nil {
						result = append(result, openai.ToolMessage(block.ToolResult.Content, block.ToolResult.ToolUseID))
					}
				}
			}

		case RoleAssistant:
			// 构建 assistant 消息
			var textContent string
			var toolCallParams []openai.ChatCompletionMessageToolCallParam

			for _, block := range msg.Content {
				switch block.Type {
				case BlockText:
					textContent += block.Text
				case BlockToolUse:
					if block.ToolCall != nil {
						toolCallParams = append(toolCallParams, openai.ChatCompletionMessageToolCallParam{
							ID: block.ToolCall.ID,
							Function: openai.ChatCompletionMessageToolCallFunctionParam{
								Name:      block.ToolCall.Name,
								Arguments: block.ToolCall.Input,
							},
						})
					}
				}
			}

			if len(toolCallParams) > 0 {
				// 带工具调用的 assistant 消息
				assistantMsg := openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: toolCallParams,
					},
				}
				if textContent != "" {
					assistantMsg.OfAssistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: param.NewOpt(textContent),
					}
				}
				result = append(result, assistantMsg)
			} else if textContent != "" {
				result = append(result, openai.AssistantMessage(textContent))
			}
		}
	}

	return result
}

// convertToOpenAITools 将统一工具定义转换为 OpenAI 格式
func convertToOpenAITools(tools []ToolDef) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, t := range tools {
		// 将 InputSchema 转为 FunctionParameters
		params := make(shared.FunctionParameters)
		for k, v := range t.InputSchema {
			params[k] = v
		}

		funcDef := shared.FunctionDefinitionParam{
			Name:       t.Name,
			Parameters: params,
		}
		if t.Description != "" {
			funcDef.Description = param.NewOpt(t.Description)
		}

		// 序列化再反序列化以创建正确的工具参数
		toolJSON, err := json.Marshal(map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		})
		if err != nil {
			continue
		}

		var toolParam openai.ChatCompletionToolParam
		if err := json.Unmarshal(toolJSON, &toolParam); err != nil {
			continue
		}
		result = append(result, toolParam)
	}
	return result
}
