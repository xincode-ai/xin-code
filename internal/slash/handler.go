package slash

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xincode-ai/xin-code/internal/memory"
)

// ResultType 命令结果类型
type ResultType string

const (
	ResultDisplay ResultType = "display" // 显示文本
	ResultPrompt  ResultType = "prompt"  // 发送预设 prompt 给 Agent
	ResultAction  ResultType = "action"  // 执行内部动作（如退出、清屏）
)

// Result 斜杠命令执行结果
type Result struct {
	Type    ResultType
	Content string
}

// Command 斜杠命令定义
type Command struct {
	Name        string
	Description string
	Handler     func(args []string, ctx *Context) Result
}

// Context 命令执行上下文（由 TUI/Agent 注入）
type Context struct {
	// 模型/配置信息
	Model      string
	Provider   string
	Version    string
	PermMode   string
	Currency   string
	MaxContext int

	// token/费用信息
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostString   string
	CostUSD      float64

	// 会话信息
	SessionID    string
	SessionTurns int
	WorkDir      string

	// 缓存信息
	CacheCreationTokens int
	CacheReadTokens     int

	// 回调函数（需要外部行为的命令）
	OnClear       func()           // /clear
	OnCompact     func() string    // /compact
	OnModelSwitch func(string)     // /model <name>
	OnExport      func() string    // /export
	OnResume      func() string    // /resume
	OnLogin       func() string    // /login
	OnLogout      func() string    // /logout
	OnMCPList     func() string    // /mcp
	OnSkillsList  func() string    // /skills
	OnPluginsList func() string    // /plugins
	OnHooksList   func() string    // /hooks
}

// Handler 斜杠命令路由器
type Handler struct {
	commands map[string]*Command
}

// NewHandler 创建命令路由器
func NewHandler() *Handler {
	h := &Handler{
		commands: make(map[string]*Command),
	}
	h.registerAll()
	return h
}

// Handle 处理斜杠命令
func (h *Handler) Handle(input string, ctx *Context) (Result, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return Result{}, false
	}

	parts := strings.Fields(input)
	cmdName := parts[0]
	args := parts[1:]

	cmd, ok := h.commands[cmdName]
	if !ok {
		return Result{
			Type:    ResultDisplay,
			Content: fmt.Sprintf("未知命令: %s\n输入 /help 查看可用命令", cmdName),
		}, true
	}

	return cmd.Handler(args, ctx), true
}

