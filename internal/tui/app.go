package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/xincode-ai/xin-code/internal/cost"
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
	tracker    *cost.Tracker
	maxContext int
	version    string
	toolCount  int
	permMode   string
}

// AppConfig TUI 初始化配置
type AppConfig struct {
	Model      string
	Tracker    *cost.Tracker
	MaxContext int
	Version    string
	ToolCount  int
	PermMode   string
}

// NewApp 创建 TUI 应用
func NewApp(cfg AppConfig) *App {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorAccent)

	return &App{
		statusBar:  NewStatusBar(cfg.Model, cfg.Tracker, cfg.MaxContext),
		chat:       NewChatView(80, 20),
		input:      NewInputBox(),
		permission: NewPermissionDialog(),
		spinner:    sp,

		state:      StateInput,
		eventCh:    make(chan tea.Msg, 512),
		submitCh:   make(chan string, 8),

		model:      cfg.Model,
		tracker:    cfg.Tracker,
		maxContext:  cfg.MaxContext,
		version:    cfg.Version,
		toolCount:  cfg.ToolCount,
		permMode:   cfg.PermMode,
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
		MsgPermissionRequest, MsgDiffPreview, MsgAskUser, MsgError:
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

	// 状态指示器
	switch a.state {
	case StateQuery:
		sections = append(sections, a.spinner.View()+StyleHint.Render(" 思考中..."))
	case StateToolExec:
		sections = append(sections, a.spinner.View()+StyleHint.Render(" 执行工具..."))
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
	switch cmd {
	case "/quit", "/exit":
		a.quitting = true
		return a, tea.Quit
	case "/help":
		help := strings.Join([]string{
			StyleBrand.Render("可用命令:"),
			"  /help    - 帮助信息",
			"  /model   - 当前模型",
			"  /version - 版本信息",
			"  /clear   - 清屏",
			"  /quit    - 退出",
			"",
			StyleHint.Render("快捷键: Ctrl+C 中断/退出  Ctrl+L 清屏"),
		}, "\n")
		a.chat.AddSystemMessage(help)
	case "/model":
		a.chat.AddSystemMessage(fmt.Sprintf("当前模型: %s", StyleModel.Render(a.model)))
	case "/version":
		a.chat.AddSystemMessage(fmt.Sprintf("xin-code %s", a.version))
	case "/clear":
		a.chat.Clear()
	default:
		a.chat.AddSystemMessage(StyleErrorMsg.Render("未知命令: " + cmd))
	}
	return a, nil
}
