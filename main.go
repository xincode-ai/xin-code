package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xincode-ai/xin-code/internal/auth"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/hooks"
	"github.com/xincode-ai/xin-code/internal/mcp"
	"github.com/xincode-ai/xin-code/internal/plugins"
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
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "设置方式（任选其一）:")
		fmt.Fprintln(os.Stderr, "  1. export ANTHROPIC_API_KEY=your-key")
		fmt.Fprintln(os.Stderr, "  2. export OPENAI_API_KEY=your-key")
		fmt.Fprintln(os.Stderr, "  3. export XINCODE_API_KEY=your-key")
		fmt.Fprintln(os.Stderr, "  4. 安装 Claude Code 并登录（自动复用 OAuth token）")
		os.Exit(1)
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
	sess := session.NewSession(cfg.Model, workDir)

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

	// 注入回调
	app.OnClear = func() {
		// 清空会话消息
		sess.Messages = sess.Messages[:0]
		sess.Turns = 0
	}
	app.OnCompact = func() string {
		compacted, msg := session.CompactMessages(sess.Messages)
		sess.Messages = compacted
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
	app.OnResume = func() string {
		entries, err := store.List(workDir)
		if err != nil {
			return fmt.Sprintf("获取历史会话失败: %s", err)
		}
		if len(entries) == 0 {
			return "当前目录没有历史会话"
		}
		var sb []string
		sb = append(sb, "📋 历史会话:\n")
		for i, e := range entries {
			if i >= 10 {
				sb = append(sb, fmt.Sprintf("  ... 还有 %d 个会话", len(entries)-10))
				break
			}
			sb = append(sb, fmt.Sprintf("  %d. [%s] %s | %d 轮 | $%.4f",
				i+1, e.ID, e.Model, e.Turns, e.CostUSD))
		}
		sb = append(sb, "\n暂不支持交互式选择恢复，功能开发中")
		result := ""
		for _, s := range sb {
			result += s + "\n"
		}
		return result
	}

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
	})

	// MCP 工具注册
	mcpClient.RegisterToRegistry(tools)

	// 创建 Agent
	agent := NewAgent(p, tools, cfg, sess, store, tracker, func(msg interface{}) {
		if m, ok := msg.(tea.Msg); ok {
			app.Send(m)
		}
	})

	// 启动 Agent goroutine：监听 TUI 提交的消息
	go func() {
		ctx := context.Background()
		for userMsg := range app.SubmitCh() {
			agent.Run(ctx, userMsg)
			// 更新 TUI 中的会话信息
			app.SessionTurns = sess.Turns
		}
	}()

	// 启动 TUI
	// 预设终端背景色检测，避免 Lipgloss OSC 查询导致 ANSI 响应泄漏到输入框
	os.Setenv("GLAMOUR_STYLE", "dark")
	prog := tea.NewProgram(app,
		tea.WithAltScreen(),
		tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
			// 过滤掉终端 OSC 响应（背景/前景色查询回复）
			// 这些响应会被 Bubbletea 的 stdin reader 捕获
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				s := keyMsg.String()
				if strings.Contains(s, "rgb:") || strings.HasPrefix(s, "]") ||
					strings.Contains(s, "\x1b") {
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

	// 退出前保存会话和关闭 MCP
	_ = store.Save(sess)
	mcpClient.Close()
}