// AllCommands 返回所有命令（按名称排序）
func (h *Handler) AllCommands() []*Command {
	var cmds []*Command
	for _, cmd := range h.commands {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// CommandNames 返回所有命令名称列表（用于 TUI 补全）
func (h *Handler) CommandNames() []string {
	var names []string
	for name := range h.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (h *Handler) register(cmd *Command) {
	h.commands[cmd.Name] = cmd
}

// registerAll 注册所有命令
func (h *Handler) registerAll() {
	// 会话类
	h.register(cmdHelp(h))
	h.register(cmdSession())
	h.register(cmdResume())
	h.register(cmdCompact())
	h.register(cmdClear())
	h.register(cmdExport())
	h.register(cmdQuit())
	h.register(cmdExit())

	// 模型与配置类
	h.register(cmdModel())
	h.register(cmdProvider())
	h.register(cmdConfig())
	h.register(cmdLogin())
	h.register(cmdLogout())
	h.register(cmdPermissions())
	h.register(cmdCost())
	h.register(cmdStatus())

	// 开发工作流类
	h.register(cmdCommit())
	h.register(cmdPR())
	h.register(cmdReview())
	h.register(cmdDiff())
	h.register(cmdPlan())
	h.register(cmdTest())
	h.register(cmdInit())

	// 系统类
	h.register(cmdEnv())
	h.register(cmdVersion())
	h.register(cmdContext())
	h.register(cmdTips())
	h.register(cmdDoctor())
	h.register(cmdBug())
	h.register(cmdMCP())
	h.register(cmdMemory())
	h.register(cmdSkills())
	h.register(cmdPlugins())
	h.register(cmdHooks())
	h.register(cmdAgents())
	h.register(cmdTeam())
	h.register(cmdBranch())
	h.register(cmdRefactor())
	h.register(cmdUpgrade())
}

// --- 会话类命令 ---

func cmdHelp(h *Handler) *Command {
	return &Command{
		Name:        "/help",
		Description: "帮助信息",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("⚡ Xin Code 命令列表\n\n")

			categories := []struct {
				name string
				cmds []string
			}{
				{"会话管理", []string{"/session", "/resume", "/compact", "/clear", "/export", "/quit"}},
				{"模型与配置", []string{"/model", "/provider", "/config", "/login", "/logout", "/permissions", "/cost"}},
				{"开发工作流", []string{"/commit", "/pr", "/review", "/diff", "/plan", "/test", "/init", "/branch", "/refactor"}},
				{"系统信息", []string{"/status", "/env", "/version", "/context", "/tips", "/doctor", "/bug", "/upgrade"}},
				{"扩展功能", []string{"/mcp", "/memory", "/skills", "/plugins", "/hooks", "/agents", "/team"}},
			}

			for _, cat := range categories {
				sb.WriteString(fmt.Sprintf("  %s\n", cat.name))
				for _, name := range cat.cmds {
					if cmd, ok := h.commands[name]; ok {
						sb.WriteString(fmt.Sprintf("    %-14s %s\n", cmd.Name, cmd.Description))
					}
				}
				sb.WriteString("\n")
			}

			sb.WriteString("快捷键: Ctrl+C 中断/退出  Ctrl+L 清屏")
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdSession() *Command {
	return &Command{
		Name:        "/session",
		Description: "当前会话信息",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("📋 会话信息\n\n")
			sb.WriteString(fmt.Sprintf("  ID:       %s\n", ctx.SessionID))
			sb.WriteString(fmt.Sprintf("  模型:     %s\n", ctx.Model))
			sb.WriteString(fmt.Sprintf("  轮次:     %d\n", ctx.SessionTurns))
			sb.WriteString(fmt.Sprintf("  费用:     %s\n", ctx.CostString))
			sb.WriteString(fmt.Sprintf("  工作目录: %s\n", ctx.WorkDir))

			// token 使用率
			if ctx.MaxContext > 0 {
				pct := float64(ctx.TotalTokens) / float64(ctx.MaxContext) * 100
				sb.WriteString(fmt.Sprintf("  上下文:   %d / %d (%.1f%%)\n", ctx.TotalTokens, ctx.MaxContext, pct))
			}

			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdResume() *Command {
	return &Command{
		Name:        "/resume",
		Description: "恢复历史会话",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnResume != nil {
				msg := ctx.OnResume()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "暂不支持会话恢复"}
		},
	}
}

func cmdCompact() *Command {
	return &Command{
		Name:        "/compact",
		Description: "压缩上下文",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnCompact != nil {
				msg := ctx.OnCompact()
				return Result{Type: ResultDisplay, Content: "⚡ " + msg}
			}
			return Result{Type: ResultDisplay, Content: "压缩功能未就绪"}
		},
	}
}

func cmdClear() *Command {
	return &Command{
		Name:        "/clear",
		Description: "清空当前对话",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnClear != nil {
				ctx.OnClear()
			}
			return Result{Type: ResultAction, Content: "clear"}
		},
	}
}

func cmdExport() *Command {
	return &Command{
		Name:        "/export",
		Description: "导出会话为 Markdown",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnExport != nil {
				msg := ctx.OnExport()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "导出功能未就绪"}
		},
	}
}

func cmdQuit() *Command {
	return &Command{
		Name:        "/quit",
		Description: "退出",
		Handler: func(args []string, ctx *Context) Result {
			return Result{Type: ResultAction, Content: "quit"}
		},
	}
}

func cmdExit() *Command {
	return &Command{
		Name:        "/exit",
		Description: "退出",
		Handler: func(args []string, ctx *Context) Result {
			return Result{Type: ResultAction, Content: "quit"}
		},
	}
}

// --- 模型与配置类命令 ---

func cmdModel() *Command {
	return &Command{
		Name:        "/model",
		Description: "显示/切换模型",
		Handler: func(args []string, ctx *Context) Result {
			if len(args) > 0 {
				newModel := args[0]
				if ctx.OnModelSwitch != nil {
					ctx.OnModelSwitch(newModel)
				}
				return Result{Type: ResultDisplay, Content: fmt.Sprintf("已切换模型: %s", newModel)}
			}
			return Result{Type: ResultDisplay, Content: fmt.Sprintf("当前模型: %s", ctx.Model)}
		},
	}
}

func cmdProvider() *Command {
	return &Command{
		Name:        "/provider",
		Description: "显示当前 Provider",
		Handler: func(args []string, ctx *Context) Result {
			return Result{Type: ResultDisplay, Content: fmt.Sprintf("当前 Provider: %s", ctx.Provider)}
		},
	}
}

func cmdConfig() *Command {
	return &Command{
		Name:        "/config",
		Description: "显示当前配置",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("⚙ 当前配置\n\n")
			sb.WriteString(fmt.Sprintf("  模型:     %s\n", ctx.Model))
			sb.WriteString(fmt.Sprintf("  Provider: %s\n", ctx.Provider))
			sb.WriteString(fmt.Sprintf("  权限模式: %s\n", ctx.PermMode))
			sb.WriteString(fmt.Sprintf("  货币:     %s\n", ctx.Currency))
			sb.WriteString(fmt.Sprintf("  最大上下文: %d\n", ctx.MaxContext))
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdLogin() *Command {
	return &Command{
		Name:        "/login",
		Description: "配置 API Key",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnLogin != nil {
				msg := ctx.OnLogin()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "请设置环境变量: export XINCODE_API_KEY=your-key"}
		},
	}
}

func cmdLogout() *Command {
	return &Command{
		Name:        "/logout",
		Description: "清除认证信息",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnLogout != nil {
				msg := ctx.OnLogout()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "认证信息已清除"}
		},
	}
}

func cmdPermissions() *Command {
	return &Command{
		Name:        "/permissions",
		Description: "显示权限模式",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("🔒 权限配置\n\n")
			sb.WriteString(fmt.Sprintf("  当前模式: %s\n\n", ctx.PermMode))
			sb.WriteString("  可用模式:\n")
			sb.WriteString("    bypass       所有工具自动放行\n")
			sb.WriteString("    acceptEdits  文件读写放行，执行类询问\n")
			sb.WriteString("    default      只读放行，写入询问（推荐）\n")
			sb.WriteString("    plan         只读放行，写入拒绝\n")
			sb.WriteString("    interactive  所有工具都询问\n")
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdCost() *Command {
	return &Command{
		Name:        "/cost",
		Description: "费用详情",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("💰 费用详情\n\n")
			sb.WriteString(fmt.Sprintf("  输入 tokens:       %d\n", ctx.InputTokens))
			sb.WriteString(fmt.Sprintf("  输出 tokens:       %d\n", ctx.OutputTokens))
			sb.WriteString(fmt.Sprintf("  总计 tokens:       %d\n", ctx.TotalTokens))
			if ctx.CacheCreationTokens > 0 || ctx.CacheReadTokens > 0 {
				sb.WriteString(fmt.Sprintf("  缓存写入 tokens:   %d\n", ctx.CacheCreationTokens))
				sb.WriteString(fmt.Sprintf("  缓存读取 tokens:   %d\n", ctx.CacheReadTokens))
				if ctx.CacheCreationTokens+ctx.CacheReadTokens > 0 {
					hitRate := float64(ctx.CacheReadTokens) / float64(ctx.CacheCreationTokens+ctx.CacheReadTokens) * 100
					sb.WriteString(fmt.Sprintf("  缓存命中率:        %.1f%%\n", hitRate))
				}
			}
			sb.WriteString(fmt.Sprintf("\n  总费用: %s\n", ctx.CostString))
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdStatus() *Command {
	return &Command{
		Name:        "/status",
		Description: "环境信息",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("📊 环境信息\n\n")
			sb.WriteString(fmt.Sprintf("  模型:       %s\n", ctx.Model))
			sb.WriteString(fmt.Sprintf("  Provider:   %s\n", ctx.Provider))
			sb.WriteString(fmt.Sprintf("  权限模式:   %s\n", ctx.PermMode))
			sb.WriteString(fmt.Sprintf("  工作目录:   %s\n", ctx.WorkDir))
			sb.WriteString(fmt.Sprintf("  费用:       %s\n", ctx.CostString))
			if ctx.MaxContext > 0 {
				pct := float64(ctx.TotalTokens) / float64(ctx.MaxContext) * 100
				sb.WriteString(fmt.Sprintf("  上下文:     %d / %d (%.1f%%)\n", ctx.TotalTokens, ctx.MaxContext, pct))
			}
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

// --- 开发工作流命令 ---

func cmdCommit() *Command {
	return &Command{
		Name:        "/commit",
		Description: "生成 commit 并提交",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultPrompt,
				Content: "请分析当前 git 变更（先执行 git diff 和 git status），生成合适的 commit message 并提交。commit 信息要简洁明了，描述变更的本质。",
			}
		},
	}
}

func cmdPR() *Command {
	return &Command{
		Name:        "/pr",
		Description: "创建 Pull Request",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultPrompt,
				Content: "请分析当前分支的所有 commit（从分支创建到 HEAD），创建一个 Pull Request。包含清晰的标题和描述。使用 gh pr create 命令。",
			}
		},
	}
}

func cmdReview() *Command {
	return &Command{
		Name:        "/review",
		Description: "审查代码变更",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultPrompt,
				Content: "请审查最近的代码变更。执行 git diff 查看变更内容，分析代码质量、潜在 bug、性能问题，并给出改进建议。",
			}
		},
	}
}

func cmdDiff() *Command {
	return &Command{
		Name:        "/diff",
		Description: "显示 git diff",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultPrompt,
				Content: "请执行 git diff 并展示当前的代码变更。如果没有 unstaged 变更，检查 staged 变更（git diff --cached）。",
			}
		},
	}
}

func cmdPlan() *Command {
	return &Command{
		Name:        "/plan",
		Description: "切换到计划模式",
		Handler: func(args []string, ctx *Context) Result {
			return Result{Type: ResultDisplay, Content: "已切换到 plan 模式（只读，不执行写入操作）"}
		},
	}
}

func cmdTest() *Command {
	return &Command{
		Name:        "/test",
		Description: "运行测试",
		Handler: func(args []string, ctx *Context) Result {
			extra := ""
			if len(args) > 0 {
				extra = " 重点关注: " + strings.Join(args, " ")
			}
			return Result{
				Type:    ResultPrompt,
				Content: "请运行项目测试。先检查项目的测试框架和配置，然后执行测试命令并报告结果。" + extra,
			}
		},
	}
}

func cmdInit() *Command {
	return &Command{
		Name:        "/init",
		Description: "创建 XINCODE.md",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultPrompt,
				Content: "请分析当前项目结构，创建一个 XINCODE.md 文件。包含项目概述、技术栈、目录结构、开发规范等关键信息，帮助 AI 助手理解项目上下文。",
			}
		},
	}
}

