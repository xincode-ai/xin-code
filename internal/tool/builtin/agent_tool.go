package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentPkg "github.com/xincode-ai/xin-code/internal/agent"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/provider"
	"github.com/xincode-ai/xin-code/internal/tool"
	"github.com/xincode-ai/xin-code/internal/tui"
)

// subAgentMaxTurns 子 agent 最大轮次限制
const subAgentMaxTurns = 30

// subAgentSystemPrompt 子 agent 精简 system prompt
const subAgentSystemPrompt = `你是一个子 Agent，被分派执行特定任务。
专注完成任务并报告结果。不要偏离任务范围。
完成后直接输出结果文本，不要调用多余的工具。`

// AgentTool 派生子 Agent 工具
type AgentTool struct {
	Provider     provider.Provider
	Tools        *tool.Registry
	Permission   tool.PermissionChecker
	Tracker      *cost.Tracker
	MaxTokens    int
	Model        string
	SendMsg      func(interface{})        // TUI 通知回调
	SubAgentReg  *agentPkg.SubAgentRegistry // 子 agent 注册表
}

type agentInput struct {
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
}

func (t *AgentTool) Name() string { return "Agent" }
func (t *AgentTool) Description() string {
	return "启动一个子 Agent 自主完成指定任务。子 Agent 拥有独立的消息历史，共享工具和模型。"
}
func (t *AgentTool) IsReadOnly() bool { return false }
func (t *AgentTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt":      map[string]any{"type": "string", "description": "子 Agent 的任务描述"},
			"description": map[string]any{"type": "string", "description": "3-5 个字的简短摘要"},
		},
		"required": []string{"prompt", "description"},
	}
}

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage) (*tool.Result, error) {
	var in agentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}
	if in.Prompt == "" {
		return &tool.Result{Content: "prompt is required", IsError: true}, nil
	}
	if in.Description == "" {
		in.Description = "子任务"
	}

	// 生成唯一 ID
	agentID := fmt.Sprintf("sa-%d", time.Now().UnixNano())

	// 注册到 SubAgent Registry
	entry := &agentPkg.SubAgentEntry{
		ID:          agentID,
		Name:        in.Description,
		Description: in.Description,
		InputCh:     make(chan string, 1),
	}
	if t.SubAgentReg != nil {
		t.SubAgentReg.Register(entry)
		defer t.SubAgentReg.Remove(agentID)
	}

	// 通知 TUI：子 agent 开始
	if t.SendMsg != nil {
		t.SendMsg(tui.MsgSubAgentStart{
			ID:          agentID,
			Description: in.Description,
		})
	}

	// 运行子 agent
	result, err := runSubAgent(ctx, subAgentRunConfig{
		prompt:     in.Prompt,
		provider:   t.Provider,
		tools:      t.Tools,
		permission: t.Permission,
		tracker:    t.Tracker,
		maxTokens:  t.MaxTokens,
		model:      t.Model,
		sendMsg:    t.SendMsg,
	})

	// 标记完成
	if entry != nil {
		entry.Done = true
		entry.Result = result
	}

	// 通知 TUI：子 agent 完成
	if t.SendMsg != nil {
		summary := result
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		t.SendMsg(tui.MsgSubAgentDone{
			ID:          agentID,
			Description: in.Description,
			Result:      summary,
		})
	}

	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("子 Agent 执行出错: %s", err),
			IsError: true,
		}, nil
	}

	if result == "" {
		result = "(子 Agent 未产生输出)"
	}

	return &tool.Result{Content: result}, nil
}

// subAgentRunConfig 子 agent 运行配置
type subAgentRunConfig struct {
	prompt     string
	provider   provider.Provider
	tools      *tool.Registry
	permission tool.PermissionChecker
	tracker    *cost.Tracker
	maxTokens  int
	model      string
	sendMsg    func(interface{})
}

