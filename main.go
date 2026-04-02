package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/xincode-ai/xin-code/internal/provider"
	"github.com/xincode-ai/xin-code/internal/tool"
	"github.com/xincode-ai/xin-code/internal/tool/builtin"
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

	// 注册工具
	tools := tool.NewRegistry()
	builtin.RegisterAll(tools)

	// 创建 Agent
	agent := NewAgent(p, tools, cfg, os.Stdout)

	// 信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// 打印欢迎信息
	fmt.Printf("XIN CODE %s\n", Version)
	fmt.Printf("model: %s  provider: %s\n", cfg.Model, p.Name())
	fmt.Printf("tools: %d  mode: %s\n", len(tools.All()), cfg.Permission.Mode)
	fmt.Println("---")
	fmt.Println("输入消息开始对话。/help 查看命令，/quit 退出。")
	fmt.Println()

	// REPL 循环
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// 斜杠命令
		if strings.HasPrefix(input, "/") {
			switch input {
			case "/quit", "/exit":
				fmt.Println("再见！")
				return
			case "/help":
				fmt.Println("可用命令:")
				fmt.Println("  /help    - 帮助信息")
				fmt.Println("  /model   - 显示当前模型")
				fmt.Println("  /version - 版本信息")
				fmt.Println("  /quit    - 退出")
			case "/model":
				fmt.Printf("当前模型: %s (%s)\n", cfg.Model, p.Name())
			case "/version":
				fmt.Printf("xin-code %s (%s %s)\n", Version, Commit, Date)
			default:
				fmt.Printf("未知命令: %s\n", input)
			}
			continue
		}

		// Shell 命令
		if strings.HasPrefix(input, "!") {
			cmd := strings.TrimPrefix(input, "!")
			result := tools.ExecuteTool(ctx, &provider.ToolCall{
				ID: "shell", Name: "Bash", Input: fmt.Sprintf(`{"command":%q}`, cmd),
			})
			fmt.Println(result.Content)
			continue
		}

		// Agent 对话
		if err := agent.Run(ctx, input); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %s\n", err)
		}
	}
}