// --- 系统类命令 ---

func cmdEnv() *Command {
	return &Command{
		Name:        "/env",
		Description: "环境变量",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("🌍 相关环境变量\n\n")
			envVars := []string{
				"XINCODE_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY",
				"XINCODE_MODEL", "XINCODE_BASE_URL", "XINCODE_PERMISSION_MODE",
			}
			for _, key := range envVars {
				val := maskEnvValue(key)
				sb.WriteString(fmt.Sprintf("  %-28s %s\n", key, val))
			}
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdVersion() *Command {
	return &Command{
		Name:        "/version",
		Description: "版本信息",
		Handler: func(args []string, ctx *Context) Result {
			return Result{Type: ResultDisplay, Content: fmt.Sprintf("xin-code %s", ctx.Version)}
		},
	}
}

func cmdContext() *Command {
	return &Command{
		Name:        "/context",
		Description: "上下文使用详情",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("📊 上下文使用详情\n\n")
			sb.WriteString(fmt.Sprintf("  模型最大上下文: %d tokens\n", ctx.MaxContext))
			sb.WriteString(fmt.Sprintf("  已使用:         %d tokens\n", ctx.TotalTokens))
			if ctx.MaxContext > 0 {
				pct := float64(ctx.TotalTokens) / float64(ctx.MaxContext) * 100
				sb.WriteString(fmt.Sprintf("  使用率:         %.1f%%\n", pct))

				// 可视化进度条
				barWidth := 30
				filled := int(pct / 100 * float64(barWidth))
				if filled > barWidth {
					filled = barWidth
				}
				bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
				sb.WriteString(fmt.Sprintf("\n  [%s] %.1f%%\n", bar, pct))

				if pct >= 90 {
					sb.WriteString("\n  ⚠ 上下文接近上限，建议执行 /compact 压缩")
				} else if pct >= 80 {
					sb.WriteString("\n  ⚠ 上下文使用率较高")
				}
			}
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdTips() *Command {
	tips := []string{
		"使用 /compact 可以压缩上下文，释放 token 空间",
		"Ctrl+L 可以清屏，但不会清除对话历史",
		"使用 /export 可以将当前会话导出为 Markdown 文件",
		"使用 /resume 可以恢复之前的会话",
		"/commit 会分析 git 变更并自动生成 commit message",
		"使用 /model <name> 可以切换 AI 模型",
		"上下文进度条变黄（>60%）时注意控制对话长度",
		"/cost 可以查看详细的 token 使用和费用信息",
		"输入 / 后会自动提示可用命令",
		"/review 可以让 AI 审查最近的代码变更",
	}

	tipIndex := 0
	return &Command{
		Name:        "/tips",
		Description: "使用技巧",
		Handler: func(args []string, ctx *Context) Result {
			tip := tips[tipIndex%len(tips)]
			tipIndex++
			return Result{Type: ResultDisplay, Content: fmt.Sprintf("💡 %s", tip)}
		},
	}
}

// maskEnvValue 脱敏环境变量值
func maskEnvValue(key string) string {
	val := strings.TrimSpace(getEnv(key))
	if val == "" {
		return "(未设置)"
	}
	if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") {
		if len(val) > 8 {
			return val[:4] + "****" + val[len(val)-4:]
		}
		return "****"
	}
	return val
}

func getEnv(key string) string {
	return os.Getenv(key)
}

// --- 补充命令 ---

func cmdMCP() *Command {
	return &Command{
		Name:        "/mcp",
		Description: "MCP 服务器和工具列表",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnMCPList != nil {
				msg := ctx.OnMCPList()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "未配置 MCP 服务器"}
		},
	}
}

func cmdMemory() *Command {
	return &Command{
		Name:        "/memory",
		Description: "查看和管理记忆",
		Handler: func(args []string, ctx *Context) Result {
			homeDir, _ := os.UserHomeDir()
			memDir := memory.GetMemoryDir(homeDir, ctx.WorkDir)
			memories, err := memory.ScanMemoryDir(memDir)
			if err != nil {
				return Result{Type: ResultDisplay, Content: fmt.Sprintf("读取记忆失败: %s", err)}
			}
			if len(memories) == 0 {
				return Result{Type: ResultDisplay, Content: fmt.Sprintf("暂无记忆。\n记忆目录: %s\n\n模型可以在对话中自动创建和更新记忆。", memDir)}
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📝 记忆 (%d 条)\n\n", len(memories)))
			for _, m := range memories {
				typeTag := string(m.Type)
				if typeTag == "" {
					typeTag = "?"
				}
				desc := m.Description
				if len(desc) > 60 {
					desc = desc[:57] + "..."
				}
				sb.WriteString(fmt.Sprintf("  [%s] %s — %s\n", typeTag, m.Name, desc))
			}
			sb.WriteString(fmt.Sprintf("\n目录: %s", memDir))
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdSkills() *Command {
	return &Command{
		Name:        "/skills",
		Description: "已加载的技能列表",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnSkillsList != nil {
				msg := ctx.OnSkillsList()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "技能系统未初始化"}
		},
	}
}

func cmdPlugins() *Command {
	return &Command{
		Name:        "/plugins",
		Description: "已加载的插件列表",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnPluginsList != nil {
				msg := ctx.OnPluginsList()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "插件系统未初始化"}
		},
	}
}

func cmdHooks() *Command {
	return &Command{
		Name:        "/hooks",
		Description: "已配置的钩子列表",
		Handler: func(args []string, ctx *Context) Result {
			if ctx.OnHooksList != nil {
				msg := ctx.OnHooksList()
				return Result{Type: ResultDisplay, Content: msg}
			}
			return Result{Type: ResultDisplay, Content: "钩子系统未初始化"}
		},
	}
}

func cmdAgents() *Command {
	return &Command{
		Name:        "/agents",
		Description: "多 Agent 协作",
		Handler: func(args []string, ctx *Context) Result {
			return Result{Type: ResultDisplay, Content: "多 Agent 协作将在 v1.1 中实现"}
		},
	}
}

func cmdTeam() *Command {
	return &Command{
		Name:        "/team",
		Description: "团队功能",
		Handler: func(args []string, ctx *Context) Result {
			return Result{Type: ResultDisplay, Content: "团队功能将在 v1.1 中实现"}
		},
	}
}

func cmdBranch() *Command {
	return &Command{
		Name:        "/branch",
		Description: "Git 分支管理",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultPrompt,
				Content: "请帮我管理 git 分支",
			}
		},
	}
}

func cmdRefactor() *Command {
	return &Command{
		Name:        "/refactor",
		Description: "代码重构",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultPrompt,
				Content: "请分析并重构代码",
			}
		},
	}
}