// runSubAgent 运行子 agent 循环，返回收集的文本输出
func runSubAgent(ctx context.Context, cfg subAgentRunConfig) (string, error) {
	// 独立的消息历史
	messages := []provider.Message{
		provider.NewTextMessage(provider.RoleUser, cfg.prompt),
	}

	var resultText strings.Builder
	toolDefs := cfg.tools.ToolDefs()

	for turn := 0; turn < subAgentMaxTurns; turn++ {
		// 构建请求
		req := &provider.Request{
			Model:     cfg.model,
			System:    subAgentSystemPrompt,
			Messages:  messages,
			Tools:     toolDefs,
			MaxTokens: cfg.maxTokens,
		}

		// 流式调用 API
		events, err := cfg.provider.Stream(ctx, req)
		if err != nil {
			return resultText.String(), fmt.Errorf("API error: %w", err)
		}

		// 处理流式事件
		assistantMsg, toolCalls, streamErr := processSubAgentStream(events, cfg.tracker, cfg.sendMsg)
		if streamErr != nil {
			return resultText.String(), streamErr
		}

		// 收集文本输出
		text := assistantMsg.TextContent()
		if text != "" {
			if resultText.Len() > 0 {
				resultText.WriteString("\n")
			}
			resultText.WriteString(text)
		}

		// 追加 assistant 消息
		messages = append(messages, assistantMsg)

		// 没有工具调用 -> 子 agent 结束
		if len(toolCalls) == 0 {
			break
		}

		// 发送工具开始事件（让 TUI 显示子 agent 的工具调用）
		if cfg.sendMsg != nil {
			for _, call := range toolCalls {
				cfg.sendMsg(tui.MsgToolStart{ID: call.ID, Name: call.Name, Input: call.Input})
			}
		}

		// 权限询问回调：复用父 agent 的权限机制
		askFn := tool.AskPermissionFunc(func(toolName string, input string) bool {
			// 子 agent 的工具调用也需要权限确认
			if cfg.sendMsg != nil {
				responseCh := make(chan tui.PermissionResponse, 1)
				cfg.sendMsg(tui.MsgPermissionRequest{
					ToolName: toolName,
					Input:    input,
					Response: responseCh,
				})
				select {
				case <-ctx.Done():
					return false
				case resp := <-responseCh:
					return resp == tui.PermAllow || resp == tui.PermAlways
				}
			}
			return true
		})

		// 执行工具
		results := cfg.tools.ExecuteBatch(ctx, toolCalls, cfg.permission, askFn)

		for i, er := range results {
			call := toolCalls[i]
			result := er.Result
			if result == nil {
				result = &tool.Result{Content: "unknown error", IsError: true}
			}

			// 通知 TUI
			if cfg.sendMsg != nil {
				cfg.sendMsg(tui.MsgToolDone{
					ID:      call.ID,
					Name:    call.Name,
					Output:  result.Content,
					IsError: result.IsError,
				})
			}

			// 工具结果追加到消息历史
			toolResultMsg := provider.NewToolResultMessage(call.ID, result.Content, result.IsError)
			messages = append(messages, toolResultMsg)
		}
	}

	return resultText.String(), nil
}

// processSubAgentStream 处理子 agent 的流式事件
func processSubAgentStream(events <-chan provider.Event, tracker *cost.Tracker, sendMsg func(interface{})) (provider.Message, []*provider.ToolCall, error) {
	var textContent string
	var toolCalls []*provider.ToolCall
	var blocks []provider.ContentBlock

	for evt := range events {
		switch evt.Type {
		case provider.EventTextDelta:
			textContent += evt.Text
			// 子 agent 的文本增量也发给 TUI 显示
			if sendMsg != nil {
				sendMsg(tui.MsgTextDelta{Text: evt.Text})
			}

		case provider.EventToolUse:
			if evt.ToolCall != nil {
				toolCalls = append(toolCalls, evt.ToolCall)
				blocks = append(blocks, provider.ContentBlock{
					Type:     provider.BlockToolUse,
					ToolCall: evt.ToolCall,
				})
			}

		case provider.EventUsage:
			if evt.Usage != nil && tracker != nil {
				tracker.AddUsage(
					evt.Usage.InputTokens,
					evt.Usage.OutputTokens,
					evt.Usage.CacheCreationInputTokens,
					evt.Usage.CacheReadInputTokens,
				)
				if sendMsg != nil {
					sendMsg(tui.MsgUsage{Usage: evt.Usage})
				}
			}

		case provider.EventError:
			return provider.Message{}, nil, fmt.Errorf("stream error: %w", evt.Error)

		case provider.EventThinking:
			// 子 agent 的思考也传递给 TUI
			if evt.Thinking != nil && sendMsg != nil {
				sendMsg(tui.MsgThinking{Text: evt.Thinking.Text})
			}

		case provider.EventDone:
			// 流结束
		}
	}

	// 构建 assistant 消息
	if textContent != "" {
		blocks = append([]provider.ContentBlock{
			{Type: provider.BlockText, Text: textContent},
		}, blocks...)
	}

	msg := provider.Message{
		Role:    provider.RoleAssistant,
		Content: blocks,
	}

	return msg, toolCalls, nil
}
