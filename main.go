package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xincode-ai/xin-code/internal/auth"
	"github.com/xincode-ai/xin-code/internal/setup"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/hooks"
	"github.com/xincode-ai/xin-code/internal/mcp"
	"github.com/xincode-ai/xin-code/internal/plugins"
	agentPkg "github.com/xincode-ai/xin-code/internal/agent"
	"github.com/xincode-ai/xin-code/internal/provider"
	"github.com/xincode-ai/xin-code/internal/session"
	"github.com/xincode-ai/xin-code/internal/skills"
	"github.com/xincode-ai/xin-code/internal/tool"
	"github.com/xincode-ai/xin-code/internal/tool/builtin"
	"github.com/xincode-ai/xin-code/internal/tui"
)

func main() {
	// --version
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("xin-code %s (%s %s)\n", Version, Commit, Date)
		os.Exit(0)
	}

	// 加载配置
	cfg := LoadConfig()

	// 认证链：通过 auth 模块按优先级解析 API Key
	authSource := ""
	if cfg.APIKey == "" {
		// config.go 的 LoadConfig 已处理环境变量，这里补充 CC OAuth
		apiKey, source := auth.ResolveAPIKey(XinCodeDir())
		if apiKey != "" {
			cfg.APIKey = apiKey
			authSource = source
		}
	}

	if cfg.APIKey == "" {
		// 无 API Key：启动 Setup 引导
		setupModel := setup.New(XinCodeDir())
		setupProg := tea.NewProgram(setupModel, tea.WithAltScreen())
		finalModel, err := setupProg.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Setup 错误: %s\n", err)
			os.Exit(1)
		}
		result, setupErr := finalModel.(setup.Model).GetResult()
		if setupErr != nil {
			fmt.Fprintf(os.Stderr, "配置失败: %s\n", setupErr)
			os.Exit(1)
		}
		if result.APIKey == "" {
			fmt.Fprintln(os.Stderr, "未完成配置，退出。")
			os.Exit(0)
		}
		// 重新加载配置（刚写入的 settings.json + credentials.json）
		cfg = LoadConfig()
		if cfg.APIKey == "" {
			fmt.Fprintln(os.Stderr, "配置已保存但加载失败，请检查 ~/.xincode/ 目录")
			os.Exit(1)
		}
		authSource = "config"
	}

	// 创建 Provider
	providerName := cfg.Provider
	if providerName == "" {
		providerName = provider.ResolveProviderName(cfg.Model)
	}
	p, err := provider.NewProvider(providerName, cfg.APIKey, cfg.Model, cfg.BaseURL, authSource)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Provider 初始化失败: %s\n", err)
		os.Exit(1)
	}

	// 创建费用追踪器
	tracker := cost.NewTracker(cfg.Model, cfg.Cost.Currency)

	// 创建会话
	workDir, _ := os.Getwd()
	store := session.NewStore(XinCodeDir())
	sess := session.NewSession(session.SessionConfig{
		Model:      cfg.Model,
		WorkDir:    workDir,
		Provider:   providerName,
		BaseURL:    cfg.BaseURL,
		AuthSource: authSource,
	})

	// 初始化 MCP 客户端
	mcpClient := mcp.NewClient()
	if len(cfg.MCP) > 0 {
		var mcpConfigs []mcp.ServerConfig
		for _, m := range cfg.MCP {
			mcpConfigs = append(mcpConfigs, mcp.ServerConfig{
				Name:    m.Name,
				Command: m.Command,
				Args:    m.Args,
				Env:     m.Env,
			})
		}
		mcpClient.LoadConfigs(mcpConfigs)
		// 异步连接 MCP 服务器
		go func() {
			ctx := context.Background()
			_ = mcpClient.ConnectAll(ctx)
		}()
	}

	// 初始化技能系统
	skillsRegistry := skills.NewRegistry()
	skillsRegistry.Discover(XinCodeDir())

	// 初始化插件系统
	pluginsRegistry := plugins.NewRegistry()
	pluginsRegistry.Discover(XinCodeDir())

	// 初始化钩子系统
	hooksConfig := hooks.LoadConfig(filepath.Join(XinCodeDir(), "settings.json"))
	// 合并配置文件中的钩子定义
	if len(cfg.Hooks.PreToolUse) > 0 || len(cfg.Hooks.PostToolUse) > 0 {
		for _, h := range cfg.Hooks.PreToolUse {
			hooksConfig.PreToolUse = append(hooksConfig.PreToolUse, hooks.HookDef{
				Match: h.Match, Command: h.Command,
			})
		}
		for _, h := range cfg.Hooks.PostToolUse {
			hooksConfig.PostToolUse = append(hooksConfig.PostToolUse, hooks.HookDef{
				Match: h.Match, Command: h.Command,
			})
		}
	}
	hooksMgr := hooks.NewManager(hooksConfig)

	// 创建 TUI
	app := tui.NewApp(tui.AppConfig{
		Model:      cfg.Model,
		Provider:   providerName,
		Tracker:    tracker,
		MaxContext: p.Capabilities().MaxContext,
		Version:    Version,
		ToolCount:  10, // 内置工具数
		PermMode:   cfg.Permission.Mode,
		WorkDir:    workDir,
	})

	// 注入会话信息
	app.SessionID = sess.ID
	app.SessionTurns = sess.Turns

	// 注入扩展系统回调
	app.OnSkillsList = func() string { return skillsRegistry.ListString() }
	app.OnPluginsList = func() string { return pluginsRegistry.ListString() }
	app.OnHooksList = func() string { return hooksMgr.ListString() }

	// 前置声明 agent（回调闭包引用，实际创建在后面）
	var agent *Agent

	// 注入回调
	app.OnClear = func() {
		// 清空会话消息
		sess.Messages = sess.Messages[:0]
		sess.Turns = 0
		if agent != nil {
			agent.SyncFromSession()
		}
	}
	app.OnCompact = func() string {
		compacted, msg := session.CompactMessages(sess.Messages)
		sess.Messages = compacted
		agent.SyncFromSession() // 同步 agent 运行时历史
		return msg
	}
	app.OnExport = func() string {
		md := sess.ExportMarkdown()
		exportPath := filepath.Join(workDir, fmt.Sprintf("session-%s.md", sess.ID))
		if err := os.WriteFile(exportPath, []byte(md), 0644); err != nil {
			return fmt.Sprintf("导出失败: %s", err)
		}
		return fmt.Sprintf("已导出到: %s", exportPath)
	}
	app.OnLogin = func() string {
		return "请退出后运行 xin-code 重新配置，或设置环境变量：\n" +
			"  export ANTHROPIC_API_KEY=your-key\n" +
			"  export OPENAI_API_KEY=your-key\n\n" +
			"当前凭据路径: " + filepath.Join(XinCodeDir(), "auth", "credentials.json")
	}
	app.OnLogout = func() string {
		if err := auth.ClearAPIKey(XinCodeDir()); err != nil {
			return fmt.Sprintf("清除凭据失败: %s", err)
		}
		return "已清除保存的 API Key。\n下次启动将重新进入配置向导。"
	}
	app.OnResume = func() ([]tui.ResumeEntry, error) {
		entries, err := store.List(workDir)
		if err != nil {
			return nil, err
		}
		result := make([]tui.ResumeEntry, 0, len(entries))
		for _, e := range entries {
			mode, reason := classifyResumeCompat(e, providerName, cfg.BaseURL, authSource)
			result = append(result, tui.ResumeEntry{
				ID:         e.ID,
				Model:      e.Model,
				Provider:   e.Provider,
				BaseURL:    e.BaseURL,
				AuthSource: e.AuthSource,
				Turns:      e.Turns,
				Cost:       fmt.Sprintf("$%.4f", e.CostUSD),
				Mode:       mode,
				ModeReason: reason,
			})
		}
		return result, nil
	}
	app.OnResumeLoad = func(sessionID string) error {
		loaded, err := store.Load(sessionID)
		if err != nil {
			return err
		}

		// ── 0. 兼容性检查 ──
		// 优先使用 session 持久化的 provider，旧会话缺失时降级到模型名推断
		loadedProvider := loaded.Provider
		if loadedProvider == "" {
			loadedProvider = provider.ResolveProviderName(loaded.Model)
		}
		if loadedProvider != providerName {
			return fmt.Errorf("该会话使用 %s (%s)，当前运行 %s (%s)，暂不支持跨 provider 恢复",
				loaded.Model, loadedProvider, cfg.Model, providerName)
		}
		// BaseURL 不一致也阻止（自定义 endpoint 不能混用）
		loadedBaseURL := loaded.BaseURL
		if loadedBaseURL != cfg.BaseURL {
			if loadedBaseURL != "" || cfg.BaseURL != "" {
				return fmt.Errorf("该会话使用 endpoint %q，当前运行 %q，暂不支持跨 endpoint 恢复",
					loadedBaseURL, cfg.BaseURL)
			}
		}
		// AuthSource 不兼容时，selector 已在 Enter 处设 readOnly=true
		// OnResumeLoad 本身不阻止——加载 transcript 是安全的，只是不应发新请求
		// 若未来有绕过 selector 的调用路径（如 CLI --resume），需在此处增加 authSource 检查

		// ── 1. 恢复 Session 真源 ──
		sess.Messages = loaded.Messages
		sess.Turns = loaded.Turns
		sess.ID = loaded.ID
		sess.CreatedAt = loaded.CreatedAt
		sess.TotalInputTokens = loaded.TotalInputTokens
		sess.TotalOutputTokens = loaded.TotalOutputTokens
		sess.TotalCostUSD = loaded.TotalCostUSD
		sess.Model = loaded.Model
		sess.Provider = loaded.Provider
		sess.BaseURL = loaded.BaseURL
		sess.AuthSource = loaded.AuthSource

		// ── 2. 同步 Agent 运行时（关键：让后续请求用恢复后的历史和模型）──
		agent.RestoreSession()

		// ── 3. 同步 TUI 状态栏 / tracker ──
		tracker.Reset()
		tracker.SetModel(loaded.Model)
		tracker.AddUsage(loaded.TotalInputTokens, loaded.TotalOutputTokens, 0, 0)
		app.SetModel(loaded.Model)
		app.SessionID = loaded.ID
		app.SessionTurns = loaded.Turns

		// ── 4. 从 session.Messages 重建前台 transcript（含真实 tool 结果）──
		app.RestoreTranscript(rebuildTranscript(loaded.Messages))
		return nil
	}

	var runMu sync.Mutex
	var cancelCurrentRun context.CancelFunc
	app.OnInterrupt = func() {
		runMu.Lock()
		cancel := cancelCurrentRun
		runMu.Unlock()
		if cancel != nil {
			cancel()
		}
	}

	// 创建子 Agent 注册表
	subAgentReg := agentPkg.NewSubAgentRegistry()

	// TUI 事件发送回调（给 SubAgent 用）
	sendMsg := func(msg interface{}) {
		if m, ok := msg.(tea.Msg); ok {
			app.Send(m)
		}
	}

	// 权限检查器
	permChecker := &tool.SimplePermissionChecker{Mode: tool.PermissionMode(cfg.Permission.Mode)}

	// 注册工具（AskUser / DiffPreview 通过 TUI channel 交互）
	tools := tool.NewRegistry()
	builtin.RegisterAll(tools, builtin.RegisterConfig{
		AskFunc: func(ctx context.Context, question string) (string, error) {
			responseCh := make(chan string, 1)
			app.Send(tui.MsgAskUser{Question: question, Response: responseCh})
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case answer := <-responseCh:
				return answer, nil
			}
		},
		ConfirmFunc: func(ctx context.Context, path string, diffText string) (bool, error) {
			responseCh := make(chan bool, 1)
			app.Send(tui.MsgDiffPreview{Path: path, DiffText: diffText, Response: responseCh})
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case confirmed := <-responseCh:
				return confirmed, nil
			}
		},
		CurrentProvider: func() provider.Provider {
			if agent != nil {
				return agent.CurrentProvider()
			}
			return p
		},
		CurrentModel: func() string {
			if agent != nil {
				return agent.CurrentModel()
			}
			return cfg.Model
		},
		Permission:      permChecker,
		Tracker:         tracker,
		MaxTokens:       cfg.MaxTokens,
		SendMsg:         sendMsg,
		SubAgentReg:     subAgentReg,
	})

	// MCP 工具注册
	mcpClient.RegisterToRegistry(tools)

	// 创建 Agent（前置声明在回调注入前）
	agent = NewAgent(p, tools, cfg, sess, store, tracker, func(msg interface{}) {
		if m, ok := msg.(tea.Msg); ok {
			app.Send(m)
		}
	})

	// 启动 Agent goroutine：监听 TUI 提交的消息
	go func() {
		for userMsg := range app.SubmitCh() {
			runCtx, cancel := context.WithCancel(context.Background())
			runMu.Lock()
			cancelCurrentRun = cancel
			runMu.Unlock()

			agent.Run(runCtx, userMsg)

			runMu.Lock()
			cancelCurrentRun = nil
			runMu.Unlock()
			cancel()
			// 更新 TUI 中的会话信息
			app.SessionTurns = sess.Turns
		}
	}()

	// 启动 TUI
	// 预设终端背景色检测，避免 Lipgloss OSC 查询导致 ANSI 响应泄漏到输入框
	os.Setenv("GLAMOUR_STYLE", "dark")
	// SGR 鼠标转义序列片段：动态鼠标模式下仍可能泄漏
	sgrMouseRe := regexp.MustCompile(`^\[<\d+;\d+;\d+[Mm]`)

	prog := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // 默认浏览模式：启用鼠标，支持滚轮滚动（Ctrl+Y 切到复制模式）
		tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				s := keyMsg.String()
				// 过滤掉终端 OSC 响应（背景/前景色查询回复）
				if strings.Contains(s, "rgb:") || strings.HasPrefix(s, "]") ||
					strings.Contains(s, "\x1b") {
					return nil
				}
				// 过滤掉泄漏的 SGR 鼠标转义序列片段
				if sgrMouseRe.MatchString(s) {
					return nil
				}
			}
			return msg
		}),
	)
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI 错误: %s\n", err)
		os.Exit(1)
	}

	// 退出前保存会话（只读浏览模式下跳过，防止误修改被持久化）
	if !app.IsReadOnly() {
		_ = store.Save(sess)
	}
	mcpClient.Close()
}

