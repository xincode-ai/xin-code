package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agentRetry "github.com/xincode-ai/xin-code/internal/agent"
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
	version    string // 版本号
	permMode   string // 权限模式
	reminderInjected bool // system-reminder 是否已注入

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
		version:    Version,
		permMode:   cfg.Permission.Mode,
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

	// 组装 system prompt（CC 风格：多层级发现 + 分层结构）
	homeDir, _ := os.UserHomeDir()
	promptCfg := xcontext.SystemPromptConfig{
		WorkDir:    a.session.WorkDir,
		HomeDir:    homeDir,
		Model:      a.config.Model,
		Provider:   a.provider.Name(),
		Version:    a.version,
		ToolCount:  len(a.tools.All()),
		PermMode:   a.permMode,
		MaxContext: a.provider.Capabilities().MaxContext,
	}
	// 分块构建 system prompt（支持 Anthropic prompt caching）
	// Block 1: 指令部分（静态，可缓存）— 包含角色定义、工具列表等
	instructionPrompt := xcontext.BuildFullSystemPrompt(promptCfg, a.tools.ToolDefs())

	// Block 2: 上下文部分（动态）— Git 状态等每轮可能变化的信息
	contextPrompt := xcontext.BuildSystemContext(promptCfg)

	// 组装 SystemBlocks
	var systemBlocks []provider.SystemBlock
	systemBlocks = append(systemBlocks, provider.SystemBlock{
		Text:         instructionPrompt,
		CacheControl: "ephemeral", // 静态指令启用缓存
	})
	if contextPrompt != "" {
		systemBlocks = append(systemBlocks, provider.SystemBlock{
			Text: contextPrompt, // 动态上下文不标记缓存，但 anthropic.go 会为最后一个 block 自动添加
		})
	}

	// 同时保留 System string 做兼容（供非 Anthropic provider 使用）
	systemPrompt := instructionPrompt
	if contextPrompt != "" {
		systemPrompt += "\n\n" + contextPrompt
	}

	// 用户上下文（XINCODE.md + 日期）作为 system-reminder user message 注入消息列表首位
	// CC 参考：prependUserContext() — 创建 role:user 消息包裹 <system-reminder> 标签
	if !a.reminderInjected {
		if userCtx := xcontext.BuildUserContext(promptCfg); userCtx != "" {
			reminderMsg := provider.NewTextMessage(provider.RoleUser, "<system-reminder>\n"+userCtx+"\n</system-reminder>")
			a.messages = append([]provider.Message{reminderMsg}, a.messages...)
			a.reminderInjected = true
		}
	}

	turns := 0
	reactiveCompacted := false // 413 自动压缩最多触发一次，避免无限循环
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
			Model:        a.config.Model,
			System:       systemPrompt,
			SystemBlocks: systemBlocks,
			Messages:     a.messages,
			Tools:        a.tools.ToolDefs(),
			MaxTokens:    a.config.MaxTokens,
		}

		// 流式调用 API（带重试）
		var events <-chan provider.Event
		var streamErr error
		retryCfg := agentRetry.DefaultRetryConfig()
		retryCfg.OnRetry = func(attempt int, err error, delay time.Duration) {
			a.send(tui.MsgSystemNotice{Text: fmt.Sprintf("API 错误，%v 后重试 (第 %d 次)...", delay.Round(time.Second), attempt)})
		}
		retryErr := agentRetry.WithRetry(ctx, retryCfg, func(ctx context.Context, attempt int) error {
			var err error
			events, err = a.provider.Stream(ctx, req)
			return err
		})
		if retryErr != nil {
			// Reactive Compact：413 prompt-too-long 时自动压缩并重试
			if isPromptTooLong(retryErr) && !reactiveCompacted {
				reactiveCompacted = true
				a.send(tui.MsgSystemNotice{Text: "⚡ 上下文过大 (413)，正在自动压缩后重试..."})
				compacted, _ := session.CompactMessages(a.messages)
				a.messages = compacted
				a.session.Messages = compacted
				continue // 重新进入循环
			}
			a.send(tui.MsgAgentDone{Err: fmt.Errorf("API error: %w", retryErr)})
			return
		}

		// 处理流式事件
		assistantMsg, toolCalls, streamErr := a.processStream(events)
		if streamErr != nil {
			// Reactive Compact：流式过程中收到 413 时自动压缩并重试
			if isPromptTooLong(streamErr) && !reactiveCompacted {
				reactiveCompacted = true
				a.send(tui.MsgSystemNotice{Text: "⚡ 上下文过大 (413)，正在自动压缩后重试..."})
				compacted, _ := session.CompactMessages(a.messages)
				a.messages = compacted
				a.session.Messages = compacted
				continue // 重新进入循环
			}
			a.send(tui.MsgAgentDone{Err: streamErr})
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

// isPromptTooLong 检测错误是否为 413 prompt-too-long（上下文超限）
func isPromptTooLong(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// 检测 HTTP 413 状态码
	if strings.Contains(msg, "413") {
		return true
	}
	// 检测 Anthropic 的 prompt_too_long 错误消息
	if strings.Contains(msg, "prompt is too long") || strings.Contains(msg, "prompt_too_long") {
		return true
	}
	return false
}

// saveSession 保存会话到存储
func (a *Agent) saveSession() {
	if a.store != nil && a.session != nil {
		_ = a.store.Save(a.session)
	}
}

