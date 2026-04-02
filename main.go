package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/provider"
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

	// 检查 API Key
	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未找到 API Key")
		fmt.Fprintln(os.Stderr, "请设置环境变量: export ANTHROPIC_API_KEY=your-key")
		fmt.Fprintln(os.Stderr, "或: export XINCODE_API_KEY=your-key")
		os.Exit(1)
	}

	// 创建 Provider
	providerName := cfg.Provider
	if providerName == "" {
		providerName = provider.ResolveProviderName(cfg.Model)
	}
	p, err := provider.NewProvider(providerName, cfg.APIKey, cfg.Model, cfg.BaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Provider 初始化失败: %s\n", err)
		os.Exit(1)
	}

	// 创建费用追踪器
	tracker := cost.NewTracker(cfg.Model, cfg.Cost.Currency)

	// 创建 TUI
	app := tui.NewApp(tui.AppConfig{
		Model:      cfg.Model,
		Tracker:    tracker,
		MaxContext:  p.Capabilities().MaxContext,
		Version:    Version,
		ToolCount:  10, // 内置工具数
		PermMode:   cfg.Permission.Mode,
	})

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

	// 创建 Agent
	agent := NewAgent(p, tools, cfg, func(msg interface{}) {
		if m, ok := msg.(tea.Msg); ok {
			app.Send(m)
		}
	})

	// 启动 Agent goroutine：监听 TUI 提交的消息
	go func() {
		ctx := context.Background()
		for userMsg := range app.SubmitCh() {
			agent.Run(ctx, userMsg)
		}
	}()

	// 启动 TUI
	prog := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI 错误: %s\n", err)
		os.Exit(1)
	}
}
