package main

import (
	"context"
	"fmt"

	xcontext "github.com/xincode-ai/xin-code/internal/context"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/provider"
	"github.com/xincode-ai/xin-code/internal/session"
	"github.com/xincode-ai/xin-code/internal/tool"
	"github.com/xincode-ai/xin-code/internal/tui"
)

// Agent 核心引擎
type Agent struct {
	provider   provider.Provider
	tools      *tool.Registry
	permission tool.PermissionChecker
	config     *Config
	messages   []provider.Message

	// 会话管理
	session *session.Session
	store   *session.Store
	tracker *cost.Tracker

	// TUI 事件发送
	send func(msg interface{})
}

// NewAgent 创建 Agent 实例
func NewAgent(p provider.Provider, tools *tool.Registry, cfg *Config, sess *session.Session, store *session.Store, tracker *cost.Tracker, send func(msg interface{})) *Agent {
	return &Agent{
		provider:   p,
		tools:      tools,
		permission: &tool.SimplePermissionChecker{Mode: tool.PermissionMode(cfg.Permission.Mode)},
		config:     cfg,
		messages:   make([]provider.Message, 0),
		session:    sess,
		store:      store,
		tracker:    tracker,
		send:       send,
	}
}

// Run 执行一轮 Agent 循环（用户消息 -> API -> 工具 -> 循环）
func (a *Agent) Run(ctx context.Context, userMessage string) {
	// 追加用户消息
	userMsg := provider.NewTextMessage(provider.RoleUser, userMessage)
	a.messages = append(a.messages, userMsg)
	a.session.AddMessage(userMsg)

	// 组装 system prompt
	projectInstructions := xcontext.LoadProjectInstructions()
	systemPrompt := xcontext.BuildSystemPrompt(a.tools.ToolDefs(), projectInstructions)

	turns := 0
	for {
		turns++
		if turns > a.config.MaxTurns {
			a.send(tui.MsgError{Err: fmt.Errorf("达到最大轮次限制 (%d)", a.config.MaxTurns)})
			break
		}

		// 自动压缩检查
		maxCtx := a.provider.Capabilities().MaxContext
		if session.NeedsCompact(a.tracker.TotalTokens(), maxCtx) {
			compacted, msg := session.CompactMessages(a.messages)
			a.messages = compacted
			a.session.Messages = compacted
			a.send(tui.MsgSystemNotice{Text: "⚡ 已自动压缩: " + msg})
		}

		// 构建请求
		req := &provider.Request{
			Model:     a.config.Model,
			System:    systemPrompt,
			Messages:  a.messages,
			Tools:     a.tools.ToolDefs(),
			MaxTokens: a.config.MaxTokens,
		}

		// 流式调用 API
		events, err := a.provider.Stream(ctx, req)
		if err != nil {
			a.send(tui.MsgAgentDone{Err: fmt.Errorf("API error: %w", err)})
			return
		}

		// 处理流式事件
		assistantMsg, toolCalls, err := a.processStream(events)
		if err != nil {
			a.send(tui.MsgAgentDone{Err: err})
			return
		}

		// 通知流式响应完成
		a.send(tui.MsgResponseDone{})

		// 追加 assistant 消息到历史
		a.messages = append(a.messages, assistantMsg)
		a.session.AddMessage(assistantMsg)

		// 没有工具调用 -> 本轮结束
		if len(toolCalls) == 0 {
			break
		}

		// 权限询问回调：通过 TUI 的 PermissionDialog 交互
		askFn := func(toolName string, input string) bool {
			responseCh := make(chan tui.PermissionResponse, 1)
			a.send(tui.MsgPermissionRequest{
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

		// 发送工具开始事件
		for _, call := range toolCalls {
			a.send(tui.MsgToolStart{ID: call.ID, Name: call.Name, Input: call.Input})
		}

		// 批量执行工具
		results := a.tools.ExecuteBatch(ctx, toolCalls, a.permission, tool.AskPermissionFunc(askFn))

		for i, er := range results {
			call := toolCalls[i]
			result := er.Result
			if result == nil {
				result = &tool.Result{Content: "unknown error", IsError: true}
			}

			// 微压缩：大输出截断
			content := session.MicroCompact(result.Content)

			a.send(tui.MsgToolDone{
				ID:      call.ID,
				Name:    call.Name,
				Output:  content,
				IsError: result.IsError,
			})

			// 工具结果追加到消息历史
			toolResultMsg := provider.NewToolResultMessage(call.ID, content, result.IsError)
			a.messages = append(a.messages, toolResultMsg)
			a.session.AddMessage(toolResultMsg)
		}

		// 每轮工具执行后自动保存会话
		a.saveSession()
	}

	// 更新费用信息并保存
	a.session.UpdateCost(0, 0, a.tracker.TotalCostUSD())
	// 重置费用增量（费用已经在 tracker.AddUsage 中累计）
	a.session.TotalCostUSD = a.tracker.TotalCostUSD()
	a.session.TotalInputTokens = a.tracker.InputTokens()
	a.session.TotalOutputTokens = a.tracker.OutputTokens()
	a.saveSession()

	a.send(tui.MsgAgentDone{})
}

// processStream 处理流式事件，收集 assistant 消息和工具调用
func (a *Agent) processStream(events <-chan provider.Event) (provider.Message, []*provider.ToolCall, error) {
	var textContent string
	var toolCalls []*provider.ToolCall
	var blocks []provider.ContentBlock

	for evt := range events {
		switch evt.Type {
		case provider.EventTextDelta:
			a.send(tui.MsgTextDelta{Text: evt.Text})
			textContent += evt.Text

		case provider.EventThinking:
			if evt.Thinking != nil {
				a.send(tui.MsgThinking{Text: evt.Thinking.Text})
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
			if evt.Usage != nil {
				a.send(tui.MsgUsage{Usage: evt.Usage})
			}

		case provider.EventError:
			return provider.Message{}, nil, fmt.Errorf("stream error: %w", evt.Error)

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

// saveSession 保存会话到存储
func (a *Agent) saveSession() {
	if a.store != nil && a.session != nil {
		_ = a.store.Save(a.session)
	}
}
