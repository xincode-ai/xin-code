package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xincode-ai/xin-code/internal/cost"
	"github.com/xincode-ai/xin-code/internal/slash"
)

// AppState TUI 状态机
type AppState int

const (
	StateInput    AppState = iota // 等待用户输入
	StateQuery                   // 等待 API 响应
	StateToolExec                // 工具执行中
	StatePermission              // 等待权限确认
	StateDiffPreview             // 等待 Diff 确认
	StateAskUser                 // AskUser 等待用户输入
)

// EventSender Agent 向 TUI 发送事件的回调接口
type EventSender func(tea.Msg)

// App TUI 主 Model
type App struct {
	// 子组件
	statusBar  StatusBar
	chat       ChatView
	input      InputBox
	permission PermissionDialog
	spinner    spinner.Model

	// 斜杠命令
	slashHandler *slash.Handler

	// 状态
	state     AppState
	width     int
	height    int
	quitting  bool

	// Agent 通信
	eventCh          chan tea.Msg    // Agent goroutine -> TUI
	submitCh         chan string     // TUI -> 外部 Agent (用户输入)
	askResponseCh    chan string     // AskUser 回传 channel
	diffResponseCh   chan bool       // Diff 确认回传 channel
	waitingForEvent  bool           // 防止重复注册 waitForEvent

	// 配置
	model      string
	provider   string
	tracker    *cost.Tracker
	maxContext int
	version    string
	toolCount  int
	permMode   string
	workDir    string

	// 会话回调（由 main.go 注入）
	OnClear       func()
	OnCompact     func() string
	OnModelSwitch func(string)
	OnExport      func() string
	OnResume      func() string
	OnSkillsList  func() string
	OnPluginsList func() string
	OnHooksList   func() string

	// 会话信息
	SessionID    string
	SessionTurns int
}

// AppConfig TUI 初始化配置
type AppConfig struct {
	Model      string
	Provider   string
	Tracker    *cost.Tracker
	MaxContext int
	Version    string
	ToolCount  int
	PermMode   string
	WorkDir    string
}

// NewApp 创建 TUI 应用
func NewApp(cfg AppConfig) *App {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	slashH := slash.NewHandler()
	chat := NewChatView(80, 20)

	// 欢迎信息
	welcome := StyleBrand.Render("⚡ XIN CODE") + " " + StyleHint.Render("v"+cfg.Version) + "\n\n" +
		StyleTextDim.Render("  模型: ") + StyleModel.Render(cfg.Model) + "\n" +
		StyleTextDim.Render("  权限: ") + StyleHint.Render(cfg.PermMode) + "\n" +
		StyleTextDim.Render("  工具: ") + StyleHint.Render(fmt.Sprintf("%d 个", cfg.ToolCount)) + "\n\n" +
		StyleTextDim.Render("  输入消息开始对话。/help 查看命令。")
	chat.AddSystemMessage(welcome)

	return &App{
		statusBar:    NewStatusBar(cfg.Model, cfg.Tracker, cfg.MaxContext),
		chat:         chat,
		input:        NewInputBox(slashH.CommandNames()),
		permission:   NewPermissionDialog(),
		spinner:      sp,
		slashHandler: slashH,

		state:      StateInput,
		eventCh:    make(chan tea.Msg, 512),
		submitCh:   make(chan string, 8),

		model:      cfg.Model,
		provider:   cfg.Provider,
		tracker:    cfg.Tracker,
		maxContext: cfg.MaxContext,
		version:    cfg.Version,
		toolCount:  cfg.ToolCount,
		permMode:   cfg.PermMode,
		workDir:    cfg.WorkDir,
	}
}

// SubmitCh 返回用户提交消息的 channel（外部 Agent 读取）
func (a *App) SubmitCh() <-chan string {
	return a.submitCh
}

// Send 向 TUI 发送消息（Agent goroutine 调用）
func (a *App) Send(msg tea.Msg) {
	a.eventCh <- msg
}

// waitForEvent 等待来自 Agent 的事件
func waitForEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return MsgAgentDone{}
		}
		return msg
	}
}

// safeWaitForEvent 带防重复保护的 waitForEvent
func (a *App) safeWaitForEvent() tea.Cmd {
	if a.waitingForEvent {
		return nil
	}
	a.waitingForEvent = true
	return func() tea.Msg {
		msg, ok := <-a.eventCh
		if !ok {
			return MsgAgentDone{}
		}
		return msg
	}
}