// rebuildTranscript 从 session messages 重建前台 transcript
// 关键改进：用真实 tool 结果替代占位文本，跳过 system-reminder 和 compact summary
func rebuildTranscript(messages []provider.Message) []tui.ChatMessage {
	// 建 toolID → result 映射（tool result 在 RoleUser 消息的 BlockToolResult 块中）
	toolResults := make(map[string]*provider.ToolResult)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == provider.BlockToolResult && block.ToolResult != nil {
				toolResults[block.ToolResult.ToolUseID] = block.ToolResult
			}
		}
	}

	var chatMsgs []tui.ChatMessage
	for _, msg := range messages {
		switch msg.Role {
		case provider.RoleUser:
			// 跳过 tool result 消息（它们会内联显示在 tool call 下方）
			isToolResult := false
			for _, block := range msg.Content {
				if block.Type == provider.BlockToolResult {
					isToolResult = true
					break
				}
			}
			if isToolResult {
				continue
			}

			text := msg.TextContent()
			if text == "" {
				continue
			}
			// 跳过 system-reminder 注入消息
			if strings.HasPrefix(text, "<system-reminder>") {
				continue
			}
			// compact summary → 显示为系统提示
			if strings.HasPrefix(text, "[上下文摘要]") {
				chatMsgs = append(chatMsgs, tui.ChatMessage{
					Role:    "system",
					Content: "⚡ 上下文已压缩",
				})
				continue
			}

			chatMsgs = append(chatMsgs, tui.ChatMessage{
				Role:    "user",
				Content: text,
			})

		case provider.RoleAssistant:
			// 恢复 thinking blocks
			for _, block := range msg.Content {
				if block.Type == provider.BlockThinking && block.Thinking != "" {
					chatMsgs = append(chatMsgs, tui.ChatMessage{
						Role:    "thinking",
						Content: block.Thinking,
						Folded:  true,
					})
				}
			}
			text := msg.TextContent()
			// 跳过 compact placeholder 回复
			if text == "已加载上下文摘要，继续当前任务。" {
				continue
			}
			if text != "" {
				chatMsgs = append(chatMsgs, tui.ChatMessage{
					Role:    "assistant",
					Content: text,
				})
			}
			// 恢复 tool call + 真实 result
			for _, call := range msg.ToolCalls() {
				content := "(已恢复)"
				isError := false
				folded := true
				if result, ok := toolResults[call.ID]; ok {
					content = result.Content
					isError = result.IsError
					// 短输出不折叠
					if strings.Count(content, "\n") <= 8 {
						folded = false
					}
				}
				chatMsgs = append(chatMsgs, tui.ChatMessage{
					Role:      "tool",
					ToolName:  call.Name,
					ToolID:    call.ID,
					ToolInput: call.Input,
					Content:   content,
					IsError:   isError,
					Folded:    folded,
				})
			}
		}
	}
	return chatMsgs
}

// classifyResumeCompat 判断历史会话与当前运行环境的兼容性
// 返回 (ResumeMode, 原因文案)
func classifyResumeCompat(e session.IndexEntry, curProvider, curBaseURL, curAuthSource string) (tui.ResumeMode, string) {
	// provider 推断（兼容旧 session 无 Provider 字段）
	entryProvider := e.Provider
	if entryProvider == "" {
		entryProvider = provider.ResolveProviderName(e.Model)
	}

	// 跨 provider → blocked
	if entryProvider != curProvider {
		return tui.ResumeBlocked, tui.ReasonProviderBlock
	}

	// 跨 endpoint → blocked
	if e.BaseURL != curBaseURL {
		return tui.ResumeBlocked, tui.ReasonEndpointBlock
	}

	// authSource 不兼容 → readonly
	// 空 authSource（旧会话）与任何当前 authSource 视为兼容
	if e.AuthSource != "" && curAuthSource != "" && e.AuthSource != curAuthSource {
		return tui.ResumeReadonly, tui.ReasonAuthMismatch
	}

	return tui.ResumeContinue, tui.ReasonCompatible
}