func cmdUpgrade() *Command {
	return &Command{
		Name:        "/upgrade",
		Description: "检查更新",
		Handler: func(args []string, ctx *Context) Result {
			return Result{
				Type:    ResultDisplay,
				Content: fmt.Sprintf("当前版本: %s\n请访问 GitHub 检查最新版本: https://github.com/xincode-ai/xin-code/releases", ctx.Version),
			}
		},
	}
}

func cmdDoctor() *Command {
	return &Command{
		Name:        "/doctor",
		Description: "环境诊断",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("🩺 环境诊断\n\n")

			sb.WriteString(fmt.Sprintf("  ✓ Provider:     %s\n", ctx.Provider))
			sb.WriteString(fmt.Sprintf("  ✓ Model:        %s\n", ctx.Model))
			sb.WriteString(fmt.Sprintf("  ✓ 权限模式:     %s\n", ctx.PermMode))
			sb.WriteString(fmt.Sprintf("  ✓ 工作目录:     %s\n", ctx.WorkDir))

			homeDir, _ := os.UserHomeDir()
			configDir := filepath.Join(homeDir, ".xincode")
			settingsPath := filepath.Join(configDir, "settings.json")
			credPath := filepath.Join(configDir, "auth", "credentials.json")
			checkMark := func(path string) string {
				if _, err := os.Stat(path); err == nil {
					return "✓"
				}
				return "✗"
			}
			sb.WriteString(fmt.Sprintf("  %s settings.json:    %s\n", checkMark(settingsPath), settingsPath))
			sb.WriteString(fmt.Sprintf("  %s credentials.json: %s\n", checkMark(credPath), credPath))

			gitMark := "✗"
			if _, err := os.Stat(filepath.Join(ctx.WorkDir, ".git")); err == nil {
				gitMark = "✓"
			}
			sb.WriteString(fmt.Sprintf("  %s git 仓库\n", gitMark))

			xmMark := "✗"
			if _, err := os.Stat(filepath.Join(ctx.WorkDir, "XINCODE.md")); err == nil {
				xmMark = "✓"
			}
			sb.WriteString(fmt.Sprintf("  %s XINCODE.md\n", xmMark))

			if ctx.MaxContext > 0 {
				pct := float64(ctx.TotalTokens) / float64(ctx.MaxContext) * 100
				ctxMark := "✓"
				if pct > 80 {
					ctxMark = "⚠"
				}
				sb.WriteString(fmt.Sprintf("  %s 上下文:       %.1f%% (%d/%d)\n", ctxMark, pct, ctx.TotalTokens, ctx.MaxContext))
			}

			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}

func cmdBug() *Command {
	return &Command{
		Name:        "/bug",
		Description: "报告问题",
		Handler: func(args []string, ctx *Context) Result {
			var sb strings.Builder
			sb.WriteString("🐛 报告问题\n\n")
			sb.WriteString("  请在 GitHub 上提交 Issue：\n")
			sb.WriteString("  https://github.com/xincode-ai/xin-code/issues/new\n\n")
			sb.WriteString("  请附上以下信息：\n")
			sb.WriteString(fmt.Sprintf("  版本:     %s\n", ctx.Version))
			sb.WriteString(fmt.Sprintf("  模型:     %s\n", ctx.Model))
			sb.WriteString(fmt.Sprintf("  Provider: %s\n", ctx.Provider))
			sb.WriteString(fmt.Sprintf("  权限模式: %s\n", ctx.PermMode))
			return Result{Type: ResultDisplay, Content: sb.String()}
		},
	}
}