func (a *App) Init() tea.Cmd {
	a.waitingForEvent = true
	return tea.Batch(
		a.input.Init(),
		a.spinner.Tick,
		waitForEvent(a.eventCh),
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// 来自 eventCh 的消息到达后，重置等待标志
	switch msg.(type) {
	case MsgTextDelta, MsgThinking, MsgToolStart, MsgToolDone,
		MsgUsage, MsgResponseDone, MsgAgentDone,
		MsgPermissionRequest, MsgDiffPreview, MsgAskUser, MsgError,
		MsgSystemNotice:
		a.waitingForEvent = false
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

		// 布局分配：状态栏 1 行，输入框 ~3 行，剩余给对话区
		statusH := 1
		inputH := 3
		chatH := a.height - statusH - inputH - 2 // 2 行间距

		chatMsg := tea.WindowSizeMsg{Width: msg.Width, Height: chatH}
		a.statusBar, _ = a.statusBar.Update(msg)
		a.chat, _ = a.chat.Update(chatMsg)
		a.input, _ = a.input.Update(msg)
		a.permission, _ = a.permission.Update(msg)
		return a, nil

	case tea.KeyMsg:
		// 全局快捷键
		switch msg.Type {
		case tea.KeyCtrlC:
			if a.state == StateQuery || a.state == StateToolExec {
				// 中断当前操作，回到输入状态
				a.state = StateInput
				a.chat.AddSystemMessage(StyleHint.Render("[已中断]"))
				return a, a.input.Focus()
			}
			a.quitting = true
			return a, tea.Quit
		case tea.KeyCtrlL:
			// 清屏
			a.chat.Clear()
			return a, nil
		}

		// 权限对话框拦截键盘
		if a.permission.IsVisible() {
			a.permission, _ = a.permission.Update(msg)
			// 权限对话框关闭后回到之前状态
			if !a.permission.IsVisible() {
				cmds = append(cmds, a.safeWaitForEvent())
			}
			return a, tea.Batch(cmds...)
		}

		// Diff 预览确认拦截键盘
		if a.state == StateDiffPreview {
			switch msg.String() {
			case "y", "Y":
				responseCh := a.diffResponseCh
				a.diffResponseCh = nil
				a.state = StateToolExec
				cmd := func() tea.Msg {
					if responseCh != nil {
						responseCh <- true
					}
					return nil
				}
				cmds = append(cmds, cmd, a.safeWaitForEvent())
				return a, tea.Batch(cmds...)
			case "n", "N":
				responseCh := a.diffResponseCh
				a.diffResponseCh = nil
				a.state = StateToolExec
				cmd := func() tea.Msg {
					if responseCh != nil {
						responseCh <- false
					}
					return nil
				}
				cmds = append(cmds, cmd, a.safeWaitForEvent())
				return a, tea.Batch(cmds...)
			}
			return a, nil
		}

		// AskUser 状态下，Enter 提交回答
		if a.state == StateAskUser {
			if msg.Type == tea.KeyEnter {
				answer := strings.TrimSpace(a.input.Value())
				if answer != "" {
					responseCh := a.askResponseCh
					a.askResponseCh = nil
					a.input.Reset()
					a.state = StateToolExec
					// 通过异步 Cmd 写入，避免阻塞 TUI 事件循环
					cmd := func() tea.Msg {
						if responseCh != nil {
							responseCh <- answer
						}
						return nil
					}
					cmds = append(cmds, cmd, a.safeWaitForEvent())
					return a, tea.Batch(cmds...)
				}
			}
			a.input, _ = a.input.Update(msg)
			return a, nil
		}

	case MsgSubmit:
		text := msg.Text
		// 斜杠命令处理
		if strings.HasPrefix(text, "/") {
			return a.handleSlashCommand(text)
		}
		// 用户消息 -> 更新对话区
		a.chat, _ = a.chat.Update(msg)
		a.state = StateQuery
		// 通过异步 Cmd 通知外部 Agent，避免阻塞 TUI 事件循环
		submitCmd := func() tea.Msg {
			a.submitCh <- text
			return nil
		}
		cmds = append(cmds, submitCmd, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgTextDelta:
		a.state = StateQuery
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgThinking:
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgToolStart:
		a.state = StateToolExec
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgToolDone:
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgUsage:
		if msg.Usage != nil {
			a.tracker.AddUsage(
				msg.Usage.InputTokens,
				msg.Usage.OutputTokens,
				msg.Usage.CacheCreationInputTokens,
				msg.Usage.CacheReadInputTokens,
			)
		}
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgResponseDone:
		a.chat, _ = a.chat.Update(msg)
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgAgentDone:
		a.state = StateInput
		if msg.Err != nil {
			a.chat, _ = a.chat.Update(MsgError{Err: msg.Err})
		}
		cmds = append(cmds, a.input.Focus())
		return a, tea.Batch(cmds...)

	case MsgPermissionRequest:
		a.state = StatePermission
		a.permission.Show(msg.ToolName, msg.Input, msg.Response)
		a.input.Blur()
		return a, nil

	case MsgDiffPreview:
		a.state = StateDiffPreview
		a.diffResponseCh = msg.Response
		a.chat.AddSystemMessage(StyleDiffHeader.Render("📝 Edit: "+msg.Path) + "\n" + msg.DiffText + "\n" + StyleHint.Render("[y]确认  [n]取消"))
		a.input.Blur()
		return a, nil

	case MsgAskUser:
		a.state = StateAskUser
		a.askResponseCh = msg.Response
		a.chat.AddSystemMessage(StylePermTitle.Render("❓ "+msg.Question))
		cmds = append(cmds, a.input.Focus())
		return a, tea.Batch(cmds...)

	case MsgSystemNotice:
		a.chat.AddSystemMessage(StyleHint.Render(msg.Text))
		cmds = append(cmds, a.safeWaitForEvent())
		return a, tea.Batch(cmds...)

	case MsgError:
		a.chat, _ = a.chat.Update(msg)
		a.state = StateInput
		cmds = append(cmds, a.input.Focus())
		return a, tea.Batch(cmds...)

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		cmds = append(cmds, cmd)
		return a, tea.Batch(cmds...)
	}

	// 默认传递给 input
	if a.state == StateInput || a.state == StateAskUser {
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a *App) View() string {
	if a.quitting {
		return StyleHint.Render("再见！\n")
	}

	if a.width == 0 {
		return "初始化中..."
	}

	var sections []string

	// 状态栏（顶部）
	sections = append(sections, a.statusBar.View())

	// 对话区域（中间）
	chatView := a.chat.View()
	sections = append(sections, chatView)

	// 状态指示器（极简，不抢视觉）
	switch a.state {
	case StateQuery:
		sections = append(sections, "  "+a.spinner.View())
	case StateToolExec:
		sections = append(sections, "  "+a.spinner.View())
	}

	// 权限对话框（覆盖在输入框位置）
	if a.permission.IsVisible() {
		sections = append(sections, a.permission.View())
	} else {
		// 输入框（底部）
		sections = append(sections, a.input.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (a *App) handleSlashCommand(cmd string) (tea.Model, tea.Cmd) {
	// 构建命令上下文
	ctx := &slash.Context{
		Model:               a.model,
		Provider:            a.provider,
		Version:             a.version,
		PermMode:            a.permMode,
		Currency:            a.tracker.Currency(),
		MaxContext:           a.maxContext,
		InputTokens:         a.tracker.InputTokens(),
		OutputTokens:        a.tracker.OutputTokens(),
		TotalTokens:         a.tracker.TotalTokens(),
		CostString:          a.tracker.CostString(),
		CostUSD:             a.tracker.TotalCostUSD(),
		SessionID:           a.SessionID,
		SessionTurns:        a.SessionTurns,
		WorkDir:             a.workDir,
		CacheCreationTokens: a.tracker.CacheCreationTokens(),
		CacheReadTokens:     a.tracker.CacheReadTokens(),
		OnClear:             a.OnClear,
		OnCompact:           a.OnCompact,
		OnModelSwitch:       a.OnModelSwitch,
		OnExport:            a.OnExport,
		OnResume:            a.OnResume,
		OnSkillsList:        a.OnSkillsList,
		OnPluginsList:       a.OnPluginsList,
		OnHooksList:         a.OnHooksList,
	}

	result, handled := a.slashHandler.Handle(cmd, ctx)
	if !handled {
		return a, nil
	}

	switch result.Type {
	case slash.ResultAction:
		switch result.Content {
		case "quit":
			a.quitting = true
			return a, tea.Quit
		case "clear":
			a.chat.Clear()
			return a, nil
		}

	case slash.ResultPrompt:
		// 预设 prompt -> 作为用户消息发送给 Agent
		a.chat.AddSystemMessage(StyleHint.Render(fmt.Sprintf("[%s]", cmd)))
		a.state = StateQuery
		submitCmd := func() tea.Msg {
			a.submitCh <- result.Content
			return nil
		}
		return a, tea.Batch(submitCmd, a.safeWaitForEvent())

	case slash.ResultDisplay:
		a.chat.AddSystemMessage(result.Content)
	}

	return a, nil
}
